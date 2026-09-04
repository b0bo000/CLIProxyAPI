package executor

import (
	"bufio"
	"bytes"
	"strings"

	claudeauth "github.com/router-for-me/CLIProxyAPI/v7/internal/auth/claude"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type claudeDiagnosticsRequestState struct {
	key      string
	sequence uint64
}

// beginClaudeDiagnostics applies the diagnostics policy at the request
// boundary. Diagnostics is deliberately stricter than the generic CLI profile:
// only a request whose native Claude Code signals were confirmed may create a
// continuity generation. Configured-but-unconfirmed profiles must not invent a
// native diagnostics chain, and caller-owned diagnostics remain untouched.
func beginClaudeDiagnostics(
	body []byte,
	auth *cliproxyauth.Auth,
	sessionID, baseURL string,
	softwareProfile helps.ResolvedClaudeSoftwareProfile,
	injectDiagnostics bool,
) ([]byte, claudeDiagnosticsRequestState) {
	if !claudeDiagnosticsEligible(injectDiagnostics, baseURL, softwareProfile) {
		return body, claudeDiagnosticsRequestState{}
	}
	if gjson.GetBytes(body, "diagnostics").Exists() {
		return body, claudeDiagnosticsRequestState{}
	}
	return injectClaudeDiagnostics(body, auth, sessionID)
}

func claudeDiagnosticsEligible(injectDiagnostics bool, baseURL string, softwareProfile helps.ResolvedClaudeSoftwareProfile) bool {
	return injectDiagnostics && isAnthropicUpstreamBase(baseURL) && softwareProfile.Confirmed && !softwareProfile.IsHelperProfile()
}

func injectClaudeDiagnostics(body []byte, auth *cliproxyauth.Auth, sessionID string) ([]byte, claudeDiagnosticsRequestState) {
	// A diagnostics object supplied by the caller owns its value and its state
	// lifecycle. Never overwrite it or allocate a CPA continuity generation.
	if gjson.GetBytes(body, "diagnostics").Exists() {
		return body, claudeDiagnosticsRequestState{}
	}
	key, sequence, previousMessageID := helps.BeginClaudeDiagnostics(claudeDiagnosticsCredentialIdentity(auth), sessionID)
	if key == "" {
		return body, claudeDiagnosticsRequestState{}
	}
	value := `{"previous_message_id":null}`
	if previousMessageID != "" {
		value = `{"previous_message_id":` + marshalJSONStringWithoutHTMLEscape(previousMessageID) + `}`
	}

	if diagnostics := gjson.GetBytes(body, "diagnostics"); diagnostics.Exists() {
		updated, errSet := sjson.SetRawBytes(body, "diagnostics", []byte(value))
		if errSet == nil {
			return updated, claudeDiagnosticsRequestState{key: key, sequence: sequence}
		}
	}
	if contextManagement := gjson.GetBytes(body, "context_management"); contextManagement.Exists() {
		start := contextManagement.Index
		insertAt := start + len(contextManagement.Raw)
		if start >= 0 && insertAt >= start && insertAt <= len(body) && bytes.Equal(body[start:insertAt], []byte(contextManagement.Raw)) {
			updated := make([]byte, 0, len(body)+len(value)+len(`,"diagnostics":`))
			updated = append(updated, body[:insertAt]...)
			updated = append(updated, `,"diagnostics":`...)
			updated = append(updated, value...)
			updated = append(updated, body[insertAt:]...)
			return updated, claudeDiagnosticsRequestState{key: key, sequence: sequence}
		}
	}
	updated, errSet := sjson.SetRawBytes(body, "diagnostics", []byte(value))
	if errSet != nil {
		return body, claudeDiagnosticsRequestState{}
	}
	return updated, claudeDiagnosticsRequestState{key: key, sequence: sequence}
}

func claudeDiagnosticsCredentialIdentity(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return ""
	}
	if id := strings.TrimSpace(auth.ID); id != "" {
		return "id:" + id
	}
	if index := strings.TrimSpace(auth.Index); index != "" {
		return "index:" + index
	}
	deviceIDs := claudeauth.NormalizeDeviceIDPool(claudeauth.ReadDeviceIDPool(&auth.Metadata))
	if len(deviceIDs) > 0 {
		return "device:" + deviceIDs[0]
	}
	if accountUUID := helps.ClaudeCredentialAccountUUID(auth); accountUUID != "" {
		return "account:" + accountUUID
	}
	return ""
}

func claudeCaptureProxyCacheKey(cfg *config.Config, auth *cliproxyauth.Auth) string {
	if auth != nil {
		if value := strings.TrimSpace(auth.ProxyURL); value != "" {
			return value
		}
	}
	if cfg != nil {
		return strings.TrimSpace(cfg.ProxyURL)
	}
	return ""
}

func commitClaudeDiagnostics(state claudeDiagnosticsRequestState, messageID string) {
	helps.CommitClaudeDiagnostics(state.key, state.sequence, messageID)
}

func claudeMessageIDFromResponse(data []byte) string {
	if !gjson.ValidBytes(data) {
		return ""
	}
	root := gjson.ParseBytes(data)
	messageType := root.Get("type")
	messageID := root.Get("id")
	if !root.IsObject() || messageType.Type != gjson.String || messageType.String() != "message" || messageID.Type != gjson.String {
		return ""
	}
	return strings.TrimSpace(messageID.String())
}

type claudeDiagnosticsStreamValidation struct {
	messageID       string
	hasData         bool
	hasMessageStart bool
	hasMessageDelta bool
	hasMessageStop  bool
	invalid         bool
}

func (validation *claudeDiagnosticsStreamValidation) Observe(line []byte) {
	line = bytes.TrimSpace(line)
	if !bytes.HasPrefix(line, []byte("data:")) {
		return
	}
	payload := bytes.TrimSpace(line[len("data:"):])
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return
	}
	validation.hasData = true
	if !gjson.ValidBytes(payload) {
		validation.invalid = true
		return
	}
	root := gjson.ParseBytes(payload)
	eventType := root.Get("type")
	if !root.IsObject() || eventType.Type != gjson.String {
		validation.invalid = true
		return
	}
	if validation.hasMessageStop {
		validation.invalid = true
		return
	}
	switch eventType.String() {
	case "error":
		validation.invalid = true
	case "message_start":
		messageID := root.Get("message.id")
		model := root.Get("message.model")
		if validation.hasMessageStart || messageID.Type != gjson.String || strings.TrimSpace(messageID.String()) == "" || model.Type != gjson.String || strings.TrimSpace(model.String()) == "" {
			validation.invalid = true
			return
		}
		validation.messageID = strings.TrimSpace(messageID.String())
		validation.hasMessageStart = true
	case "message_delta":
		if !validation.hasMessageStart {
			validation.invalid = true
			return
		}
		validation.hasMessageDelta = true
	case "message_stop":
		if !validation.hasMessageStart || !validation.hasMessageDelta {
			validation.invalid = true
			return
		}
		validation.hasMessageStop = true
	}
}

func (validation claudeDiagnosticsStreamValidation) Complete() bool {
	return !validation.invalid && validation.hasData && validation.hasMessageStart && validation.hasMessageDelta && validation.hasMessageStop && validation.messageID != ""
}

func claudeMessageIDFromSSE(data []byte) string {
	validation := claudeDiagnosticsStreamValidation{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(nil, 52_428_800)
	for scanner.Scan() {
		validation.Observe(scanner.Bytes())
	}
	if scanner.Err() != nil || !validation.Complete() {
		return ""
	}
	return validation.messageID
}
