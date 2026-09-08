package helps

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

type claudeCodeTransportCacheKey struct {
	ProxyURL             string
	CredentialScope      string
	OwnerScope           *claudeCodeTransportOwnerToken
	TLSSessionResumption bool
}

// ClaudeCodeTLSSessionResumptionEnabled resolves the optional inference-plane
// policy. The nil default intentionally preserves the pre-S7 behavior.
func ClaudeCodeTLSSessionResumptionEnabled(cfg *config.Config) bool {
	if cfg == nil || cfg.ClaudeCode.TLSSessionResumption == nil {
		return true
	}
	return *cfg.ClaudeCode.TLSSessionResumption
}

// claudeCodeTransportCredentialScope returns an opaque, stable cache scope for
// a credential without retaining any credential material in the cache key.
func claudeCodeTransportCredentialScope(auth *cliproxyauth.Auth) (string, bool) {
	seed := strings.TrimSpace(ClaudeCLIAuthIdentitySeed(auth))
	if seed == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte("cpa:claude-code-transport-credential:v1\x00" + seed))
	return hex.EncodeToString(sum[:]), true
}

func claudeCodeTransportCacheKeyForAuthAndOwner(proxyURL string, auth *cliproxyauth.Auth, owner *claudeCodeTransportOwnerToken) (claudeCodeTransportCacheKey, bool) {
	return claudeCodeTransportCacheKeyForAuthOwnerAndPolicy(proxyURL, auth, owner, true)
}

func claudeCodeTransportCacheKeyForAuthOwnerAndPolicy(proxyURL string, auth *cliproxyauth.Auth, owner *claudeCodeTransportOwnerToken, tlsSessionResumption bool) (claudeCodeTransportCacheKey, bool) {
	key := claudeCodeTransportCacheKey{ProxyURL: proxyURL, OwnerScope: owner, TLSSessionResumption: tlsSessionResumption}
	if auth == nil {
		return key, true
	}
	scope, ok := claudeCodeTransportCredentialScope(auth)
	if !ok {
		return claudeCodeTransportCacheKey{}, false
	}
	key.CredentialScope = scope
	return key, true
}

func claudeCodeTransportCacheKeyForAuth(proxyURL string, auth *cliproxyauth.Auth) (claudeCodeTransportCacheKey, bool) {
	return claudeCodeTransportCacheKeyForAuthAndOwner(proxyURL, auth, nil)
}
