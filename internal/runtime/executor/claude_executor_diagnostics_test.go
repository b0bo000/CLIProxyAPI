package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/tidwall/gjson"
)

func TestInjectClaudeDiagnosticsMatchesNativeFieldOrderAndContinuity(t *testing.T) {
	t.Parallel()

	body := []byte(`{"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},"max_tokens":1,"messages":[]}`)
	testID := uuid.NewString()
	auth := &cliproxyauth.Auth{ID: "credential-diagnostics-order-" + testID}
	first, state := injectClaudeDiagnostics(body, auth, "session-diagnostics-order-"+testID)
	wantOrder := `"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},"diagnostics":{"previous_message_id":null},"max_tokens"`
	if !bytes.Contains(first, []byte(wantOrder)) {
		t.Fatalf("diagnostics field order differs from native: %s", first)
	}
	if got := gjson.GetBytes(first, "diagnostics.previous_message_id"); got.Type != gjson.Null {
		t.Fatalf("first previous_message_id = %s, want null", got.Raw)
	}

	commitClaudeDiagnostics(state, "msg_01ABCDEF0123456789ABCDEFG")
	second, _ := injectClaudeDiagnostics(body, auth, "session-diagnostics-order-"+testID)
	if got := gjson.GetBytes(second, "diagnostics.previous_message_id").String(); got != "msg_01ABCDEF0123456789ABCDEFG" {
		t.Fatalf("second previous_message_id = %q, want committed upstream ID", got)
	}
}

