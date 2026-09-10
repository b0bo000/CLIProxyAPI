package helps

import (
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestClaudeCodeTransportCredentialScopeUsesOpaqueStableIdentity(t *testing.T) {
	t.Parallel()

	a := &cliproxyauth.Auth{ID: "credential-a"}
	b := &cliproxyauth.Auth{ID: "credential-b"}
	first, ok := claudeCodeTransportCredentialScope(a)
	if !ok || first == "" {
		t.Fatalf("credential A scope = %q/%v, want non-empty scope", first, ok)
	}
	if second, ok := claudeCodeTransportCredentialScope(a.Clone()); !ok || second != first {
		t.Fatalf("credential A clone scope = %q/%v, want %q/true", second, ok, first)
	}
	other, ok := claudeCodeTransportCredentialScope(b)
	if !ok || other == first {
		t.Fatalf("credential B scope = %q/%v, want a distinct non-empty scope", other, ok)
	}
	if strings.Contains(first, a.ID) || strings.Contains(first, "credential") {
		t.Fatalf("scope contains credential material: %q", first)
	}
}

func TestClaudeCodeTransportCredentialScopePrecedence(t *testing.T) {
	t.Parallel()

	withID := &cliproxyauth.Auth{ID: "id-a", Index: "index-a", FileName: "file-a"}
	withIndex := &cliproxyauth.Auth{Index: "index-a", FileName: "file-a"}
	withFile := &cliproxyauth.Auth{FileName: "file-a"}
	idScope, idOK := claudeCodeTransportCredentialScope(withID)
	indexScope, indexOK := claudeCodeTransportCredentialScope(withIndex)
	fileScope, fileOK := claudeCodeTransportCredentialScope(withFile)
	if !idOK || !indexOK || !fileOK {
		t.Fatalf("scope availability = %v/%v/%v, want true", idOK, indexOK, fileOK)
	}
	if idScope == indexScope || indexScope == fileScope || idScope == fileScope {
		t.Fatalf("scope precedence collapsed identities: id=%q index=%q file=%q", idScope, indexScope, fileScope)
	}
}

func TestClaudeCodeTransportCredentialScopeMissingIdentityIsNotCacheable(t *testing.T) {
	t.Parallel()

	if scope, ok := claudeCodeTransportCredentialScope(&cliproxyauth.Auth{}); ok || scope != "" {
		t.Fatalf("missing identity scope = %q/%v, want empty/false", scope, ok)
	}
	if key, ok := claudeCodeTransportCacheKeyForAuth("http://proxy.invalid", &cliproxyauth.Auth{}); ok || key != (claudeCodeTransportCacheKey{}) {
		t.Fatalf("missing identity key = %#v/%v, want zero/false", key, ok)
	}
}

func TestCachedClaudeCodeRoundTripperScopesCredential(t *testing.T) {
	proxyURL := "http://127.0.0.1:29654"
	a := &cliproxyauth.Auth{ID: "transport-a"}
	b := &cliproxyauth.Auth{ID: "transport-b"}
	first := cachedClaudeCodeRoundTripperForAuth(proxyURL, a)
	if reused := cachedClaudeCodeRoundTripperForAuth(proxyURL, a.Clone()); reused != first {
		t.Fatal("same credential and proxy did not reuse transport")
	}
	if isolated := cachedClaudeCodeRoundTripperForAuth(proxyURL, b); isolated == first {
		t.Fatal("different credentials and proxy reused transport")
	}
	if isolated := cachedClaudeCodeRoundTripperForAuth("http://127.0.0.1:29655", a); isolated == first {
		t.Fatal("different proxies reused transport")
	}
}

func TestCachedClaudeCodeRoundTripperUnknownAuthDoesNotShare(t *testing.T) {
	proxyURL := "http://127.0.0.1:29656"
	first := cachedClaudeCodeRoundTripperForAuth(proxyURL, &cliproxyauth.Auth{})
	second := cachedClaudeCodeRoundTripperForAuth(proxyURL, &cliproxyauth.Auth{})
	if second == first {
		t.Fatal("unknown credentials unexpectedly shared a cached transport")
	}
}

func TestCachedClaudeCodeRoundTripperNilAuthRetainsProxyCompatibility(t *testing.T) {
	proxyURL := "http://127.0.0.1:29657"
	first := cachedClaudeCodeRoundTripperForAuth(proxyURL, nil)
	if second := cachedClaudeCodeRoundTripper(proxyURL); second != first {
		t.Fatal("nil-auth compatibility lookup did not reuse proxy transport")
	}
}

func TestCachedClaudeCodeRoundTripperScopesOwner(t *testing.T) {
	t.Parallel()

	proxyURL := "http://127.0.0.1:29658"
	auth := &cliproxyauth.Auth{ID: "transport-owner"}
	firstOwner := NewClaudeCodeTransportOwner()
	secondOwner := NewClaudeCodeTransportOwner()
	firstToken, ok := claudeCodeTransportOwnerFromContext(WithClaudeCodeTransportOwner(t.Context(), firstOwner))
	if !ok {
		t.Fatal("first owner was not extracted")
	}
	secondToken, ok := claudeCodeTransportOwnerFromContext(WithClaudeCodeTransportOwner(t.Context(), secondOwner))
	if !ok {
		t.Fatal("second owner was not extracted")
	}

	first := cachedClaudeCodeRoundTripperForAuthAndOwner(proxyURL, auth, firstToken)
	if reused := cachedClaudeCodeRoundTripperForAuthAndOwner(proxyURL, auth.Clone(), firstToken); reused != first {
		t.Fatal("same credential, proxy, and owner did not reuse transport")
	}
	if isolated := cachedClaudeCodeRoundTripperForAuthAndOwner(proxyURL, auth, secondToken); isolated == first {
		t.Fatal("different owners unexpectedly reused transport")
	}
	if fallback := cachedClaudeCodeRoundTripperForAuth(proxyURL, auth); fallback == first {
		t.Fatal("owner-scoped transport unexpectedly reused ownerless transport")
	}
}

func TestNewUtlsHTTPClientCarriesOwnerIntoClaudeTransportCache(t *testing.T) {
	t.Parallel()

	proxyURL := "http://127.0.0.1:29659"
	auth := &cliproxyauth.Auth{ID: "client-owner"}
	firstOwner := NewClaudeCodeTransportOwner()
	secondOwner := NewClaudeCodeTransportOwner()
	firstCtx := WithClaudeCodeTransportOwner(t.Context(), firstOwner)
	secondCtx := WithClaudeCodeTransportOwner(t.Context(), secondOwner)
	firstClient := NewUtlsHTTPClient(firstCtx, nil, &cliproxyauth.Auth{ID: auth.ID, ProxyURL: proxyURL}, 0)
	reusedClient := NewUtlsHTTPClient(firstCtx, nil, &cliproxyauth.Auth{ID: auth.ID, ProxyURL: proxyURL}, 0)
	isolatedClient := NewUtlsHTTPClient(secondCtx, nil, &cliproxyauth.Auth{ID: auth.ID, ProxyURL: proxyURL}, 0)

	first := firstClient.Transport.(*fallbackRoundTripper).anthropic
	if reused := reusedClient.Transport.(*fallbackRoundTripper).anthropic; reused != first {
		t.Fatal("same owner context did not reuse Claude transport")
	}
	if isolated := isolatedClient.Transport.(*fallbackRoundTripper).anthropic; isolated == first {
		t.Fatal("different owner contexts unexpectedly reused Claude transport")
	}
}

func TestCachedClaudeCodeRoundTripperScopesNilAuthOwner(t *testing.T) {
	t.Parallel()

	proxyURL := "http://127.0.0.1:29660"
	firstToken, ok := claudeCodeTransportOwnerFromContext(WithClaudeCodeTransportOwner(t.Context(), NewClaudeCodeTransportOwner()))
	if !ok {
		t.Fatal("first owner was not extracted")
	}
	secondToken, ok := claudeCodeTransportOwnerFromContext(WithClaudeCodeTransportOwner(t.Context(), NewClaudeCodeTransportOwner()))
	if !ok {
		t.Fatal("second owner was not extracted")
	}

	first := cachedClaudeCodeRoundTripperForAuthAndOwner(proxyURL, nil, firstToken)
	if reused := cachedClaudeCodeRoundTripperForAuthAndOwner(proxyURL, nil, firstToken); reused != first {
		t.Fatal("same owner and nil auth did not reuse transport")
	}
	if isolated := cachedClaudeCodeRoundTripperForAuthAndOwner(proxyURL, nil, secondToken); isolated == first {
		t.Fatal("different owners with nil auth unexpectedly reused transport")
	}
}

func TestCachedClaudeCodeRoundTripperScopesTLSSessionPolicy(t *testing.T) {
	t.Parallel()

	proxyURL := "http://127.0.0.1:29661"
	auth := &cliproxyauth.Auth{ID: "transport-policy"}
	owner, ok := claudeCodeTransportOwnerFromContext(WithClaudeCodeTransportOwner(t.Context(), NewClaudeCodeTransportOwner()))
	if !ok {
		t.Fatal("owner was not extracted")
	}
	enabled := cachedClaudeCodeRoundTripperForAuthOwnerAndPolicy(proxyURL, auth, owner, true)
	if reused := cachedClaudeCodeRoundTripperForAuthOwnerAndPolicy(proxyURL, auth.Clone(), owner, true); reused != enabled {
		t.Fatal("same enabled policy did not reuse transport")
	}
	disabled := cachedClaudeCodeRoundTripperForAuthOwnerAndPolicy(proxyURL, auth, owner, false)
	if disabled == enabled {
		t.Fatal("enabled and disabled TLS policies unexpectedly shared transport")
	}
	if key, ok := claudeCodeTransportCacheKeyForAuthOwnerAndPolicy(proxyURL, auth, owner, true); !ok || !key.TLSSessionResumption {
		t.Fatalf("enabled cache key = %#v/%v", key, ok)
	}
	if key, ok := claudeCodeTransportCacheKeyForAuthOwnerAndPolicy(proxyURL, auth, owner, false); !ok || key.TLSSessionResumption {
		t.Fatalf("disabled cache key = %#v/%v", key, ok)
	}
	if !ClaudeCodeTLSSessionResumptionEnabled(&config.Config{}) {
		t.Fatal("nil policy did not preserve enabled default")
	}
	disabledValue := false
	if ClaudeCodeTLSSessionResumptionEnabled(&config.Config{SDKConfig: config.SDKConfig{ClaudeCode: config.ClaudeCodeConfig{TLSSessionResumption: &disabledValue}}}) {
		t.Fatal("explicit false policy resolved as enabled")
	}
}

func TestNewUtlsHTTPClientScopesTransportByClaudeSession(t *testing.T) {
	proxyURL := "http://127.0.0.1:29662"
	auth := &cliproxyauth.Auth{ID: "session-scoped-client", ProxyURL: proxyURL}
	cfg := &config.Config{SDKConfig: config.SDKConfig{ClaudeCode: config.ClaudeCodeConfig{SessionScopedTransport: true}}}
	first := NewUtlsHTTPClient(claudeTransportSessionContext("session-scope-a"), cfg, auth, 0).Transport.(*fallbackRoundTripper).anthropic
	reused := NewUtlsHTTPClient(claudeTransportSessionContext("session-scope-a"), cfg, auth.Clone(), 0).Transport.(*fallbackRoundTripper).anthropic
	isolated := NewUtlsHTTPClient(claudeTransportSessionContext("session-scope-b"), cfg, auth.Clone(), 0).Transport.(*fallbackRoundTripper).anthropic
	otherCredential := NewUtlsHTTPClient(claudeTransportSessionContext("session-scope-a"), cfg, &cliproxyauth.Auth{ID: "session-scoped-other", ProxyURL: proxyURL}, 0).Transport.(*fallbackRoundTripper).anthropic

	if reused != first {
		t.Fatal("same credential, proxy, and session did not reuse transport")
	}
	if isolated == first {
		t.Fatal("different sessions reused one transport")
	}
	if otherCredential == first {
		t.Fatal("different credentials reused one session-scoped transport")
	}
}

func TestNewUtlsHTTPClientSessionScopeMissingSessionDoesNotShare(t *testing.T) {
	proxyURL := "http://127.0.0.1:29663"
	auth := &cliproxyauth.Auth{ID: "session-scoped-missing", ProxyURL: proxyURL}
	cfg := &config.Config{SDKConfig: config.SDKConfig{ClaudeCode: config.ClaudeCodeConfig{SessionScopedTransport: true}}}
	first := NewUtlsHTTPClient(t.Context(), cfg, auth, 0).Transport.(*fallbackRoundTripper).anthropic
	second := NewUtlsHTTPClient(t.Context(), cfg, auth.Clone(), 0).Transport.(*fallbackRoundTripper).anthropic
	if second == first {
		t.Fatal("sessionless requests shared a transport")
	}
}

func TestNewUtlsHTTPClientExplicitOwnerPrecedesSessionScope(t *testing.T) {
	proxyURL := "http://127.0.0.1:29664"
	auth := &cliproxyauth.Auth{ID: "session-scoped-explicit", ProxyURL: proxyURL}
	cfg := &config.Config{SDKConfig: config.SDKConfig{ClaudeCode: config.ClaudeCodeConfig{SessionScopedTransport: true}}}
	owner := NewClaudeCodeTransportOwner()
	firstContext := WithClaudeCodeTransportOwner(claudeTransportSessionContext("session-explicit-a"), owner)
	secondContext := WithClaudeCodeTransportOwner(claudeTransportSessionContext("session-explicit-b"), owner)
	first := NewUtlsHTTPClient(firstContext, cfg, auth, 0).Transport.(*fallbackRoundTripper).anthropic
	second := NewUtlsHTTPClient(secondContext, cfg, auth.Clone(), 0).Transport.(*fallbackRoundTripper).anthropic
	if second != first {
		t.Fatal("explicit trusted owner did not override session approximation")
	}
}

func TestNewUtlsHTTPClientSessionScopeDisabledPreservesOwnerlessReuse(t *testing.T) {
	proxyURL := "http://127.0.0.1:29665"
	auth := &cliproxyauth.Auth{ID: "session-scoped-disabled", ProxyURL: proxyURL}
	cfg := &config.Config{}
	first := NewUtlsHTTPClient(claudeTransportSessionContext("session-disabled-a"), cfg, auth, 0).Transport.(*fallbackRoundTripper).anthropic
	second := NewUtlsHTTPClient(claudeTransportSessionContext("session-disabled-b"), cfg, auth.Clone(), 0).Transport.(*fallbackRoundTripper).anthropic
	if second != first {
		t.Fatal("disabled session scope changed ownerless compatibility behavior")
	}
	if ClaudeCodeSessionScopedTransportEnabled(cfg) {
		t.Fatal("empty config enabled session-scoped transport")
	}
}
