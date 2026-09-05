package helps

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

type claudeCodeTransportCacheKey struct {
	ProxyURL        string
	CredentialScope string
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

func claudeCodeTransportCacheKeyForAuth(proxyURL string, auth *cliproxyauth.Auth) (claudeCodeTransportCacheKey, bool) {
	key := claudeCodeTransportCacheKey{ProxyURL: proxyURL}
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
