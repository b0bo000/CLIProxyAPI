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
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

type claudePrevRequestStreamReadErrorBody struct {
	reader io.Reader
}

func (body *claudePrevRequestStreamReadErrorBody) Read(buffer []byte) (int, error) {
	if body.reader != nil {
		read, errRead := body.reader.Read(buffer)
		if read > 0 {
			return read, nil
		}
		if errRead != nil && !errors.Is(errRead, io.EOF) {
			return 0, errRead
		}
		body.reader = nil
	}
	return 0, errors.New("synthetic stream read failure")
}

func (body *claudePrevRequestStreamReadErrorBody) Close() error { return nil }

func claudePrevRequestCompleteSSE(messageID string) string {
	return strings.Join([]string{
		`event: message_start`,
		fmt.Sprintf(`data: {"type":"message_start","message":{"id":%q,"type":"message","model":"claude-opus-5"}}`, messageID),
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
}

func claudePrevRequestStreamResponse(req *http.Request, requestID, stream string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": {"text/event-stream"},
			"Request-Id":   {requestID},
		},
		Body:    io.NopCloser(strings.NewReader(stream)),
		Request: req,
	}
}

func claudePrevRequestDrainStream(result *cliproxyexecutor.StreamResult, errStart error) error {
	if errStart != nil {
		return errStart
	}
	if result == nil {
		return errors.New("nil stream result")
	}
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			return chunk.Err
		}
	}
	return nil
}

func claudePrevRequestExecuteStream(
	ctx context.Context,
	transport http.RoundTripper,
	executor *ClaudeExecutor,
	auth *cliproxyauth.Auth,
	request cliproxyexecutor.Request,
	options cliproxyexecutor.Options,
) error {
	ctx = context.WithValue(ctx, "cliproxy.roundtripper", transport)
	result, errStart := executor.ExecuteStream(ctx, auth, request, options)
	return claudePrevRequestDrainStream(result, errStart)
}

func TestClaudeExecutorPrevRequestStreamAdvancesOnlyAfterMessageStop(t *testing.T) {
	sessionID := uuid.NewString()
	cfg, auth, request, options := claudePrevRequestFixture(t, "s4a4-success-"+uuid.NewString(), sessionID)
	var bodies [][]byte
	requestIDs := []string{"req_s4a4_first", "req_s4a4_second", "req_s4a4_third"}
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			return nil, errRead
		}
		bodies = append(bodies, body)
		return claudePrevRequestStreamResponse(req, requestIDs[len(bodies)-1], claudePrevRequestCompleteSSE(fmt.Sprintf("msg_stream_%d", len(bodies)))), nil
	})
	executor := NewClaudeExecutor(cfg)
	for range 3 {
		if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options); errStream != nil {
			t.Fatalf("ExecuteStream() error = %v", errStream)
		}
	}
	want := []string{"", requestIDs[0], requestIDs[1]}
	for index, expected := range want {
		if got := claudePrevRequestValue(bodies[index]); got != expected {
			t.Fatalf("request %d cc_prev_req = %q, want %q; body=%s", index+1, got, expected, bodies[index])
		}
	}
	for index := 1; index < len(bodies); index++ {
		resigned, errSign := finalizeAnthropicMessagesBodyCCH(bodies[index], "")
		if errSign != nil || !bytes.Equal(resigned, bodies[index]) {
			t.Fatalf("request %d CCH does not cover cc_prev_req: error=%v", index+1, errSign)
		}
	}
}

func TestClaudeExecutorPrevRequestStreamTranslatedPathCommits(t *testing.T) {
	sessionID := uuid.NewString()
	cfg, auth, request, options := claudePrevRequestFixture(t, "s4a4-translated-"+uuid.NewString(), sessionID)
	options.ResponseFormat = sdktranslator.FormatOpenAI
	var bodies [][]byte
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			return nil, errRead
		}
		bodies = append(bodies, body)
		return claudePrevRequestStreamResponse(req, fmt.Sprintf("req_translated_%d", len(bodies)), claudePrevRequestCompleteSSE(fmt.Sprintf("msg_translated_%d", len(bodies)))), nil
	})
	executor := NewClaudeExecutor(cfg)
	for range 2 {
		if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options); errStream != nil {
			t.Fatalf("translated ExecuteStream() error = %v", errStream)
		}
	}
	if got := claudePrevRequestValue(bodies[1]); got != "req_translated_1" {
		t.Fatalf("translated second cc_prev_req = %q, want req_translated_1", got)
	}
}