func TestClaudeExecutorDiagnosticsAdvancesAfterSuccessfulResponse(t *testing.T) {
	var previousValues []gjson.Result
	var betaValues []string
	call := 0
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			t.Fatal(errRead)
		}
		previousValues = append(previousValues, gjson.GetBytes(body, "diagnostics.previous_message_id"))
		betas := req.Header.Get("Anthropic-Beta")
		if betas == "" {
			betas = strings.Join(req.Header["anthropic-beta"], ",")
		}
		betaValues = append(betaValues, betas)
		call++
		response := `{"id":"msg_diagnostics_` + string(rune('0'+call)) + `","type":"message","model":"claude-opus-5","role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: req}, nil
	})
	ctx := context.WithValue(context.Background(), "cliproxy.roundtripper", http.RoundTripper(transport))
	testID := uuid.NewString()
	cfg, auth, request, options := claudePrevRequestFixture(t, "diagnostics-live-path-"+testID, uuid.NewString())
	// A confirmed native caller owns its beta list. Include the measured
	// diagnostics beta in the native fixture so the body/header pair remains
	// coherent while this test isolates call-site wiring.
	options.Headers.Set("Anthropic-Beta", options.Headers.Get("Anthropic-Beta")+","+claudeCacheDiagnosisBeta)
	executor := NewClaudeExecutor(cfg)
	for turn := range 2 {
		if _, errExecute := executor.Execute(ctx, auth, request, options); errExecute != nil {
			t.Fatalf("Execute() error = %v", errExecute)
		}
		if turn == 0 {
			auth.Attributes["api_key"] = "sk-ant-oat-diagnostics-live-path-rotated"
		}
	}
	if len(previousValues) != 2 || previousValues[0].Type != gjson.Null || previousValues[0].Raw != "null" {
		t.Fatalf("first diagnostics value = %#v, want explicit null", previousValues)
	}
	if got := previousValues[1].String(); got != "msg_diagnostics_1" {
		t.Fatalf("second diagnostics previous_message_id = %q, want first upstream response ID", got)
	}
	for turn, betas := range betaValues {
		for _, beta := range []string{claudeExtendedCacheTTLBeta, claudeCacheDiagnosisBeta} {
			if !strings.Contains(betas, beta) {
				t.Fatalf("turn %d Anthropic-Beta = %q, missing native diagnostics beta %q", turn+1, betas, beta)
			}
		}
	}
}

func TestClaudeExecutorDiagnosticsAdvancesAfterSuccessfulStream(t *testing.T) {
	var previousValues []gjson.Result
	call := 0
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			t.Fatal(errRead)
		}
		previousValues = append(previousValues, gjson.GetBytes(body, "diagnostics.previous_message_id"))
		call++
		stream := fmt.Sprintf("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_stream_%d\",\"model\":\"claude-opus-5\"}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", call)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "Request-Id": []string{fmt.Sprintf("req_stream_%d", call)}},
			Body:       io.NopCloser(strings.NewReader(stream)),
			Request:    req,
		}, nil
	})
	ctx := context.WithValue(context.Background(), "cliproxy.roundtripper", http.RoundTripper(transport))
	testID := uuid.NewString()
	cfg, auth, request, options := claudePrevRequestFixture(t, "diagnostics-stream-path-"+testID, uuid.NewString())
	request.Payload = []byte(strings.Replace(string(request.Payload), `"max_tokens":16}`, `"max_tokens":16,"stream":true}`, 1))
	options.OriginalRequest = request.Payload
	executor := NewClaudeExecutor(cfg)
	for turn := range 2 {
		result, errStream := executor.ExecuteStream(ctx, auth, request, options)
		if errStream != nil {
			t.Fatalf("ExecuteStream() turn %d error = %v", turn+1, errStream)
		}
		for chunk := range result.Chunks {
			if chunk.Err != nil {
				t.Fatalf("stream chunk error = %v", chunk.Err)
			}
		}
	}
	if len(previousValues) != 2 || previousValues[0].Type != gjson.Null || previousValues[0].Raw != "null" {
		t.Fatalf("first stream diagnostics value = %#v, want explicit null", previousValues)
	}
	if got := previousValues[1].String(); got != "msg_stream_1" {
		t.Fatalf("second stream diagnostics previous_message_id = %q, want first upstream message ID", got)
	}
}

func TestClaudeDiagnosticsEligibilityRequiresConfirmedNativeAnthropicRequest(t *testing.T) {
	confirmed := helps.ResolvedClaudeSoftwareProfile{Confirmed: true, Provenance: helps.ClaudeSoftwareProfileDetected}
	configured := helps.ResolvedClaudeSoftwareProfile{Confirmed: false, Provenance: helps.ClaudeSoftwareProfileConfiguredCLI}
	for _, tc := range []struct {
		name    string
		inject  bool
		baseURL string
		profile helps.ResolvedClaudeSoftwareProfile
		want    bool
	}{
		{name: "confirmed native Anthropic", inject: true, baseURL: "https://api.anthropic.com", profile: confirmed, want: true},
		{name: "configured but unconfirmed", inject: true, baseURL: "https://api.anthropic.com", profile: configured},
		{name: "custom upstream", inject: true, baseURL: "https://gateway.example", profile: confirmed},
		{name: "policy disabled", inject: false, baseURL: "https://api.anthropic.com", profile: confirmed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := claudeDiagnosticsEligible(tc.inject, tc.baseURL, tc.profile); got != tc.want {
				t.Fatalf("claudeDiagnosticsEligible() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBeginClaudeDiagnosticsPreservesCallerOwnedValueWithoutState(t *testing.T) {
	auth := &cliproxyauth.Auth{ID: "caller-owned-diagnostics"}
	profile := helps.ResolvedClaudeSoftwareProfile{Confirmed: true, Provenance: helps.ClaudeSoftwareProfileDetected}
	body := []byte(`{"diagnostics":{"previous_message_id":"caller_value"},"messages":[]}`)
	updated, state := beginClaudeDiagnostics(body, auth, "session-caller-owned", "https://api.anthropic.com", profile, true)
	if !bytes.Equal(updated, body) {
		t.Fatalf("caller-owned diagnostics body changed: got %s want %s", updated, body)
	}
	if state != (claudeDiagnosticsRequestState{}) {
		t.Fatalf("caller-owned diagnostics allocated state: %+v", state)
	}
}

func TestBeginClaudeDiagnosticsDoesNotCreateStateForUnconfirmedProfile(t *testing.T) {
	auth := &cliproxyauth.Auth{ID: "unconfirmed-diagnostics"}
	profile := helps.ResolvedClaudeSoftwareProfile{Provenance: helps.ClaudeSoftwareProfileConfiguredCLI}
	body := []byte(`{"messages":[]}`)
	updated, state := beginClaudeDiagnostics(body, auth, "session-unconfirmed", "https://api.anthropic.com", profile, true)
	if !bytes.Equal(updated, body) {
		t.Fatalf("unconfirmed diagnostics body changed: got %s want %s", updated, body)
	}
	if state != (claudeDiagnosticsRequestState{}) {
		t.Fatalf("unconfirmed profile allocated state: %+v", state)
	}
}

func TestClaudeMessageIDFromSSECommitsOnlyCompletedMessage(t *testing.T) {
	t.Parallel()

	complete := []byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_complete\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	if got := claudeMessageIDFromSSE(complete); got != "msg_complete" {
		t.Fatalf("completed SSE message ID = %q, want msg_complete", got)
	}
	incomplete := []byte(strings.Replace(string(complete), "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", "", 1))
	if got := claudeMessageIDFromSSE(incomplete); got != "" {
		t.Fatalf("incomplete SSE message ID = %q, want empty", got)
	}
}
