package executor

import (
	"bytes"
	"context"
	"errors"
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

type claudePrevRequestReadErrorBody struct{}

func (claudePrevRequestReadErrorBody) Read([]byte) (int, error) {
	return 0, errors.New("synthetic response read failure")
}

func (claudePrevRequestReadErrorBody) Close() error { return nil }

type claudePrevRequestCancelOnEOFBody struct {
	reader io.Reader
	cancel context.CancelFunc
}

func (b *claudePrevRequestCancelOnEOFBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	if err == io.EOF && b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	return n, err
}

func (b *claudePrevRequestCancelOnEOFBody) Close() error { return nil }

func claudePrevRequestFixture(t *testing.T, credentialID, sessionID string) (*config.Config, *cliproxyauth.Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) {
	t.Helper()
	const deviceID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const accountID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	userID := fmt.Sprintf(`{"device_id":%q,"account_uuid":%q,"session_id":%q}`, deviceID, accountID, sessionID)
	payload := []byte(fmt.Sprintf(`{"model":"claude-opus-5","system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241.test; cc_entrypoint=sdk-cli; cch=00000;"},{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"test"}],"tools":[],"metadata":{"user_id":%q},"max_tokens":16}`, userID))
	headers := http.Header{
		"User-Agent":                  {"claude-cli/2.1.241 (external, sdk-cli)"},
		"X-App":                       {"cli"},
		"Anthropic-Beta":              {"claude-code-20250219,interleaved-thinking-2025-05-14"},
		"X-Claude-Code-Session-Id":    {sessionID},
		"X-Stainless-Package-Version": {"0.112.1"},
		"X-Stainless-Runtime-Version": {"v26.3.0"},
		"X-Stainless-Os":              {"Windows"},
		"X-Stainless-Arch":            {"x64"},
	}
	cfg := &config.Config{ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
		UserAgent:      "claude-cli/2.1.241 (external, sdk-cli)",
		PackageVersion: "0.112.1",
		RuntimeVersion: "v26.3.0",
		OS:             "Windows",
		Arch:           "x64",
	}}
	auth := &cliproxyauth.Auth{
		ID:         credentialID,
		Attributes: map[string]string{"api_key": "sk-ant-oat-s4a3-synthetic"},
		Metadata: map[string]any{
			"account_uuid":                        accountID,
			claudeauth.ClaudeDeviceIDsMetadataKey: []string{deviceID},
		},
	}
	request := cliproxyexecutor.Request{Model: "claude-opus-5", Payload: payload}
	options := cliproxyexecutor.Options{
		SourceFormat:    sdktranslator.FormatClaude,
		OriginalRequest: payload,
		Headers:         headers,
		Metadata:        map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: sessionID},
	}
	return cfg, auth, request, options
}

func claudePrevRequestExecute(t *testing.T, transport http.RoundTripper, executor *ClaudeExecutor, auth *cliproxyauth.Auth, request cliproxyexecutor.Request, options cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	t.Helper()
	ctx := context.WithValue(context.Background(), "cliproxy.roundtripper", transport)
	return executor.Execute(ctx, auth, request, options)
}