func TestClaudeExecutorPrevRequestStreamSeparatesAgentCredentialAndSessionScopes(t *testing.T) {
	sessionA := uuid.NewString()
	cfg, authA, requestA, optionsA := claudePrevRequestFixture(t, "s4a4-isolation-a-"+uuid.NewString(), sessionA)
	_, authB, requestB, optionsB := claudePrevRequestFixture(t, "s4a4-isolation-b-"+uuid.NewString(), sessionA)
	_, _, requestOtherSession, optionsOtherSession := claudePrevRequestFixture(t, authA.ID, uuid.NewString())

	parentOptions := optionsA
	parentOptions.Headers = optionsA.Headers.Clone()
	parentOptions.Headers.Set("X-Claude-Code-Agent-Id", "main")
	childOptions := optionsA
	childOptions.Headers = optionsA.Headers.Clone()
	childOptions.Headers.Set("X-Claude-Code-Agent-Id", "subagent-1")

	var bodies [][]byte
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			return nil, errRead
		}
		bodies = append(bodies, body)
		index := len(bodies)
		return claudePrevRequestStreamResponse(req, fmt.Sprintf("req_scope_%d", index), claudePrevRequestCompleteSSE(fmt.Sprintf("msg_scope_%d", index))), nil
	})
	executor := NewClaudeExecutor(cfg)
	for _, turn := range []struct {
		auth    *cliproxyauth.Auth
		request cliproxyexecutor.Request
		options cliproxyexecutor.Options
	}{
		{authA, requestA, parentOptions},
		{authA, requestA, childOptions},
		{authA, requestOtherSession, optionsOtherSession},
		{authB, requestB, optionsB},
		{authA, requestA, parentOptions},
	} {
		if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, turn.auth, turn.request, turn.options); errStream != nil {
			t.Fatalf("ExecuteStream() error = %v", errStream)
		}
	}

	want := []string{"", "", "", "", "req_scope_1"}
	for index, expected := range want {
		if got := claudePrevRequestValue(bodies[index]); got != expected {
			t.Fatalf("request %d cc_prev_req = %q, want %q", index+1, got, expected)
		}
	}
}

