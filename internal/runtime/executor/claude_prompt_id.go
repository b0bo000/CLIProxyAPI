package executor

import (
	"fmt"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/tidwall/gjson"
)

type claudePromptIDRequestKind uint8

const (
	claudePromptIDRequestKindMessages claudePromptIDRequestKind = iota
	claudePromptIDRequestKindCompactionBoundary
)

// claudePromptIDRequestKindForBody recognizes the native compaction summary
// request without treating the same text in retained history as a new boundary.
func claudePromptIDRequestKindForBody(body []byte) claudePromptIDRequestKind {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() || len(messages.Array()) == 0 {
		return claudePromptIDRequestKindMessages
	}
	last := messages.Array()[len(messages.Array())-1]
	if last.Get("role").String() != "user" {
		return claudePromptIDRequestKindMessages
	}
	content := last.Get("content")
	var textParts []string
	if content.Type == gjson.String {
		textParts = append(textParts, content.String())
	} else if content.IsArray() {
		for _, block := range content.Array() {
			if block.Get("type").String() == "text" && block.Get("text").Type == gjson.String {
				textParts = append(textParts, block.Get("text").String())
			}
		}
	}
	text := strings.Join(textParts, "\n")
	if strings.Contains(text, "CRITICAL: Respond with TEXT ONLY.") &&
		strings.Contains(text, "Do NOT call any tools.") &&
		strings.Contains(text, "Your task is to create a detailed summary of the conversation so far") &&
		strings.Contains(text, "<analysis>") &&
		strings.Contains(text, "<summary>") {
		return claudePromptIDRequestKindCompactionBoundary
	}
	return claudePromptIDRequestKindMessages
}

// claudePromptIDGenerationEligible mirrors the first-party billing gate. The
// caller must already be a confirmed native request or an explicit CLI profile;
// helper/title requests and custom gateways retain their measured behavior.
func claudePromptIDGenerationEligible(baseURL string, profile helps.ResolvedClaudeSoftwareProfile, cchSigning bool) bool {
	if !cchSigning || !isAnthropicUpstreamBase(baseURL) || profile.IsHelperProfile() || profile.IsTitleRequest() {
		return false
	}
	return profile.Confirmed || profile.Provenance == helps.ClaudeSoftwareProfileConfiguredCLI
}

func claudePromptIDGenerationEligibleForBody(baseURL string, profile helps.ResolvedClaudeSoftwareProfile, cchSigning bool, body []byte) bool {
	return claudePromptIDGenerationEligible(baseURL, profile, cchSigning) &&
		claudePromptIDRequestKindForBody(body) == claudePromptIDRequestKindMessages
}

func beginClaudePromptIDForRequest(
	body []byte,
	auth *cliproxyauth.Auth,
	apiKey, sessionScope, baseURL string,
	profile helps.ResolvedClaudeSoftwareProfile,
	cchSigning bool,
) (string, error) {
	if !claudePromptIDGenerationEligibleForBody(baseURL, profile, cchSigning, body) {
		return "", nil
	}
	identity := claudePrevRequestCredentialIdentity(auth, apiKey)
	if strings.TrimSpace(identity) == "" || strings.TrimSpace(sessionScope) == "" {
		return "", nil
	}
	isSubagent, markerFound, errMarker := helps.ClaudeCodeSubagentMarkerFromBody(body)
	if errMarker != nil {
		return "", fmt.Errorf("resolve Claude subagent marker: %w", errMarker)
	}
	var promptID string
	var errPrompt error
	if markerFound && isSubagent {
		promptID, _, errPrompt = helps.BeginClaudePromptIDInherited(identity, sessionScope, body)
	} else {
		promptID, _, errPrompt = helps.BeginClaudePromptID(identity, sessionScope, body)
	}
	if errPrompt != nil {
		return "", fmt.Errorf("resolve Claude cc_prompt_id: %w", errPrompt)
	}
	return promptID, nil
}