func claudePrevRequestValue(body []byte) string {
	text := gjson.GetBytes(body, "system.0.text").String()
	for _, segment := range strings.Split(strings.TrimPrefix(text, claudeBillingHeaderPrefix), ";") {
		key, value, found := strings.Cut(strings.TrimSpace(segment), "=")
		if found && strings.TrimSpace(key) == "cc_prev_req" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func claudePrevRequestSuccessResponse(req *http.Request, requestID string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": {"application/json"},
			"Request-Id":   {requestID},
		},
		Body:    io.NopCloser(strings.NewReader(`{"id":"msg_s4a3","type":"message","model":"claude-opus-5","role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)),
		Request: req,
	}
}

func TestClaudeExecutorPrevRequestExecuteAdvancesOnlyAfterSuccess(t *testing.T) {
	sessionID := uuid.NewString()
	cfg, auth, request, options := claudePrevRequestFixture(t, "s4a3-success-"+uuid.NewString(), sessionID)
	var bodies [][]byte
	requestIDs := []string{"req_s4a3_first", "req_s4a3_second", "req_s4a3_third"}
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			t.Fatal(errRead)
		}
		bodies = append(bodies, body)
		return claudePrevRequestSuccessResponse(req, requestIDs[len(bodies)-1]), nil
	})
	executor := NewClaudeExecutor(cfg)
	for range 3 {
		if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
			t.Fatalf("Execute() error = %v", errExecute)
		}
	}
	got := []string{claudePrevRequestValue(bodies[0]), claudePrevRequestValue(bodies[1]), claudePrevRequestValue(bodies[2])}
	want := []string{"", requestIDs[0], requestIDs[1]}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("request %d cc_prev_req = %q, want %q; body=%s", index+1, got[index], want[index], bodies[index])
		}
	}
	for index := 1; index < len(bodies); index++ {
		resigned, errSign := finalizeAnthropicMessagesBodyCCH(bodies[index], "")
		if errSign != nil {
			t.Fatalf("request %d re-sign error = %v", index+1, errSign)
		}
		if !bytes.Equal(resigned, bodies[index]) {
			t.Fatalf("request %d CCH does not cover cc_prev_req", index+1)
		}
	}
}

func TestClaudeExecutorPrevRequestPolicyGateIsIndependentFromDiagnostics(t *testing.T) {
	_, auth, request, _ := claudePrevRequestFixture(t, "s4a5-policy-"+uuid.NewString(), uuid.NewString())
	profile := helps.ResolvedClaudeSoftwareProfile{Confirmed: true, Provenance: helps.ClaudeSoftwareProfileDetected}
	policy := resolveClaudeFingerprintPolicy(nil, auth, auth.Attributes["api_key"])
	if !policy.InjectDiagnostics || !policy.InjectPrevRequest {
		t.Fatalf("OAuth policy = %+v, want both independent policy results enabled", policy)
	}

	// Simulate diagnostics remaining enabled while the prev-request policy is
	// disabled. No probe, state generation, or body rewrite is allowed.
	updated, state, errPrepare := beginClaudePrevRequestExecute(
		request.Payload,
		auth,
		auth.Attributes["api_key"],
		"claude:session:agent:main",
		"https://api.anthropic.com",
		profile,
		false,
		false,
	)
	if errPrepare != nil {
		t.Fatalf("disabled begin() error = %v", errPrepare)
	}
	if !bytes.Equal(updated, request.Payload) {
		t.Fatalf("disabled policy rewrote body: %s", updated)
	}
	if state != (claudePrevRequestState{}) {
		t.Fatalf("disabled policy allocated state: %+v", state)
	}

	// Caller-owned continuity remains untouched even when the gate is disabled.
	callerBody := bytes.Replace(request.Payload, []byte(" cch=00000;"), []byte(" cch=00000; cc_prev_req=req_caller_owned;"), 1)
	updated, state, errPrepare = beginClaudePrevRequestExecute(
		callerBody,
		auth,
		auth.Attributes["api_key"],
		"claude:session:agent:main",
		"https://api.anthropic.com",
		profile,
		false,
		false,
	)
	if errPrepare != nil {
		t.Fatalf("disabled caller-owned begin() error = %v", errPrepare)
	}
	if !bytes.Equal(updated, callerBody) || state != (claudePrevRequestState{}) {
		t.Fatalf("disabled policy changed caller-owned request or state: body=%s state=%+v", updated, state)
	}
}

func TestClaudeExecutorPrevRequestExecuteFailuresDoNotAdvance(t *testing.T) {
	tests := []struct {
		name     string
		response func(*http.Request) (*http.Response, error)
		wantErr  bool
	}{
		{
			name:    "upstream 400",
			wantErr: true,
			response: func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"failure"}}`)), Request: req}, nil
			},
		},
		{
			name:    "upstream 500",
			wantErr: true,
			response: func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusInternalServerError, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"failure"}}`)), Request: req}, nil
			},
		},
		{
			name:    "transport cancellation",
			wantErr: true,
			response: func(*http.Request) (*http.Response, error) {
				return nil, context.Canceled
			},
		},
		{
			name:    "response read error",
			wantErr: true,
			response: func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}, "Request-Id": {"req_must_not_commit"}}, Body: claudePrevRequestReadErrorBody{}, Request: req}, nil
			},
		},
		{
			name: "invalid JSON",
			response: func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}, "Request-Id": {"req_must_not_commit"}}, Body: io.NopCloser(strings.NewReader(`not-json`)), Request: req}, nil
			},
		},
		{
			name: "non-message JSON",
			response: func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}, "Request-Id": {"req_must_not_commit"}}, Body: io.NopCloser(strings.NewReader(`{"type":"error","id":"msg_not_success"}`)), Request: req}, nil
			},
		},
		{
			name: "missing request ID",
			response: func(req *http.Request) (*http.Response, error) {
				response := claudePrevRequestSuccessResponse(req, "req_unused")
				response.Header.Del("Request-Id")
				return response, nil
			},
		},
		{
			name: "invalid request ID",
			response: func(req *http.Request) (*http.Response, error) {
				return claudePrevRequestSuccessResponse(req, "req_invalid;value"), nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, auth, request, options := claudePrevRequestFixture(t, "s4a3-failure-"+uuid.NewString(), uuid.NewString())
			var bodies [][]byte
			call := 0
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				body, errRead := io.ReadAll(req.Body)
				if errRead != nil {
					t.Fatal(errRead)
				}
				bodies = append(bodies, body)
				call++
				switch call {
				case 1:
					return claudePrevRequestSuccessResponse(req, "req_seed"), nil
				case 2:
					return test.response(req)
				default:
					return claudePrevRequestSuccessResponse(req, "req_after_failure"), nil
				}
			})
			executor := NewClaudeExecutor(cfg)
			if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
				t.Fatalf("seed Execute() error = %v", errExecute)
			}
			_, errFailure := claudePrevRequestExecute(t, transport, executor, auth, request, options)
			if test.wantErr && errFailure == nil {
				t.Fatalf("failure Execute() error = nil, want failure")
			}
			if !test.wantErr && errFailure != nil {
				t.Fatalf("non-committing Execute() error = %v, want no error", errFailure)
			}
			if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
				t.Fatalf("post-failure Execute() error = %v", errExecute)
			}
			if len(bodies) != 3 {
				t.Fatalf("request count = %d, want 3", len(bodies))
			}
			if got := claudePrevRequestValue(bodies[2]); got != "req_seed" {
				t.Fatalf("post-failure cc_prev_req = %q, want req_seed", got)
			}
		})
	}
}

func TestClaudeExecutorPrevRequestExecuteSeparatesAgentScopes(t *testing.T) {
	sessionID := uuid.NewString()
	cfg, auth, request, options := claudePrevRequestFixture(t, "s4a3-agent-"+uuid.NewString(), sessionID)
	var bodies [][]byte
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			t.Fatal(errRead)
		}
		bodies = append(bodies, body)
		return claudePrevRequestSuccessResponse(req, fmt.Sprintf("req_agent_%d", len(bodies))), nil
	})
	executor := NewClaudeExecutor(cfg)
	parentOptions := options
	parentOptions.Headers = options.Headers.Clone()
	parentOptions.Headers.Set("X-Claude-Code-Agent-Id", "main")
	childOptions := options
	childOptions.Headers = options.Headers.Clone()
	childOptions.Headers.Set("X-Claude-Code-Agent-Id", "subagent-1")
	for _, turn := range []cliproxyexecutor.Options{parentOptions, childOptions, parentOptions} {
		if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, turn); errExecute != nil {
			t.Fatalf("Execute() error = %v", errExecute)
		}
	}
	want := []string{"", "", "req_agent_1"}
	for index, expected := range want {
		if got := claudePrevRequestValue(bodies[index]); got != expected {
			t.Fatalf("request %d cc_prev_req = %q, want %q", index+1, got, expected)
		}
	}
}

func TestClaudeExecutorPrevRequestExecuteCancellationAfterBodyDoesNotCommit(t *testing.T) {
	sessionID := uuid.NewString()
	cfg, auth, request, options := claudePrevRequestFixture(t, "s4a3-cancel-"+uuid.NewString(), sessionID)
	var bodies [][]byte
	call := 0
	var cancel context.CancelFunc
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			t.Fatal(errRead)
		}
		bodies = append(bodies, body)
		call++
		if call == 2 {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}, "Request-Id": {"req_cancelled"}},
				Body:       &claudePrevRequestCancelOnEOFBody{reader: strings.NewReader(`{"id":"msg_cancelled","type":"message","content":[]}`), cancel: cancel},
				Request:    req,
			}, nil
		}
		return claudePrevRequestSuccessResponse(req, fmt.Sprintf("req_cancel_%d", call)), nil
	})
	executor := NewClaudeExecutor(cfg)
	if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
		t.Fatalf("seed Execute() error = %v", errExecute)
	}
	ctx, cancelFn := context.WithCancel(context.Background())
	cancel = cancelFn
	ctx = context.WithValue(ctx, "cliproxy.roundtripper", transport)
	if _, errExecute := executor.Execute(ctx, auth, request, options); errExecute != nil {
		t.Fatalf("cancel-after-body Execute() error = %v", errExecute)
	}
	if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
		t.Fatalf("post-cancellation Execute() error = %v", errExecute)
	}
	if got := claudePrevRequestValue(bodies[2]); got != "req_cancel_1" {
		t.Fatalf("post-cancellation cc_prev_req = %q, want seed request ID", got)
	}
}

func TestClaudeExecutorPrevRequestIDHeaderRequiresOneBoundedASCIIValue(t *testing.T) {
	for _, test := range []struct {
		name    string
		headers http.Header
		wantErr bool
	}{
		{name: "one value", headers: http.Header{"Request-Id": {"req_valid"}}},
		{name: "duplicate values", headers: http.Header{"Request-Id": {"req_first", "req_second"}}, wantErr: true},
		{name: "duplicate case variants", headers: http.Header{"Request-Id": {"req_first"}, "request-id": {"req_second"}}, wantErr: true},
		{name: "non ascii", headers: http.Header{"Request-Id": {"req-é"}}, wantErr: true},
		{name: "combined list value", headers: http.Header{"Request-Id": {"req_first, req_second"}}, wantErr: true},
		{name: "suffix punctuation", headers: http.Header{"Request-Id": {"req_first-second"}}, wantErr: true},
		{name: "missing req prefix", headers: http.Header{"Request-Id": {"id_first"}}, wantErr: true},
		{name: "too long", headers: http.Header{"Request-Id": {"req_" + strings.Repeat("r", 125)}}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, errHeader := claudePrevRequestIDHeader(test.headers)
			if test.wantErr {
				if errHeader == nil {
					t.Fatalf("header value = %q, error = nil", got)
				}
				return
			}
			if errHeader != nil || got != "req_valid" {
				t.Fatalf("header = %q, error = %v", got, errHeader)
			}
		})
	}
}

func TestClaudeExecutorPrevRequestCommitRejectsNonStringMessageID(t *testing.T) {
	credentialID := "s4a3-message-id-" + uuid.NewString()
	sessionScope := "s4a3-session-" + uuid.NewString()
	key, sequence, _ := helps.BeginClaudePrevRequest(credentialID, sessionScope)
	state := claudePrevRequestState{key: key, sequence: sequence}

	for _, upstreamBody := range [][]byte{
		[]byte(`{"type":"message","id":123}`),
		[]byte(`{"type":"message","id":true}`),
		[]byte(`{"type":"message","id":{"value":"msg"}}`),
	} {
		commitClaudePrevRequestExecute(context.Background(), state, http.Header{"Request-Id": {"req_must_not_commit"}}, upstreamBody, upstreamBody)
	}
	_, _, previous := helps.BeginClaudePrevRequest(credentialID, sessionScope)
	if previous != "" {
		t.Fatalf("previous request after non-string message id = %q, want empty", previous)
	}
}

func TestClaudeExecutorPrevRequestMalformedBillingIsRequestScoped(t *testing.T) {
	_, auth, request, _ := claudePrevRequestFixture(t, "s4a3-malformed-"+uuid.NewString(), uuid.NewString())
	request.Payload = bytes.Replace(request.Payload, []byte("cc_entrypoint=sdk-cli"), []byte("cc_entrypoint"), 1)
	profile := helps.ResolvedClaudeSoftwareProfile{Confirmed: true, Provenance: helps.ClaudeSoftwareProfileDetected}
	_, _, errPrepare := beginClaudePrevRequestExecute(request.Payload, auth, auth.Attributes["api_key"], "claude:session:agent:main", "https://api.anthropic.com", profile, false, true)
	if errPrepare == nil {
		t.Fatal("malformed billing error = nil")
	}
	var status interface{ StatusCode() int }
	if !errors.As(errPrepare, &status) || status.StatusCode() != http.StatusBadRequest {
		t.Fatalf("error = %T %v, want HTTP 400", errPrepare, errPrepare)
	}
	var scoped interface{ IsRequestScoped() bool }
	if !errors.As(errPrepare, &scoped) || !scoped.IsRequestScoped() {
		t.Fatalf("error = %T %v, want request-scoped", errPrepare, errPrepare)
	}
}

func TestClaudeExecutorPrevRequestExecuteSupportsConfirmedAPIKeyCaller(t *testing.T) {
	sessionID := uuid.NewString()
	cfg, auth, request, options := claudePrevRequestFixture(t, "s4a3-api-key-"+uuid.NewString(), sessionID)
	auth.Attributes["api_key"] = "sk-ant-api03-s4a3-synthetic"
	delete(auth.Metadata, "account_uuid")
	var bodies [][]byte
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			t.Fatal(errRead)
		}
		bodies = append(bodies, body)
		return claudePrevRequestSuccessResponse(req, fmt.Sprintf("req_api_key_%d", len(bodies))), nil
	})
	executor := NewClaudeExecutor(cfg)
	for range 2 {
		if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
			t.Fatalf("Execute() error = %v", errExecute)
		}
	}
	if got := claudePrevRequestValue(bodies[1]); got != "req_api_key_1" {
		t.Fatalf("second API-key cc_prev_req = %q, want req_api_key_1", got)
	}
}

func TestClaudePrevRequestCredentialIdentitySeparatesAccountHotSwap(t *testing.T) {
	auth := &cliproxyauth.Auth{ID: "stable-record", Metadata: map[string]any{"account_uuid": "account-a"}}
	first := claudePrevRequestCredentialIdentity(auth, "sk-ant-oat-synthetic")
	auth.Metadata["account_uuid"] = "account-b"
	second := claudePrevRequestCredentialIdentity(auth, "sk-ant-oat-synthetic")
	if first == "" || second == "" || first == second {
		t.Fatalf("credential identities = %q and %q, want distinct non-empty values", first, second)
	}
}

func TestClaudePrevRequestCredentialIdentityAPIKeyIgnoresStaleAccountUUID(t *testing.T) {
	auth := &cliproxyauth.Auth{
		ID:         "stable-api-key-record",
		Attributes: map[string]string{cliproxyauth.AttributeAPIKey: "key-a"},
		Metadata:   map[string]any{"account_uuid": "stale-oauth-account"},
	}
	first := claudePrevRequestCredentialIdentity(auth, "key-a")
	auth.Metadata["account_uuid"] = "another-stale-account"
	second := claudePrevRequestCredentialIdentity(auth, "key-a")
	if first == "" || second == "" || first != second {
		t.Fatalf("API-key identities = %q and %q, want same key digest despite stale account UUID", first, second)
	}
}

func TestClaudePrevRequestCredentialIdentityAPIKeyHotSwapStartsNewChain(t *testing.T) {
	auth := &cliproxyauth.Auth{
		ID:         "stable-api-key-record",
		Attributes: map[string]string{cliproxyauth.AttributeAPIKey: "key-a"},
		Metadata:   map[string]any{"account_uuid": "stale-oauth-account"},
	}
	first := claudePrevRequestCredentialIdentity(auth, "key-a")
	second := claudePrevRequestCredentialIdentity(auth, "key-b")
	if first == "" || second == "" || first == second {
		t.Fatalf("API-key identities = %q and %q, want distinct key digests", first, second)
	}
}

func TestClaudePrevRequestCredentialIdentityOAuthTokenRotationKeepsAccountChain(t *testing.T) {
	auth := &cliproxyauth.Auth{
		ID:         "stable-oauth-record",
		Attributes: map[string]string{cliproxyauth.AttributeAuthKind: cliproxyauth.AuthKindOAuth},
		Metadata:   map[string]any{"account_uuid": "oauth-account"},
	}
	first := claudePrevRequestCredentialIdentity(auth, "sk-ant-oat-token-a")
	second := claudePrevRequestCredentialIdentity(auth, "sk-ant-oat-token-b")
	if first == "" || second == "" || first != second {
		t.Fatalf("OAuth identities = %q and %q, want same account chain across token rotation", first, second)
	}
}

func TestClaudeExecutorPrevRequestExecuteEligibilityControls(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241; cc_entrypoint=sdk-cli; cch=00000;"}]}`)
	auth := &cliproxyauth.Auth{ID: "eligibility-record", Metadata: map[string]any{"account_uuid": "eligibility-account"}}
	profile := helps.ResolvedClaudeSoftwareProfile{Confirmed: true, Provenance: helps.ClaudeSoftwareProfileDetected}
	for _, test := range []struct {
		name      string
		profile   helps.ResolvedClaudeSoftwareProfile
		stream    bool
		baseURL   string
		scope     string
		wantState bool
	}{
		{name: "upstream stream excluded", profile: profile, stream: true, baseURL: "https://api.anthropic.com", scope: "claude:s:agent:main"},
		{name: "custom upstream excluded", profile: profile, baseURL: "https://custom.example", scope: "claude:s:agent:main"},
		{name: "unconfirmed excluded", profile: helps.ResolvedClaudeSoftwareProfile{Provenance: helps.ClaudeSoftwareProfileUnknown}, baseURL: "https://api.anthropic.com", scope: "claude:s:agent:main"},
		{name: "missing scope excluded", profile: profile, baseURL: "https://api.anthropic.com"},
	} {
		t.Run(test.name, func(t *testing.T) {
			updated, state, errPrepare := beginClaudePrevRequestExecute(body, auth, "sk-ant-oat-s4a3", test.scope, test.baseURL, test.profile, test.stream, true)
			if errPrepare != nil {
				t.Fatalf("begin() error = %v", errPrepare)
			}
			if !bytes.Equal(updated, body) {
				t.Fatalf("ineligible body changed: %s", updated)
			}
			if (state.key != "") != test.wantState {
				t.Fatalf("state key present = %v, want %v", state.key != "", test.wantState)
			}
		})
	}
}

