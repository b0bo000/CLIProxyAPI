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
	claudeauth "github.com/router-for-me/CLIProxyAPI/v7/internal/auth/claude"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
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

func TestClaudeDiagnosticsResponseMessageIDRequiresValidMessage(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "valid message", body: `{"id":"msg_valid","type":"message"}`, want: "msg_valid"},
		{name: "numeric id", body: `{"id":123,"type":"message"}`},
		{name: "boolean id", body: `{"id":true,"type":"message"}`},
		{name: "wrong type", body: `{"id":"msg_wrong_type","type":"error"}`},
		{name: "missing type", body: `{"id":"msg_missing_type"}`},
		{name: "empty id", body: `{"id":"  ","type":"message"}`},
		{name: "malformed JSON", body: `{"id":"msg_malformed","type":"message"`},
		{name: "array", body: `[{"id":"msg_array","type":"message"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := claudeMessageIDFromResponse([]byte(tc.body)); got != tc.want {
				t.Fatalf("claudeMessageIDFromResponse() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClaudeDiagnosticsSSERequiresCompleteSequence(t *testing.T) {
	valid := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_valid\",\"model\":\"claude-opus-5\"}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "complete", body: valid, want: "msg_valid"},
		{name: "missing delta", body: strings.Replace(valid, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n", "", 1)},
		{name: "missing stop", body: strings.Replace(valid, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", "", 1)},
		{name: "error event", body: strings.Replace(valid, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}", "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"no\"}}", 1)},
		{name: "duplicate start", body: strings.Replace(valid, "event: message_delta", "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_second\",\"model\":\"claude-opus-5\"}}\n\nevent: message_delta", 1)},
		{name: "after stop", body: valid + "event: ping\ndata: {}\n\n"},
		{name: "numeric id", body: strings.Replace(valid, `"id":"msg_valid"`, `"id":123`, 1)},
		{name: "missing model", body: strings.Replace(valid, `,"model":"claude-opus-5"`, "", 1)},
		{name: "malformed data", body: strings.Replace(valid, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`, `data: {"type":"message_delta","delta":`, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := claudeMessageIDFromSSE([]byte(tc.body)); got != tc.want {
				t.Fatalf("claudeMessageIDFromSSE() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClaudeExecutorDiagnosticsDoesNotAdvanceAfterInvalidJSONMessage(t *testing.T) {
	testID := uuid.NewString()
	cfg, auth, request, options := claudePrevRequestFixture(t, "diagnostics-invalid-json-"+testID, uuid.NewString())
	var bodies [][]byte
	call := 0
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			return nil, errRead
		}
		bodies = append(bodies, body)
		call++
		responseBody := `{"id":"msg_diag_seed","type":"message","model":"claude-opus-5","role":"assistant","content":[]}`
		if call == 2 {
			responseBody = `{"id":123,"type":"message","model":"claude-opus-5","role":"assistant","content":[]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(responseBody)), Request: req}, nil
	})
	executor := NewClaudeExecutor(cfg)
	for turn := 0; turn < 3; turn++ {
		if _, errExecute := executor.Execute(context.WithValue(context.Background(), "cliproxy.roundtripper", transport), auth, request, options); errExecute != nil {
			t.Fatalf("Execute() turn %d error = %v", turn+1, errExecute)
		}
	}
	if got := gjson.GetBytes(bodies[1], "diagnostics.previous_message_id").String(); got != "msg_diag_seed" {
		t.Fatalf("invalid response diagnostics previous_message_id = %q, want seed", got)
	}
	if got := gjson.GetBytes(bodies[2], "diagnostics.previous_message_id").String(); got != "msg_diag_seed" {
		t.Fatalf("post-invalid diagnostics previous_message_id = %q, want seed", got)
	}
}

func TestClaudeExecutorDiagnosticsStreamDoesNotAdvanceAfterIncompleteSSE(t *testing.T) {
	testID := uuid.NewString()
	cfg, auth, request, options := claudePrevRequestFixture(t, "diagnostics-incomplete-stream-"+testID, uuid.NewString())
	request.Payload = []byte(strings.Replace(string(request.Payload), `"max_tokens":16}`, `"max_tokens":16,"stream":true}`, 1))
	options.OriginalRequest = request.Payload
	valid := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_stream_seed\",\"model\":\"claude-opus-5\"}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	incomplete := strings.Replace(valid, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n", "", 1)
	var bodies [][]byte
	call := 0
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			return nil, errRead
		}
		bodies = append(bodies, body)
		call++
		stream := valid
		if call == 2 {
			stream = incomplete
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream)), Request: req}, nil
	})
	executor := NewClaudeExecutor(cfg)
	for turn := 0; turn < 3; turn++ {
		result, errStream := executor.ExecuteStream(context.WithValue(context.Background(), "cliproxy.roundtripper", transport), auth, request, options)
		if errStream != nil {
			t.Fatalf("ExecuteStream() turn %d error = %v", turn+1, errStream)
		}
		for chunk := range result.Chunks {
			if chunk.Err != nil {
				t.Fatalf("stream chunk error = %v", chunk.Err)
			}
		}
	}
	if got := gjson.GetBytes(bodies[1], "diagnostics.previous_message_id").String(); got != "msg_stream_seed" {
		t.Fatalf("incomplete stream diagnostics previous_message_id = %q, want seed", got)
	}
	if got := gjson.GetBytes(bodies[2], "diagnostics.previous_message_id").String(); got != "msg_stream_seed" {
		t.Fatalf("post-incomplete diagnostics previous_message_id = %q, want seed", got)
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

	complete := []byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_complete\",\"model\":\"claude-opus-5\"}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	if got := claudeMessageIDFromSSE(complete); got != "msg_complete" {
		t.Fatalf("completed SSE message ID = %q, want msg_complete", got)
	}
	incomplete := []byte(strings.Replace(string(complete), "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", "", 1))
	if got := claudeMessageIDFromSSE(incomplete); got != "" {
		t.Fatalf("incomplete SSE message ID = %q, want empty", got)
	}
}

func TestClaudeExecutorContinuityAdvancesRequestIDAndPromptIDInBillingHeader(t *testing.T) {
	var capturedBillingHeaders []string
	call := 0
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			t.Fatal(errRead)
		}
		billing := gjson.GetBytes(body, "system.0.text").String()
		capturedBillingHeaders = append(capturedBillingHeaders, billing)
		call++
		response := `{"id":"msg_turn_` + string(rune('0'+call)) + `","type":"message","model":"claude-sonnet-5","role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`
		header := http.Header{
			"Content-Type": []string{"application/json"},
			"request-id":   []string{fmt.Sprintf("req_upstream_turn_%d", call)},
		}
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(response)), Request: req}, nil
	})
	ctx := context.WithValue(context.Background(), "cliproxy.roundtripper", http.RoundTripper(transport))
	deviceIDs := []string{"0000000000000000000000000000000000000000000000000000000000000000"}
	testID := uuid.NewString()
	auth := &cliproxyauth.Auth{
		ID:         "continuity-test-" + testID,
		Attributes: map[string]string{"api_key": "sk-ant-oat-continuity-test"},
		Metadata: map[string]any{
			"account_uuid":                        "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			claudeauth.ClaudeDeviceIDsMetadataKey: deviceIDs,
		},
	}
	executor := NewClaudeExecutor(&config.Config{})
	options := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FormatClaude,
		Metadata:     map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "continuity-conv-" + testID},
	}

	// Turn 1: User prompt
	req1 := cliproxyexecutor.Request{Model: "claude-sonnet-5", Payload: []byte(`{"model":"claude-sonnet-5","messages":[{"role":"user","content":"turn 1 prompt"}],"max_tokens":100}`)}
	if _, err := executor.Execute(ctx, auth, req1, options); err != nil {
		t.Fatalf("turn 1 failed: %v", err)
	}

	// Turn 2: User prompt in same conversation
	req2 := cliproxyexecutor.Request{Model: "claude-sonnet-5", Payload: []byte(`{"model":"claude-sonnet-5","messages":[{"role":"user","content":"turn 1 prompt"},{"role":"assistant","content":"ok"},{"role":"user","content":"turn 2 prompt"}],"max_tokens":100}`)}
	if _, err := executor.Execute(ctx, auth, req2, options); err != nil {
		t.Fatalf("turn 2 failed: %v", err)
	}

	// Turn 2.1: Tool result continuation within turn 2
	req2Tool := cliproxyexecutor.Request{Model: "claude-sonnet-5", Payload: []byte(`{"model":"claude-sonnet-5","messages":[{"role":"user","content":"turn 1 prompt"},{"role":"assistant","content":"ok"},{"role":"user","content":"turn 2 prompt"},{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"bash","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"tool output"}]}],"max_tokens":100}`)}
	if _, err := executor.Execute(ctx, auth, req2Tool, options); err != nil {
		t.Fatalf("turn 2.1 tool failed: %v", err)
	}

	// Turn 3: Probe request (max_tokens: 1)
	reqProbe := cliproxyexecutor.Request{Model: "claude-sonnet-5", Payload: []byte(`{"model":"claude-sonnet-5","messages":[{"role":"user","content":"probe"}],"max_tokens":1}`)}
	if _, err := executor.Execute(ctx, auth, reqProbe, options); err != nil {
		t.Fatalf("probe failed: %v", err)
	}

	// Turn 4: Subagent request (carrying X-Claude-Code-Agent-Id)
	subagentOptions := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FormatClaude,
		Headers:      http.Header{"X-Claude-Code-Agent-Id": []string{"subagent-worker-1"}},
		Metadata:     map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "continuity-conv-" + testID},
	}
	reqSubagent := cliproxyexecutor.Request{Model: "claude-sonnet-5", Payload: []byte(`{"model":"claude-sonnet-5","system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.258.test; cc_entrypoint=cli; cc_is_subagent=true;"}],"messages":[{"role":"user","content":"subagent prompt"}],"max_tokens":100}`)}
	if _, err := executor.Execute(ctx, auth, reqSubagent, subagentOptions); err != nil {
		t.Fatalf("subagent failed: %v", err)
	}

	// Turn 5: Resumed main session user prompt after probe (must chain from turn 2.1, NOT from probe turn 3)
	req5 := cliproxyexecutor.Request{Model: "claude-sonnet-5", Payload: []byte(`{"model":"claude-sonnet-5","messages":[{"role":"user","content":"turn 5 prompt"}],"max_tokens":100}`)}
	if _, err := executor.Execute(ctx, auth, req5, options); err != nil {
		t.Fatalf("turn 5 failed: %v", err)
	}

	if len(capturedBillingHeaders) != 6 {
		t.Fatalf("captured %d billing headers, want 6", len(capturedBillingHeaders))
	}

	// Verify Turn 1:
	h1 := capturedBillingHeaders[0]
	if !strings.Contains(h1, "cc_version=2.1.258.") || !strings.Contains(h1, "cc_entrypoint=cli;") || !strings.Contains(h1, "cch=") {
		t.Fatalf("h1 invalid: %s", h1)
	}
	if strings.Contains(h1, "cc_prev_req=") {
		t.Fatalf("h1 must not contain cc_prev_req: %s", h1)
	}
	if !strings.Contains(h1, "cc_prompt_id=") {
		t.Fatalf("h1 must contain cc_prompt_id: %s", h1)
	}
	prompt1 := extractTag(h1, "cc_prompt_id=")

	// Verify Turn 2:
	h2 := capturedBillingHeaders[1]
	if !strings.Contains(h2, "cc_prev_req=req_upstream_turn_1;") {
		t.Fatalf("h2 must contain cc_prev_req=req_upstream_turn_1;, got: %s", h2)
	}
	if !strings.Contains(h2, "cc_prompt_id=") {
		t.Fatalf("h2 must contain cc_prompt_id: %s", h2)
	}
	prompt2 := extractTag(h2, "cc_prompt_id=")
	if prompt2 == prompt1 {
		t.Fatalf("h2 promptID (%s) must differ from h1 promptID (%s)", prompt2, prompt1)
	}

	// Verify Turn 2.1 (tool continuation):
	h2Tool := capturedBillingHeaders[2]
	if !strings.Contains(h2Tool, "cc_prev_req=req_upstream_turn_2;") {
		t.Fatalf("h2Tool must contain cc_prev_req=req_upstream_turn_2;, got: %s", h2Tool)
	}
	prompt2Tool := extractTag(h2Tool, "cc_prompt_id=")
	if prompt2Tool != prompt2 {
		t.Fatalf("h2Tool promptID (%s) must match turn 2 promptID (%s)", prompt2Tool, prompt2)
	}

	// Verify Turn 3 (probe with max_tokens: 1):
	hProbe := capturedBillingHeaders[3]
	if strings.Contains(hProbe, "cc_prev_req=") || strings.Contains(hProbe, "cc_prompt_id=") {
		t.Fatalf("probe must not contain cc_prev_req or cc_prompt_id: %s", hProbe)
	}

	// Verify Turn 4 (subagent):
	hSubagent := capturedBillingHeaders[4]
	if !strings.Contains(hSubagent, "cc_is_subagent=true;") {
		t.Fatalf("hSubagent must contain cc_is_subagent=true;, got: %s", hSubagent)
	}
	if !strings.Contains(hSubagent, "cc_prompt_id=") {
		t.Fatalf("hSubagent must contain cc_prompt_id: %s", hSubagent)
	}

	// Verify Turn 5 (main turn after probe):
	// Must chain to turn 2.1's response (req_upstream_turn_3), bypassing probe turn 3 (req_upstream_turn_4)!
	h5 := capturedBillingHeaders[5]
	if !strings.Contains(h5, "cc_prev_req=req_upstream_turn_3;") {
		t.Fatalf("h5 must contain cc_prev_req=req_upstream_turn_3; (bypassing probe turn 4), got: %s", h5)
	}
	if !strings.Contains(h5, "cc_prompt_id=") {
		t.Fatalf("h5 must contain cc_prompt_id: %s", h5)
	}
}

func extractTag(header, prefix string) string {
	idx := strings.Index(header, prefix)
	if idx < 0 {
		return ""
	}
	val := header[idx+len(prefix):]
	if end := strings.IndexByte(val, ';'); end >= 0 {
		val = val[:end]
	}
	return val
}
