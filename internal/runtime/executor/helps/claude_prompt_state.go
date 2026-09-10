package helps

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const ClaudePromptIDBillingField = "cc_prompt_id"

const claudeSubagentBillingField = "cc_is_subagent"

const (
	claudePromptStateTTL            = time.Hour
	claudePromptStateCleanupPeriod  = 15 * time.Minute
	claudePromptStateMaxEntries     = 4096
	claudePromptStateEvictBatchSize = 256
)

type claudePromptStateEntry struct {
	promptID     string
	boundaryHash string
	lastAccess   uint64
	expiresAt    time.Time
}

var claudePromptState = struct {
	sync.Mutex
	entries     map[string]claudePromptStateEntry
	lastCleanup time.Time
	nextAccess  uint64
}{entries: make(map[string]claudePromptStateEntry)}

// ParseClaudePromptIDBillingText reads a caller-owned prompt ID from one
// Claude billing block. It does not generate a value. The boolean distinguishes
// an absent field from a present field whose value is empty or invalid.
func ParseClaudePromptIDBillingText(text string) (string, bool, error) {
	trimmed := strings.TrimSpace(text)
	const prefix = "x-anthropic-billing-header:"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", false, nil
	}

	seen := false
	value := ""
	for _, segment := range strings.Split(trimmed[len(prefix):], ";") {
		part := strings.TrimSpace(segment)
		if part == "" {
			continue
		}
		key, fieldValue, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return "", false, fmt.Errorf("parse Claude cc_prompt_id: malformed billing segment %q", part)
		}
		if strings.TrimSpace(key) != ClaudePromptIDBillingField {
			continue
		}
		if seen {
			return "", false, fmt.Errorf("parse Claude cc_prompt_id: duplicate %s", ClaudePromptIDBillingField)
		}
		seen = true
		value = strings.TrimSpace(fieldValue)
		if errValidate := validateClaudePromptID(value); errValidate != nil {
			return "", false, errValidate
		}
	}
	if !seen {
		return "", false, nil
	}
	return value, true, nil
}

func validateClaudePromptID(value string) error {
	if value == "" {
		return fmt.Errorf("parse Claude cc_prompt_id: empty prompt ID")
	}
	if len(value) != 36 || !utf8.ValidString(value) {
		return fmt.Errorf("parse Claude cc_prompt_id: prompt ID must be a UUID")
	}
	for index, character := range []byte(value) {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return fmt.Errorf("parse Claude cc_prompt_id: prompt ID must be a UUID")
			}
			continue
		}
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return fmt.Errorf("parse Claude cc_prompt_id: prompt ID must be a lowercase UUID")
		}
	}
	return nil
}

// ClaudePromptIDFromBody reads the single first-party billing block from a
// Messages body. It returns absent when the body has no billing block or the
// block has no caller-owned prompt ID.
func ClaudePromptIDFromBody(body []byte) (string, bool, error) {
	if !gjson.ValidBytes(body) {
		return "", false, fmt.Errorf("parse Claude cc_prompt_id: malformed JSON body")
	}
	candidates := claudePromptBillingCandidates(body)
	if len(candidates) > 1 {
		return "", false, fmt.Errorf("parse Claude cc_prompt_id: multiple billing blocks")
	}
	if len(candidates) == 0 {
		return "", false, nil
	}
	return ParseClaudePromptIDBillingText(candidates[0].text)
}

// ClaudeCodeSubagentMarkerFromBody reads the explicit first-party subagent
// marker from the single Claude billing block. A malformed or ambiguous body
// fails closed as (false, false, error); callers that only need a routing
// decision can treat any non-nil error as an absent marker.
func ClaudeCodeSubagentMarkerFromBody(body []byte) (bool, bool, error) {
	if !gjson.ValidBytes(body) {
		return false, false, fmt.Errorf("parse Claude subagent marker: malformed JSON body")
	}
	candidates := claudePromptBillingCandidates(body)
	if len(candidates) > 1 {
		return false, false, fmt.Errorf("parse Claude subagent marker: multiple billing blocks")
	}
	if len(candidates) == 0 {
		return false, false, nil
	}
	trimmed := strings.TrimSpace(candidates[0].text)
	const prefix = "x-anthropic-billing-header:"
	if !strings.HasPrefix(trimmed, prefix) {
		return false, false, nil
	}
	found := false
	value := ""
	for _, segment := range strings.Split(trimmed[len(prefix):], ";") {
		part := strings.TrimSpace(segment)
		if part == "" {
			continue
		}
		key, fieldValue, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return false, false, fmt.Errorf("parse Claude subagent marker: malformed billing segment %q", part)
		}
		if strings.TrimSpace(key) != claudeSubagentBillingField {
			continue
		}
		if found {
			return false, false, fmt.Errorf("parse Claude subagent marker: duplicate %s", claudeSubagentBillingField)
		}
		found = true
		value = strings.TrimSpace(fieldValue)
	}
	return value == "true", found, nil
}