func TestClaudeExecutorPrevRequestExecutePreservesCallerOwnership(t *testing.T) {
	cfg, auth, request, options := claudePrevRequestFixture(t, "s4a3-caller-"+uuid.NewString(), uuid.NewString())
	callerPayload := bytes.Replace(request.Payload, []byte(" cch=00000;"), []byte(" cch=00000; cc_prev_req=req_caller_owned;"), 1)
	request.Payload = callerPayload
	options.OriginalRequest = callerPayload
	var bodies [][]byte
	ids := []string{"req_caller_response", "req_first_managed", "req_second_managed"}
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		bodies = append(bodies, body)
		return claudePrevRequestSuccessResponse(req, ids[len(bodies)-1]), nil
	})
	executor := NewClaudeExecutor(cfg)
	if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
		t.Fatalf("caller-owned Execute() error = %v", errExecute)
	}
	_, _, managedRequest, managedOptions := claudePrevRequestFixture(t, auth.ID, options.Metadata[cliproxyexecutor.ExecutionSessionMetadataKey].(string))
	for range 2 {
		if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, managedRequest, managedOptions); errExecute != nil {
			t.Fatalf("managed Execute() error = %v", errExecute)
		}
	}
	got := []string{claudePrevRequestValue(bodies[0]), claudePrevRequestValue(bodies[1]), claudePrevRequestValue(bodies[2])}
	want := []string{"req_caller_owned", "", "req_first_managed"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("request %d cc_prev_req = %q, want %q", index+1, got[index], want[index])
		}
	}
	if firstBilling := gjson.GetBytes(bodies[0], "system.0.text").String(); !strings.Contains(firstBilling, "cc_prev_req=req_caller_owned;") || strings.Count(firstBilling, "cc_prev_req=") != 1 {
		t.Fatalf("caller-owned billing text was rewritten: first=%q", firstBilling)
	}
}

