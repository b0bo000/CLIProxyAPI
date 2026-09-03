package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func claudePrevRequestHTTPFailureResponse(req *http.Request, status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"error":{"type":"synthetic_%d","message":"synthetic failure"}}`, status))),
		Request:    req,
	}
}

func TestClaudePrevRequestExecuteHTTPStatusMatrixDoesNotAdvance(t *testing.T) {
	statuses := []int{
		http.StatusNotFound,
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		529,
	}
	for _, status := range statuses {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			cfg, auth, request, options := claudePrevRequestFixture(t, fmt.Sprintf("s4a6-execute-%d-%s", status, uuid.NewString()), uuid.NewString())
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
					return claudePrevRequestSuccessResponse(req, "req_s4a6_seed"), nil
				case 2:
					return claudePrevRequestHTTPFailureResponse(req, status), nil
				default:
					return claudePrevRequestSuccessResponse(req, "req_s4a6_after_failure"), nil
				}
			})
			executor := NewClaudeExecutor(cfg)
			if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
				t.Fatalf("seed Execute() error = %v", errExecute)
			}
			if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute == nil {
				t.Fatalf("status %d Execute() error = nil", status)
			}
			if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
				t.Fatalf("post-failure Execute() error = %v", errExecute)
			}
			if got := claudePrevRequestValue(bodies[1]); got != "req_s4a6_seed" {
				t.Fatalf("failed request cc_prev_req = %q, want committed seed request ID", got)
			}
			if got := claudePrevRequestValue(bodies[2]); got != "req_s4a6_seed" {
				t.Fatalf("retry request cc_prev_req = %q, want committed seed request ID", got)
			}
		})
	}
}

func TestClaudePrevRequestStreamHTTPStatusMatrixDoesNotAdvance(t *testing.T) {
	statuses := []int{
		http.StatusNotFound,
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		529,
	}
	for _, status := range statuses {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			cfg, auth, request, options := claudePrevRequestFixture(t, fmt.Sprintf("s4a6-stream-%d-%s", status, uuid.NewString()), uuid.NewString())
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
					return claudePrevRequestStreamResponse(req, "req_s4a6_stream_seed", claudePrevRequestCompleteSSE("msg_s4a6_stream_seed")), nil
				case 2:
					return claudePrevRequestHTTPFailureResponse(req, status), nil
				default:
					return claudePrevRequestStreamResponse(req, "req_s4a6_stream_after_failure", claudePrevRequestCompleteSSE("msg_s4a6_stream_after_failure")), nil
				}
			})
			executor := NewClaudeExecutor(cfg)
			if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options); errStream != nil {
				t.Fatalf("seed ExecuteStream() error = %v", errStream)
			}
			if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options); errStream == nil {
				t.Fatalf("status %d ExecuteStream() error = nil", status)
			}
			if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options); errStream != nil {
				t.Fatalf("post-failure ExecuteStream() error = %v", errStream)
			}
			if got := claudePrevRequestValue(bodies[1]); got != "req_s4a6_stream_seed" {
				t.Fatalf("failed stream cc_prev_req = %q, want committed seed request ID", got)
			}
			if got := claudePrevRequestValue(bodies[2]); got != "req_s4a6_stream_seed" {
				t.Fatalf("retry stream cc_prev_req = %q, want committed seed request ID", got)
			}
		})
	}
}

func TestClaudePrevRequestCountTokensAndCompactDoNotCreateChain(t *testing.T) {
	t.Run("count_tokens", func(t *testing.T) {
		cfg, auth, request, options := claudePrevRequestFixture(t, "s4a6-count-tokens-"+uuid.NewString(), uuid.NewString())
		var countTokensBody []byte
		var messageBodies [][]byte
		transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body, errRead := io.ReadAll(req.Body)
			if errRead != nil {
				return nil, errRead
			}
			if strings.HasSuffix(req.URL.Path, "/count_tokens") {
				countTokensBody = body
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"input_tokens":1}`)),
					Request:    req,
				}, nil
			}
			messageBodies = append(messageBodies, body)
			return claudePrevRequestSuccessResponse(req, fmt.Sprintf("req_s4a6_count_message_%d", len(messageBodies))), nil
		})
		executor := NewClaudeExecutor(cfg)
		countOptions := options
		countOptions.OriginalRequest = request.Payload
		countResponse, errCount := executor.CountTokens(context.WithValue(context.Background(), "cliproxy.roundtripper", transport), auth, request, countOptions)
		if errCount != nil {
			t.Fatalf("CountTokens() error = %v", errCount)
		}
		if len(countResponse.Payload) == 0 || len(countTokensBody) == 0 {
			t.Fatalf("CountTokens() did not produce a response and upstream body")
		}
		if got := claudePrevRequestValue(countTokensBody); got != "" {
			t.Fatalf("count_tokens cc_prev_req = %q, want absent", got)
		}
		if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
			t.Fatalf("first Execute() error = %v", errExecute)
		}
		if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
			t.Fatalf("second Execute() error = %v", errExecute)
		}
		if got := claudePrevRequestValue(messageBodies[0]); got != "" {
			t.Fatalf("first message after count_tokens cc_prev_req = %q, want absent", got)
		}
		if got := claudePrevRequestValue(messageBodies[1]); got != "req_s4a6_count_message_1" {
			t.Fatalf("second message after count_tokens cc_prev_req = %q, want first message request ID", got)
		}
	})

	t.Run("responses_compact", func(t *testing.T) {
		cfg, auth, request, options := claudePrevRequestFixture(t, "s4a6-compact-"+uuid.NewString(), uuid.NewString())
		options.Alt = "responses/compact"
		executor := NewClaudeExecutor(cfg)
		if _, errExecute := claudePrevRequestExecute(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return claudePrevRequestSuccessResponse(req, "req_must_not_reach_upstream"), nil
		}), executor, auth, request, options); errExecute == nil {
			t.Fatal("responses/compact Execute() error = nil")
		}
		options.Alt = ""
		var bodies [][]byte
		transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body, errRead := io.ReadAll(req.Body)
			if errRead != nil {
				return nil, errRead
			}
			bodies = append(bodies, body)
			return claudePrevRequestSuccessResponse(req, fmt.Sprintf("req_s4a6_compact_message_%d", len(bodies))), nil
		})
		if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
			t.Fatalf("first Execute() error = %v", errExecute)
		}
		if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
			t.Fatalf("second Execute() error = %v", errExecute)
		}
		if got := claudePrevRequestValue(bodies[0]); got != "" {
			t.Fatalf("first message after compact cc_prev_req = %q, want absent", got)
		}
		if got := claudePrevRequestValue(bodies[1]); got != "req_s4a6_compact_message_1" {
			t.Fatalf("second message after compact cc_prev_req = %q, want first message request ID", got)
		}
	})
}