type claudePromptBillingCandidate struct {
	path string
	text string
}

func claudePromptBillingCandidates(body []byte) []claudePromptBillingCandidate {
	var candidates []claudePromptBillingCandidate
	system := gjson.GetBytes(body, "system")
	if system.Type == gjson.String {
		if strings.HasPrefix(strings.TrimSpace(system.String()), "x-anthropic-billing-header:") {
			candidates = append(candidates, claudePromptBillingCandidate{path: "system", text: system.String()})
		}
		return candidates
	}
	if !system.IsArray() {
		return candidates
	}
	for index, block := range system.Array() {
		text := block.Get("text")
		if text.Type == gjson.String && strings.HasPrefix(strings.TrimSpace(text.String()), "x-anthropic-billing-header:") {
			candidates = append(candidates, claudePromptBillingCandidate{
				path: fmt.Sprintf("system.%d.text", index),
				text: text.String(),
			})
		}
	}
	return candidates
}

// InsertClaudePromptIDBilling appends a generated prompt ID to the existing
// billing block. Caller-owned values are left byte-for-byte unchanged.
func InsertClaudePromptIDBilling(body []byte, promptID string) ([]byte, error) {
	if errValidate := validateClaudePromptID(promptID); errValidate != nil {
		return nil, fmt.Errorf("insert Claude cc_prompt_id: %w", errValidate)
	}
	candidates := claudePromptBillingCandidates(body)
	if len(candidates) > 1 {
		return nil, fmt.Errorf("insert Claude cc_prompt_id: multiple billing blocks")
	}
	if len(candidates) == 0 {
		return body, nil
	}
	_, found, errPrompt := ParseClaudePromptIDBillingText(candidates[0].text)
	if errPrompt != nil {
		return nil, errPrompt
	}
	if found {
		return body, nil
	}
	updatedText, errAppend := appendClaudePromptIDBillingText(candidates[0].text, promptID)
	if errAppend != nil {
		return nil, errAppend
	}
	updated, errSet := sjson.SetBytes(body, candidates[0].path, updatedText)
	if errSet != nil {
		return nil, fmt.Errorf("insert Claude cc_prompt_id at %s: %w", candidates[0].path, errSet)
	}
	return updated, nil
}

// AppendClaudePromptIDBillingText adds the field to a generated fallback
// billing text before it is installed and signed by the executor.
func AppendClaudePromptIDBillingText(text, promptID string) (string, error) {
	if errValidate := validateClaudePromptID(promptID); errValidate != nil {
		return "", fmt.Errorf("append Claude cc_prompt_id: %w", errValidate)
	}
	if _, found, errPrompt := ParseClaudePromptIDBillingText(text); errPrompt != nil {
		return "", errPrompt
	} else if found {
		return text, nil
	}
	return appendClaudePromptIDBillingText(text, promptID)
}

func appendClaudePromptIDBillingText(text, promptID string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "x-anthropic-billing-header:") {
		return text, nil
	}
	if _, _, errPrompt := ParseClaudePromptIDBillingText(trimmed); errPrompt != nil {
		return text, errPrompt
	}
	field := " cc_prompt_id=" + promptID + ";"
	updatedTrimmed := trimmed
	if !strings.HasSuffix(updatedTrimmed, ";") {
		updatedTrimmed += ";"
	}
	updatedTrimmed += field
	start := strings.Index(text, trimmed)
	if start < 0 {
		return text, fmt.Errorf("append Claude cc_prompt_id: invalid billing text")
	}
	return text[:start] + updatedTrimmed + text[start+len(trimmed):], nil
}

// BeginClaudePromptID resolves the prompt ID for one request. A non-tool
// message graph starts a new UUID, while an identical graph is treated as an
// idempotent retry. A tool-result continuation reuses the active ID. State is
// keyed by credential and session/agent scope, and caller-owned IDs always win.
func BeginClaudePromptID(credentialIdentity, sessionScope string, body []byte) (promptID string, generated bool, err error) {
	return beginClaudePromptID(credentialIdentity, sessionScope, body, false)
}

// BeginClaudePromptIDInherited resolves a subagent request against an already
// active parent prompt scope. It never allocates a new ID when the parent
// state is absent, so an orphan or reordered child remains unannotated.
func BeginClaudePromptIDInherited(credentialIdentity, sessionScope string, body []byte) (promptID string, generated bool, err error) {
	return beginClaudePromptID(credentialIdentity, sessionScope, body, true)
}

