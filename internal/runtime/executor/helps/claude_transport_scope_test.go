package helps

import (
	"strings"
	"testing"

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
