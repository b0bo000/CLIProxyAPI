package executor

import (
	"fmt"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// claudePromptIDGenerationEligible mirrors the first-party billing gate. The
// caller must already be a confirmed native request or an explicit CLI profile;
// helper/title requests and custom gateways retain their measured behavior.
func claudePromptIDGenerationEligible(baseURL string, profile helps.ResolvedClaudeSoftwareProfile, cchSigning bool) bool {
	if !cchSigning || !isAnthropicUpstreamBase(baseURL) || profile.IsHelperProfile() {
		return false
	}
	return profile.Confirmed || profile.Provenance == helps.ClaudeSoftwareProfileConfiguredCLI
}

func beginClaudePromptIDForRequest(
	body []byte,
	auth *cliproxyauth.Auth,
	apiKey, sessionScope, baseURL string,
	profile helps.ResolvedClaudeSoftwareProfile,
	cchSigning bool,
) (string, error) {
	if !claudePromptIDGenerationEligible(baseURL, profile, cchSigning) {
		return "", nil
	}
	identity := claudePrevRequestCredentialIdentity(auth, apiKey)
	if strings.TrimSpace(identity) == "" || strings.TrimSpace(sessionScope) == "" {
		return "", nil
	}
	promptID, _, errPrompt := helps.BeginClaudePromptID(identity, sessionScope, body)
	if errPrompt != nil {
		return "", fmt.Errorf("resolve Claude cc_prompt_id: %w", errPrompt)
	}
	return promptID, nil
}