func TestClaudePrevRequestExecuteStreamParityKeepsFinalBodyAndHeaderState(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream_%t", stream), func(t *testing.T) {
			cfg, auth, request, options := claudePrevRequestFixture(t, "s4a6-parity-"+uuid.NewString(), uuid.NewString())
			var bodies [][]byte
			var headers []http.Header
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				body, errRead := io.ReadAll(req.Body)
				if errRead != nil {
					return nil, errRead
				}
				bodies = append(bodies, body)
				responseHeaders := http.Header{"Request-Id": {fmt.Sprintf("req_s4a6_parity_%d", len(bodies))}}
				headers = append(headers, responseHeaders.Clone())
				if stream {
					responseHeaders.Set("Content-Type", "text/event-stream")
					return &http.Response{StatusCode: http.StatusOK, Header: responseHeaders, Body: io.NopCloser(strings.NewReader(claudePrevRequestCompleteSSE(fmt.Sprintf("msg_s4a6_parity_%d", len(bodies))))), Request: req}, nil
				}
				responseHeaders.Set("Content-Type", "application/json")
				return &http.Response{StatusCode: http.StatusOK, Header: responseHeaders, Body: io.NopCloser(strings.NewReader(`{"id":"msg_s4a6_parity","type":"message","model":"claude-opus-5","role":"assistant","content":[]}`)), Request: req}, nil
			})
			executor := NewClaudeExecutor(cfg)
			if stream {
				if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options); errStream != nil {
					t.Fatalf("first ExecuteStream() error = %v", errStream)
				}
				if errStream := claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options); errStream != nil {
					t.Fatalf("second ExecuteStream() error = %v", errStream)
				}
			} else {
				if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
					t.Fatalf("first Execute() error = %v", errExecute)
				}
				if _, errExecute := claudePrevRequestExecute(t, transport, executor, auth, request, options); errExecute != nil {
					t.Fatalf("second Execute() error = %v", errExecute)
				}
			}
			if got := claudePrevRequestValue(bodies[0]); got != "" {
				t.Fatalf("first body cc_prev_req = %q, want absent", got)
			}
			if got := claudePrevRequestValue(bodies[1]); got != headers[0].Get("Request-Id") {
				t.Fatalf("second body cc_prev_req = %q, want response request ID %q", got, headers[0].Get("Request-Id"))
			}
			if strings.Contains(string(bodies[1]), claudePrevRequestProbeID) {
				t.Fatalf("probe request ID leaked into final body: %s", bodies[1])
			}
		})
	}
}