func beginClaudePromptID(credentialIdentity, sessionScope string, body []byte, inheritParent bool) (promptID string, generated bool, err error) {
	credentialIdentity = strings.TrimSpace(credentialIdentity)
	sessionScope = strings.TrimSpace(sessionScope)
	if credentialIdentity == "" || sessionScope == "" {
		return "", false, nil
	}
	callerID, callerFound, errCaller := ClaudePromptIDFromBody(body)
	if errCaller != nil {
		return "", false, errCaller
	}
	boundaryHash, continuation, ok := claudePromptBoundary(body)
	if !ok {
		return "", false, nil
	}
	digest := sha256.Sum256([]byte(credentialIdentity + "\x00" + sessionScope))
	key := hex.EncodeToString(digest[:])
	now := time.Now()

	claudePromptState.Lock()
	defer claudePromptState.Unlock()
	cleanupClaudePromptStateLocked(now)
	entry, found := claudePromptState.entries[key]
	if found && !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
		delete(claudePromptState.entries, key)
		entry = claudePromptStateEntry{}
		found = false
	}
	claudePromptState.nextAccess++
	access := claudePromptState.nextAccess
	if callerFound {
		entry.promptID = callerID
		entry.boundaryHash = boundaryHash
		entry.lastAccess = access
		entry.expiresAt = now.Add(claudePromptStateTTL)
		claudePromptState.entries[key] = entry
		return callerID, false, nil
	}
	if continuation {
		if !found || entry.promptID == "" {
			return "", false, nil
		}
		entry.lastAccess = access
		entry.expiresAt = now.Add(claudePromptStateTTL)
		claudePromptState.entries[key] = entry
		return entry.promptID, false, nil
	}
	if inheritParent {
		if !found || entry.promptID == "" {
			return "", false, nil
		}
		entry.lastAccess = access
		entry.expiresAt = now.Add(claudePromptStateTTL)
		claudePromptState.entries[key] = entry
		return entry.promptID, false, nil
	}
	if found && entry.promptID != "" && entry.boundaryHash == boundaryHash {
		entry.lastAccess = access
		entry.expiresAt = now.Add(claudePromptStateTTL)
		claudePromptState.entries[key] = entry
		return entry.promptID, false, nil
	}
	if !found && len(claudePromptState.entries) >= claudePromptStateMaxEntries {
		evictClaudePromptStateLocked()
	}
	newID, errUUID := uuid.NewRandom()
	if errUUID != nil {
		return "", false, fmt.Errorf("generate Claude cc_prompt_id: %w", errUUID)
	}
	entry = claudePromptStateEntry{
		promptID:     newID.String(),
		boundaryHash: boundaryHash,
		lastAccess:   access,
		expiresAt:    now.Add(claudePromptStateTTL),
	}
	claudePromptState.entries[key] = entry
	return entry.promptID, true, nil
}

func claudePromptBoundary(body []byte) (boundaryHash string, continuation, ok bool) {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() || len(messages.Array()) == 0 {
		return "", false, false
	}
	var lastUser gjson.Result
	for index := len(messages.Array()) - 1; index >= 0; index-- {
		candidate := messages.Array()[index]
		if candidate.Get("role").String() == "user" {
			lastUser = candidate
			break
		}
	}
	if !lastUser.Exists() {
		return "", false, false
	}
	content := lastUser.Get("content")
	if content.IsArray() {
		for _, block := range content.Array() {
			if block.Get("type").String() == "tool_result" {
				return hashClaudePromptBoundary(messages.Raw), true, true
			}
		}
	}
	return hashClaudePromptBoundary(messages.Raw), false, true
}

func hashClaudePromptBoundary(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func cleanupClaudePromptStateLocked(now time.Time) {
	if !claudePromptState.lastCleanup.IsZero() && now.Sub(claudePromptState.lastCleanup) < claudePromptStateCleanupPeriod {
		return
	}
	for key, entry := range claudePromptState.entries {
		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			delete(claudePromptState.entries, key)
		}
	}
	claudePromptState.lastCleanup = now
}

func evictClaudePromptStateLocked() {
	type candidate struct {
		key        string
		lastAccess uint64
	}
	candidates := make([]candidate, 0, len(claudePromptState.entries))
	for key, entry := range claudePromptState.entries {
		candidates = append(candidates, candidate{key: key, lastAccess: entry.lastAccess})
	}
	for count := 0; count < claudePromptStateEvictBatchSize && len(candidates) > 0; count++ {
		oldest := 0
		for index := 1; index < len(candidates); index++ {
			if candidates[index].lastAccess < candidates[oldest].lastAccess {
				oldest = index
			}
		}
		delete(claudePromptState.entries, candidates[oldest].key)
		candidates = append(candidates[:oldest], candidates[oldest+1:]...)
	}
}

func resetClaudePromptStateForTest() {
	claudePromptState.Lock()
	defer claudePromptState.Unlock()
	claudePromptState.entries = make(map[string]claudePromptStateEntry)
	claudePromptState.lastCleanup = time.Time{}
	claudePromptState.nextAccess = 0
}
