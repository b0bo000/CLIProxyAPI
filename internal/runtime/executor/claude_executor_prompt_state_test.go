package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	"github.com/tidwall/gjson"
)

func TestInsertClaudePrevRequestBillingPreservesCallerPromptID(t *testing.T) {
	t.Parallel()

	const promptID = "986813f9-290b-409e-8541-06cd11254627"
	body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cch=00000; cc_prompt_id=` + promptID + `;"}]}`)
	updated, errInsert := insertClaudePrevRequestBilling(body, "req_previous_123")
	if errInsert != nil {
		t.Fatalf("insertClaudePrevRequestBilling() error = %v", errInsert)
	}
	billing := gjson.GetBytes(updated, "system.0.text").String()
	if !strings.Contains(billing, "cc_prompt_id="+promptID+";") {
		t.Fatalf("prompt ID changed or disappeared: %q", billing)
	}
	if !strings.Contains(billing, "cc_prev_req=req_previous_123;") {
		t.Fatalf("previous request ID missing: %q", billing)
	}
	resigned, errSign := finalizeAnthropicMessagesBodyCCH(updated, "")
	if errSign != nil {
		t.Fatalf("finalizeAnthropicMessagesBodyCCH() error = %v", errSign)
	}
	resignedBilling := gjson.GetBytes(resigned, "system.0.text").String()
	if !strings.Contains(resignedBilling, "cc_prompt_id="+promptID+";") ||
		!strings.Contains(resignedBilling, "cc_prev_req=req_previous_123;") {
		t.Fatalf("CCH finalization changed continuity fields: %q", resignedBilling)
	}
}

func TestInsertClaudePrevRequestBillingRejectsMalformedCallerPromptID(t *testing.T) {
	t.Parallel()

	for _, promptID := range []string{"", "prompt;bad", "prompt=bad", "prompt bad"} {
		body := []byte(`{"system":"x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cch=00000; cc_prompt_id=` + promptID + `;"}`)
		updated, errInsert := insertClaudePrevRequestBilling(body, "req_previous_123")
		if errInsert == nil || updated != nil {
			t.Fatalf("prompt ID %q: body=%q error=%v, want fail-closed", promptID, updated, errInsert)
		}
	}
}

func TestClaudePromptIDGenerationExcludesResolvedTitleRequest(t *testing.T) {
	const titleInstruction = "You are naming a coding session so the user can pick it out of a long list of sessions."
	const userID = `{"device_id":"0000000000000000000000000000000000000000000000000000000000000000","account_uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","session_id":"11111111-2222-4333-8444-555555555555"}`
	headers := http.Header{
		"User-Agent":     {"claude-cli/2.1.241 (external, sdk-cli)"},
		"X-App":          {"cli"},
		"Anthropic-Beta": {"claude-code-20250219,context-1m-2025-08-07,interleaved-thinking-2025-05-14"},
	}
	encodedUserID, _ := json.Marshal(userID)
	payload := []byte(`{"model":"claude-opus-5","messages":[{"role":"user","content":[{"type":"text","text":"title me"}]}],"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241.37e; cc_entrypoint=sdk-cli;"},{"type":"text","text":"You are a Claude agent, built on Anthropic's Claude Agent SDK."},{"type":"text","text":"` + titleInstruction + ` Return JSON."}],"tools":[],"metadata":{"user_id":` + string(encodedUserID) + `},"thinking":{"type":"disabled"},"output_config":{"format":{"type":"json_schema","schema":{"type":"object","properties":{"title":{"type":"string"}},"required":["title"],"additionalProperties":false}}},"stream":true}`)
	cfg := &config.Config{ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{UserAgent: "claude-cli/2.1.241 (external, sdk-cli)"}}
	titleProfile, errTitle := helps.ResolveClaudeSoftwareProfile(context.Background(), nil, "", headers, payload, false, cfg, false)
	if errTitle != nil {
		t.Fatalf("title profile resolution error = %v", errTitle)
	}
	if !titleProfile.IsTitleRequest() {
		t.Fatalf("title profile = %#v, want title request", titleProfile)
	}
	if claudePromptIDGenerationEligible("https://api.anthropic.com", titleProfile, true) {
		t.Fatal("title request was eligible for generated prompt ID")
	}

	mainPayload := []byte(strings.Replace(string(payload), titleInstruction, "You are answering the user's request.", 1))
	mainProfile, errMain := helps.ResolveClaudeSoftwareProfile(context.Background(), nil, "", headers, mainPayload, false, cfg, false)
	if errMain != nil {
		t.Fatalf("main profile resolution error = %v", errMain)
	}
	if mainProfile.IsTitleRequest() || !claudePromptIDGenerationEligible("https://api.anthropic.com", mainProfile, true) {
		t.Fatalf("main profile = %#v, want prompt-ID eligibility", mainProfile)
	}
}

func TestClaudePromptIDRequestKindCompactionBoundary(t *testing.T) {
	const compactInstruction = `CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.

Your task is to create a detailed summary of the conversation so far.
<analysis>reason about the retained context</analysis>
<summary>preserve the important state</summary>`

	tests := []struct {
		name string
		body string
		want claudePromptIDRequestKind
	}{
		{
			name: "string content",
			body: `{"messages":[{"role":"user","content":"` + compactInstruction + `"}]}`,
			want: claudePromptIDRequestKindCompactionBoundary,
		},
		{
			name: "text block content",
			body: `{"messages":[{"role":"user","content":[{"type":"text","text":"` + compactInstruction + `"}]}]}`,
			want: claudePromptIDRequestKindCompactionBoundary,
		},
		{
			name: "historical compact text does not classify continuation",
			body: `{"messages":[{"role":"user","content":"` + compactInstruction + `"},{"role":"assistant","content":"summary complete"},{"role":"user","content":"continue normally"}]}`,
			want: claudePromptIDRequestKindMessages,
		},
		{
			name: "near miss",
			body: `{"messages":[{"role":"user","content":"CRITICAL: Respond with TEXT ONLY. Do NOT call any tools. Your task is to create a detailed summary of the conversation so far. <analysis>reason</analysis>"}]}`,
			want: claudePromptIDRequestKindMessages,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claudePromptIDRequestKindForBody([]byte(tt.body)); got != tt.want {
				t.Fatalf("request kind = %d, want %d", got, tt.want)
			}
		})
	}
}