func TestClaudeExecutorPrevRequestExecuteIsolatesCredentialAndSession(t *testing.T) {
	sessionA := uuid.NewString()
	cfg, authA, requestA, optionsA := claudePrevRequestFixture(t, "s4a3-isolation-a-"+uuid.NewString(), sessionA)
	_, authB, requestB, optionsB := claudePrevRequestFixture(t, "s4a3-isolation-b-"+uuid.NewString(), sessionA)
	_, _, requestOtherSession, optionsOtherSession := claudePrevRequestFixture(t, authA.ID, uuid.NewString())
	var bodies [][]byte
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		bodies = append(bodies, body)
		return claudePrevRequestSuccessResponse(req, fmt.Sprintf("req_scope_%d", len(bodies))), nil
	})
	executor := NewClaudeExecutor(cfg)
	for _, turn := range []struct {
		auth    *cliproxyauth.Auth
		request cliproxyexecutor.Request
		options cliproxyexecutor.Options
	}{
		{authA, requestA, optionsA},
		{authA, requestOtherSession, optionsOtherSession},
		{authB, requestB, optionsB},
		{authA, requestA, optionsA},
	} {
		if _, errExecute := claudePrevRequestExecute(t, transport, executor, turn.auth, turn.request, turn.options); errExecute != nil {
			t.Fatalf("Execute() error = %v", errExecute)
		}
	}
	want := []string{"", "", "", "req_scope_1"}
	for index := range want {
		if got := claudePrevRequestValue(bodies[index]); got != want[index] {
			t.Fatalf("request %d cc_prev_req = %q, want %q", index+1, got, want[index])
		}
	}
}

