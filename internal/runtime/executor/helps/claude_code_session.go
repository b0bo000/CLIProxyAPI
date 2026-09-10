package helps

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	ClaudeCodeSessionHeader = "X-Claude-Code-Session-Id"
	ClaudeCodeAgentHeader   = "X-Claude-Code-Agent-Id"
	ClaudeCodeMainAgentID   = "main"
)

var claudeCodeSessionSuffixPattern = regexp.MustCompile(`_session_([a-f0-9-]+)$`)

// ExtractClaudeCodeSessionID resolves a Claude Code session ID, preferring X-Claude-Code-Session-Id over payload metadata.
func ExtractClaudeCodeSessionID(ctx context.Context, payload []byte, headers http.Header) string {
	if sessionID := claudeCodeHeader(ctx, headers, ClaudeCodeSessionHeader); sessionID != "" {
		return sessionID
	}
	return extractClaudeCodeSessionIDFromPayload(payload)
}

// ExtractClaudeCodeAgentID resolves the Claude Code agent ID and uses a stable sentinel for the root agent.
func ExtractClaudeCodeAgentID(ctx context.Context, headers http.Header) string {
	if agentID := claudeCodeHeader(ctx, headers, ClaudeCodeAgentHeader); agentID != "" {
		return agentID
	}
	return ClaudeCodeMainAgentID
}

// ClaudeCodeExecutionScope returns the stable root-session and agent identity used by Codex execution state.
func ClaudeCodeExecutionScope(ctx context.Context, payload []byte, headers http.Header) (string, bool) {
	sessionID := ExtractClaudeCodeSessionID(ctx, payload, headers)
	return ClaudeCodeExecutionScopeForSession(ctx, sessionID, headers)
}

// ClaudeCodeExecutionScopeForSession builds the execution scope from an
// already-resolved provider session. It is used when an embedding host supplies
// the canonical session through executor metadata rather than Claude-specific
// headers or metadata.user_id.
func ClaudeCodeExecutionScopeForSession(ctx context.Context, sessionID string, headers http.Header) (string, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", false
	}
	return "claude:" + sessionID + ":agent:" + ExtractClaudeCodeAgentID(ctx, headers), true
}

// ClaudeCodePromptIDScope returns the scope used by prompt-ID state. The root
// agent and a child carrying the explicit first-party cc_is_subagent=true
// marker share the root scope, matching the official parent/child lifecycle.
// Other agent IDs remain isolated, and malformed or absent markers never cause
// an untrusted request to inherit the root scope.
func ClaudeCodePromptIDScope(ctx context.Context, payload []byte, headers http.Header) (string, bool) {
	sessionID := ExtractClaudeCodeSessionID(ctx, payload, headers)
	return ClaudeCodePromptIDScopeForSession(ctx, sessionID, payload, headers)
}

// ClaudeCodePromptIDScopeForSession builds prompt state scope from an
// already-resolved provider session while retaining the native parent/subagent
// inheritance rule.
func ClaudeCodePromptIDScopeForSession(ctx context.Context, sessionID string, payload []byte, headers http.Header) (string, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", false
	}
	agentID := ExtractClaudeCodeAgentID(ctx, headers)
	if agentID != ClaudeCodeMainAgentID {
		isSubagent, found, errMarker := ClaudeCodeSubagentMarkerFromBody(payload)
		if errMarker == nil && found && isSubagent {
			agentID = ClaudeCodeMainAgentID
		}
	}
	return "claude:" + sessionID + ":agent:" + agentID, true
}

func claudeCodeHeader(ctx context.Context, headers http.Header, name string) string {
	if value := headerValueCaseInsensitive(headers, name); value != "" {
		return value
	}
	if ctx != nil {
		if ginCtx, ok := ctx.Value("gin").(*gin.Context); ok && ginCtx != nil && ginCtx.Request != nil {
			return headerValueCaseInsensitive(ginCtx.Request.Header, name)
		}
	}
	return ""
}

// HeaderValueCaseInsensitive returns the first non-empty header value matching name case-insensitively.
func HeaderValueCaseInsensitive(headers http.Header, name string) string {
	return headerValueCaseInsensitive(headers, name)
}

// HeaderValuesCaseInsensitive returns all non-empty header values matching name case-insensitively.
func HeaderValuesCaseInsensitive(headers http.Header, name string) []string {
	if headers == nil {
		return nil
	}
	var result []string
	for key, values := range headers {
		if strings.EqualFold(key, name) {
			for _, value := range values {
				if trimmed := strings.TrimSpace(value); trimmed != "" {
					result = append(result, trimmed)
				}
			}
		}
	}
	return result
}

func headerValueCaseInsensitive(headers http.Header, name string) string {
	if headers == nil {
		return ""
	}
	if value := strings.TrimSpace(headers.Get(name)); value != "" {
		return value
	}
	for key, values := range headers {
		if !strings.EqualFold(key, name) {
			continue
		}
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func extractClaudeCodeSessionIDFromPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	userID := gjson.GetBytes(payload, "metadata.user_id").String()
	if userID == "" {
		return ""
	}
	if matches := claudeCodeSessionSuffixPattern.FindStringSubmatch(userID); len(matches) >= 2 {
		return matches[1]
	}
	if len(userID) > 0 && userID[0] == '{' {
		return strings.TrimSpace(gjson.Get(userID, "session_id").String())
	}
	return ""
}

// ClaudeCodePromptCache derives a deterministic upstream prompt_cache_key for one Claude Code agent.
func ClaudeCodePromptCache(ctx context.Context, modelName string, payload []byte, headers http.Header) (CodexCache, bool, error) {
	modelName = strings.TrimSpace(modelName)
	executionScope, ok := ClaudeCodeExecutionScope(ctx, payload, headers)
	if modelName == "" || !ok {
		return CodexCache{}, false, nil
	}
	identity := strings.Join([]string{"cli-proxy-api:codex:claude-code", modelName, executionScope}, "\x00")
	return CodexCache{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(identity)).String()}, true, nil
}