func TestClaudeExecutorPrevRequestStreamFailuresDoNotAdvance(t *testing.T) {
	tests := []struct {
		name     string
		response func(*http.Request) (*http.Response, error)
		wantErr  bool
	}{
		{
			name:    "upstream non-2xx",
			wantErr: true,
			response: func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusInternalServerError, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"failure"}}`)), Request: req}, nil
			},
		},
		{
			name:    "read error after message stop",
			wantErr: true,
			response: func(req *http.Request) (*http.Response, error) {
				response := claudePrevRequestStreamResponse(req, "req_read_error", "")
				response.Body = &claudePrevRequestStreamReadErrorBody{reader: strings.NewReader(claudePrevRequestCompleteSSE("msg_read_error"))}
				return response, nil
			},
		},
		{
			name: "missing message stop",
			response: func(req *http.Request) (*http.Response, error) {
				stream := strings.Replace(claudePrevRequestCompleteSSE("msg_no_stop"), "event: message_stop\ndata: {\"type\":\"message_stop\"}\n", "", 1)
				return claudePrevRequestStreamResponse(req, "req_no_stop", stream), nil
			},
		},
		{
			name: "malformed event after message stop",
			response: func(req *http.Request) (*http.Response, error) {
				return claudePrevRequestStreamResponse(req, "req_malformed", claudePrevRequestCompleteSSE("msg_malformed")+"data: not-json\n\n"), nil
			},
		},
		{
			name: "upstream error event",
			response: func(req *http.Request) (*http.Response, error) {
				stream := claudePrevRequestCompleteSSE("msg_error_event") + `data: {"type":"error","error":{"message":"late"}}` + "\n\n"
				return claudePrevRequestStreamResponse(req, "req_error_event", stream), nil
			},
		},
		{
			name: "invalid response request id",
			response: func(req *http.Request) (*http.Response, error) {
				return claudePrevRequestStreamResponse(req, "req_first,req_second", claudePrevRequestCompleteSSE("msg_invalid_id")), nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, auth, request, options := claudePrevRequestFixture(t, "s4a4-failure-"+uuid.NewString(), uuid.NewString())
			var bodies [][]byte
			call := 0
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				body, errRead := io.ReadAll(req.Body)
				if errRead != nil {
					return nil, errRead
				}
				bodies = append(bodies, body)
				call++
				switch call {
				case 1:
					return claudePrevRequestStreamResponse(req, "req_seed", claudePrevRequestCompleteSSE("msg_seed")), nil
				case 2:
					return test.response(req)
				default:
					return claudePrevRequestStreamResponse(req, "req_after_failure", claudePrevRequestCompleteSSE("msg_after_failure")), nil
				}
			})
			executor := NewClaudeExecutor(cfg)
			if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options); errStream != nil {
				t.Fatalf("seed ExecuteStream() error = %v", errStream)
			}
			errFailure := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options)
			if test.wantErr && errFailure == nil {
				t.Fatal("failure ExecuteStream() error = nil")
			}
			if !test.wantErr && errFailure != nil {
				t.Fatalf("failure fixture ExecuteStream() error = %v", errFailure)
			}
			if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options); errStream != nil {
				t.Fatalf("post-failure ExecuteStream() error = %v", errStream)
			}
			if got := claudePrevRequestValue(bodies[2]); got != "req_seed" {
				t.Fatalf("post-failure cc_prev_req = %q, want req_seed", got)
			}
		})
	}
}

func TestClaudeExecutorPrevRequestStreamCancellationDoesNotAdvance(t *testing.T) {
	cfg, auth, request, options := claudePrevRequestFixture(t, "s4a4-cancel-"+uuid.NewString(), uuid.NewString())
	var bodies [][]byte
	call := 0
	var cancel context.CancelFunc
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			return nil, errRead
		}
		bodies = append(bodies, body)
		call++
		response := claudePrevRequestStreamResponse(req, fmt.Sprintf("req_cancel_%d", call), claudePrevRequestCompleteSSE(fmt.Sprintf("msg_cancel_%d", call)))
		if call == 2 {
			response.Body = &claudePrevRequestCancelOnEOFBody{reader: strings.NewReader(claudePrevRequestCompleteSSE("msg_cancelled")), cancel: cancel}
		}
		return response, nil
	})
	executor := NewClaudeExecutor(cfg)
	if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options); errStream != nil {
		t.Fatalf("seed ExecuteStream() error = %v", errStream)
	}
	cancelContext, cancelFn := context.WithCancel(context.Background())
	cancel = cancelFn
	_ = claudePrevRequestExecuteStream(cancelContext, transport, executor, auth, request, options)
	if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options); errStream != nil {
		t.Fatalf("post-cancellation ExecuteStream() error = %v", errStream)
	}
	if got := claudePrevRequestValue(bodies[2]); got != "req_cancel_1" {
		t.Fatalf("post-cancellation cc_prev_req = %q, want req_cancel_1", got)
	}
}

func TestClaudeExecutorPrevRequestStreamPreservesCallerOwnership(t *testing.T) {
	cfg, auth, request, options := claudePrevRequestFixture(t, "s4a4-caller-"+uuid.NewString(), uuid.NewString())
	callerPayload := bytes.Replace(request.Payload, []byte(" cch=00000;"), []byte(" cch=00000; cc_prev_req=req_caller_owned;"), 1)
	var bodies [][]byte
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			return nil, errRead
		}
		bodies = append(bodies, body)
		return claudePrevRequestStreamResponse(req, fmt.Sprintf("req_caller_%d", len(bodies)), claudePrevRequestCompleteSSE(fmt.Sprintf("msg_caller_%d", len(bodies)))), nil
	})
	executor := NewClaudeExecutor(cfg)
	callerRequest := request
	callerRequest.Payload = callerPayload
	callerOptions := options
	callerOptions.OriginalRequest = callerPayload
	if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, callerRequest, callerOptions); errStream != nil {
		t.Fatalf("caller-owned ExecuteStream() error = %v", errStream)
	}
	for range 2 {
		if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options); errStream != nil {
			t.Fatalf("managed ExecuteStream() error = %v", errStream)
		}
	}
	want := []string{"req_caller_owned", "", "req_caller_2"}
	for index, expected := range want {
		if got := claudePrevRequestValue(bodies[index]); got != expected {
			t.Fatalf("request %d cc_prev_req = %q, want %q", index+1, got, expected)
		}
	}
}

func TestClaudePrevRequestStreamValidationRequiresCompleteSequence(t *testing.T) {
	complete := claudePrevRequestCompleteSSE("msg_valid")
	for _, test := range []struct {
		name   string
		stream string
		want   bool
	}{
		{name: "complete", stream: complete, want: true},
		{name: "missing stop", stream: strings.Replace(complete, `data: {"type":"message_stop"}`, "", 1)},
		{name: "missing delta", stream: strings.Replace(complete, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`, "", 1)},
		{name: "numeric message id", stream: strings.Replace(complete, `"id":"msg_valid"`, `"id":123`, 1)},
		{name: "malformed data", stream: complete + "data: nope\n"},
		{name: "event after stop", stream: complete + `data: {"type":"ping"}` + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			validation := claudePrevRequestStreamValidation{}
			for _, line := range bytes.Split([]byte(test.stream), []byte("\n")) {
				validation.Observe(line)
			}
			if got := validation.Complete(); got != test.want {
				t.Fatalf("Complete() = %t, want %t", got, test.want)
			}
		})
	}
}