func TestClaudeExecutorPrevRequestExecuteExcludesHelperAndCustomUpstream(t *testing.T) {
	t.Run("structured helper", func(t *testing.T) {
		betas := claudeNativeHelperCoreBetas + ",structured-outputs-2025-12-15"
		payload := []byte(`{"model":"claude-haiku-4-5-20251001","messages":[{"role":"user","content":[{"type":"text","text":"helper probe"}]}],"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.258; cc_entrypoint=cli; cch=00000;"},{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."},{"type":"text","text":"Return a short title."}],"tools":[],"metadata":{"user_id":"` + strings.ReplaceAll(claudeNativeHelperUserID, `"`, `\"`) + `"},"max_tokens":32000,"thinking":{"type":"disabled"},"temperature":1,"output_config":{"format":{"type":"json_schema","schema":{"type":"object","properties":{"title":{"type":"string"}},"required":["title"],"additionalProperties":false}}},"stream":true}`)
		auth := claudeNativeHelperOAuthAuth("")
		auth.ID = "s4a3-helper-" + uuid.NewString()
		request := cliproxyexecutor.Request{Model: "claude-haiku-4-5-20251001", Payload: payload}
		options := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude, OriginalRequest: payload, Headers: claudeNativeHelperHeaders(betas, "gzip, deflate, br, zstd")}
		var bodies [][]byte
		transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			bodies = append(bodies, body)
			return claudePrevRequestSuccessResponse(req, fmt.Sprintf("req_helper_%d", len(bodies))), nil
		})
		for range 2 {
			if _, errExecute := claudePrevRequestExecute(t, transport, NewClaudeExecutor(&config.Config{}), auth, request, options); errExecute != nil {
				t.Fatalf("helper Execute() error = %v", errExecute)
			}
		}
		if got := claudePrevRequestValue(bodies[1]); got != "" {
			t.Fatalf("helper cc_prev_req = %q, want absent", got)
		}
	})

	t.Run("custom upstream", func(t *testing.T) {
		cfg, auth, request, options := claudePrevRequestFixture(t, "s4a3-custom-"+uuid.NewString(), uuid.NewString())
		auth.Attributes["base_url"] = "https://custom.example"
		var bodies [][]byte
		transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			bodies = append(bodies, body)
			return claudePrevRequestSuccessResponse(req, fmt.Sprintf("req_custom_%d", len(bodies))), nil
		})
		executor := NewClaudeExecutor(cfg)
		for range 2 {
			if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
				t.Fatalf("custom Execute() error = %v", errExecute)
			}
		}
		if got := claudePrevRequestValue(bodies[1]); got != "" {
			t.Fatalf("custom upstream cc_prev_req = %q, want absent", got)
		}
	})
}
