package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/tidwall/gjson"
)

const claudePromptIDIntegrationValue = "986813f9-290b-409e-8541-06cd11254627"

func claudePromptIDWithBillingFields(
	t *testing.T,
	request cliproxyexecutor.Request,
	options cliproxyexecutor.Options,
	fields string,
) (cliproxyexecutor.Request, cliproxyexecutor.Options) {
	t.Helper()
	const marker = " cch=00000;"
	if bytes.Count(request.Payload, []byte(marker)) != 1 {
		t.Fatalf("billing CCH marker count = %d, want 1", bytes.Count(request.Payload, []byte(marker)))
	}
	payload := bytes.Replace(request.Payload, []byte(marker), []byte(marker+fields), 1)
	request.Payload = payload
	options.OriginalRequest = payload
	return request, options
}

func claudePromptIDInvoke(
	stream bool,
	transport http.RoundTripper,
	executor *ClaudeExecutor,
	auth *cliproxyauth.Auth,
	request cliproxyexecutor.Request,
	options cliproxyexecutor.Options,
) error {
	if stream {
		return claudePrevRequestExecuteStream(context.Background(), transport, executor, auth, request, options)
	}
	ctx := context.WithValue(context.Background(), "cliproxy.roundtripper", transport)
	_, errExecute := executor.Execute(ctx, auth, request, options)
	return errExecute
}

func claudePromptIDBillingField(body []byte, key string) (string, int) {
	billing := gjson.GetBytes(body, "system.0.text").String()
	value := ""
	count := 0
	for _, segment := range strings.Split(strings.TrimPrefix(billing, claudeBillingHeaderPrefix), ";") {
		field, fieldValue, found := strings.Cut(strings.TrimSpace(segment), "=")
		if found && strings.TrimSpace(field) == key {
			value = strings.TrimSpace(fieldValue)
			count++
		}
	}
	return value, count
}

func assertClaudePromptIDUpstreamBody(t *testing.T, body []byte, marker, previousRequestID, previousMessageID string) {
	t.Helper()
	if !gjson.ValidBytes(body) {
		t.Fatalf("upstream body is not valid JSON: %s", body)
	}
	system := gjson.GetBytes(body, "system")
	if !system.IsArray() || len(system.Array()) != 2 {
		t.Fatalf("system blocks = %s, want the original two blocks", system.Raw)
	}
	billingBlocks := 0
	for index, block := range system.Array() {
		if strings.HasPrefix(strings.TrimSpace(block.Get("text").String()), claudeBillingHeaderPrefix) {
			billingBlocks++
			if index != 0 {
				t.Fatalf("billing block index = %d, want 0", index)
			}
		}
	}
	if billingBlocks != 1 {
		t.Fatalf("billing block count = %d, want 1", billingBlocks)
	}

	billing := system.Array()[0].Get("text").String()
	promptID, promptCount := claudePromptIDBillingField(body, "cc_prompt_id")
	if promptID != claudePromptIDIntegrationValue || promptCount != 1 {
		t.Fatalf("cc_prompt_id = %q count=%d, want %q exactly once; billing=%q", promptID, promptCount, claudePromptIDIntegrationValue, billing)
	}
	if strings.Count(billing, "cc_prompt_id=") != 1 {
		t.Fatalf("cc_prompt_id key count = %d, want 1; billing=%q", strings.Count(billing, "cc_prompt_id="), billing)
	}
	previous, previousCount := claudePromptIDBillingField(body, "cc_prev_req")
	if previous != previousRequestID {
		t.Fatalf("cc_prev_req = %q, want %q; billing=%q", previous, previousRequestID, billing)
	}
	wantPreviousCount := 0
	if previousRequestID != "" {
		wantPreviousCount = 1
	}
	if previousCount != wantPreviousCount {
		t.Fatalf("cc_prev_req count = %d, want %d; billing=%q", previousCount, wantPreviousCount, billing)
	}

	entrypointAt := strings.Index(billing, "cc_entrypoint=")
	cchAt := strings.Index(billing, "cch=")
	promptAt := strings.Index(billing, "cc_prompt_id=")
	if !(entrypointAt >= 0 && entrypointAt < cchAt && cchAt < promptAt) {
		t.Fatalf("billing order is not entrypoint < cch < prompt: %q", billing)
	}
	if previousRequestID != "" {
		previousAt := strings.Index(billing, "cc_prev_req=")
		if !(cchAt < previousAt && previousAt < promptAt) {
			t.Fatalf("billing order is not cch < previous < prompt: %q", billing)
		}
	}

	cch, cchCount := claudePromptIDBillingField(body, "cch")
	if cchCount != 1 || len(cch) != claudeCCHLength || !isLowerHex([]byte(cch)) {
		t.Fatalf("cch = %q count=%d, want one finalized lowercase value", cch, cchCount)
	}
	resigned, errSign := finalizeAnthropicMessagesBodyCCH(body, "")
	if errSign != nil {
		t.Fatalf("re-sign CCH: %v", errSign)
	}
	if !bytes.Equal(resigned, body) {
		t.Fatal("CCH re-signing changed the finalized upstream body")
	}

	if got := gjson.GetBytes(body, "metadata.s5_prompt_preservation").String(); got != marker {
		t.Fatalf("payload-rule marker = %q, want %q", got, marker)
	}
	diagnostics := gjson.GetBytes(body, "diagnostics")
	if !diagnostics.Exists() || !diagnostics.IsObject() {
		t.Fatalf("diagnostics = %s, want injected object", diagnostics.Raw)
	}
	if previousMessageID != "" {
		if got := diagnostics.Get("previous_message_id").String(); got != previousMessageID {
			t.Fatalf("diagnostics.previous_message_id = %q, want %q", got, previousMessageID)
		}
	}
	if got := system.Array()[1].Get("text").String(); got != "You are Claude Code, Anthropic's official CLI for Claude." {
		t.Fatalf("unrelated system text changed: %q", got)
	}
	if got := gjson.GetBytes(body, "messages.0.content").String(); got != "test" {
		t.Fatalf("message content changed: %q", got)
	}
}

func claudePromptIDTestPreservation(t *testing.T, stream bool) {
	t.Helper()
	sessionID := "11111111-2222-4333-8444-555555555555"
	cfg, auth, request, options := claudePrevRequestFixture(t, "s5-2-3-preserve-"+fmt.Sprint(stream), sessionID)
	cfg.Payload.Override = []config.PayloadRule{{
		Models: []config.PayloadModelRule{{Name: "claude-opus-5"}},
		Params: map[string]any{"metadata.s5_prompt_preservation": fmt.Sprint(stream)},
	}}
	request, options = claudePromptIDWithBillingFields(t, request, options, " cc_prompt_id="+claudePromptIDIntegrationValue+";")

	var bodies [][]byte
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			return nil, errRead
		}
		bodies = append(bodies, body)
		index := len(bodies)
		if stream {
			return claudePrevRequestStreamResponse(req, fmt.Sprintf("req_s523_stream_%d", index), claudePrevRequestCompleteSSE(fmt.Sprintf("msg_s523_stream_%d", index))), nil
		}
		return claudePrevRequestSuccessResponse(req, fmt.Sprintf("req_s523_execute_%d", index)), nil
	})
	executor := NewClaudeExecutor(cfg)
	for index := range 2 {
		if errExecute := claudePromptIDInvoke(stream, transport, executor, auth, request, options); errExecute != nil {
			t.Fatalf("request %d error = %v", index+1, errExecute)
		}
	}
	if len(bodies) != 2 {
		t.Fatalf("captured bodies = %d, want 2", len(bodies))
	}
	previousRequestID := ""
	previousMessageID := ""
	if stream {
		previousRequestID = "req_s523_stream_1"
		previousMessageID = "msg_s523_stream_1"
	} else {
		previousRequestID = "req_s523_execute_1"
		previousMessageID = "msg_s4a3"
	}
	assertClaudePromptIDUpstreamBody(t, bodies[0], fmt.Sprint(stream), "", "")
	assertClaudePromptIDUpstreamBody(t, bodies[1], fmt.Sprint(stream), previousRequestID, previousMessageID)
}

func TestClaudeExecutorPromptIDExecutePreservesCallerValue(t *testing.T) {
	claudePromptIDTestPreservation(t, false)
}

func TestClaudeExecutorPromptIDStreamPreservesCallerValue(t *testing.T) {
	claudePromptIDTestPreservation(t, true)
}

func TestClaudeExecutorPromptIDGeneratedAndStable(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			cfg, auth, request, options := claudePrevRequestFixture(t, "s5-2-3-absent-"+fmt.Sprint(stream), "22222222-3333-4444-8555-666666666666")
			withPromptRequest, withPromptOptions := claudePromptIDWithBillingFields(t, request, options, " cc_prompt_id="+claudePromptIDIntegrationValue+";")
			var bodies [][]byte
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				body, errRead := io.ReadAll(req.Body)
				if errRead != nil {
					return nil, errRead
				}
				bodies = append(bodies, body)
				index := len(bodies)
				if stream {
					return claudePrevRequestStreamResponse(req, fmt.Sprintf("req_s523_absent_%d", index), claudePrevRequestCompleteSSE(fmt.Sprintf("msg_s523_absent_%d", index))), nil
				}
				return claudePrevRequestSuccessResponse(req, fmt.Sprintf("req_s523_absent_%d", index)), nil
			})
			executor := NewClaudeExecutor(cfg)
			if errFirst := claudePromptIDInvoke(stream, transport, executor, auth, withPromptRequest, withPromptOptions); errFirst != nil {
				t.Fatalf("caller-owned request error = %v", errFirst)
			}
			if errSecond := claudePromptIDInvoke(stream, transport, executor, auth, request, options); errSecond != nil {
				t.Fatalf("idempotent retry error = %v", errSecond)
			}
			if len(bodies) != 2 {
				t.Fatalf("captured bodies = %d, want 2", len(bodies))
			}
			if promptID, count := claudePromptIDBillingField(bodies[1], "cc_prompt_id"); promptID != claudePromptIDIntegrationValue || count != 1 {
				t.Fatalf("idempotent retry cc_prompt_id = %q count=%d, want caller value %q; body=%s", promptID, count, claudePromptIDIntegrationValue, bodies[1])
			}

			newPayload := bytes.Replace(request.Payload, []byte(`"test"`), []byte(`"second"`), 1)
			newRequest := request
			newRequest.Payload = newPayload
			newOptions := options
			newOptions.OriginalRequest = newPayload
			if errThird := claudePromptIDInvoke(stream, transport, executor, auth, newRequest, newOptions); errThird != nil {
				t.Fatalf("new prompt error = %v", errThird)
			}
			if len(bodies) != 3 {
				t.Fatalf("captured bodies = %d, want 3", len(bodies))
			}
			if promptID, count := claudePromptIDBillingField(bodies[2], "cc_prompt_id"); promptID == claudePromptIDIntegrationValue || count != 1 {
				t.Fatalf("new prompt cc_prompt_id = %q count=%d, want one distinct generated value; body=%s", promptID, count, bodies[2])
			}
		})
	}
}

func TestClaudeExecutorPromptIDSubagentInheritsParent(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			cfg, auth, parentRequest, parentOptions := claudePrevRequestFixture(t, "s5-3-1b-inherit-"+fmt.Sprint(stream), "55555555-6666-4777-8888-999999999999")
			childRequest := parentRequest
			childPayload := bytes.Replace(parentRequest.Payload, []byte(`"test"`), []byte(`"different child"`), 1)
			childRequest.Payload = childPayload
			childRequest, childOptions := claudePromptIDWithBillingFields(t, childRequest, parentOptions, " cc_is_subagent=true;")
			childOptions.Headers = parentOptions.Headers.Clone()
			childOptions.Headers.Set("X-Claude-Code-Agent-Id", "subagent-1")

			var bodies [][]byte
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				body, errRead := io.ReadAll(req.Body)
				if errRead != nil {
					return nil, errRead
				}
				bodies = append(bodies, body)
				index := len(bodies)
				if stream {
					return claudePrevRequestStreamResponse(req, fmt.Sprintf("req_s531b_%d", index), claudePrevRequestCompleteSSE(fmt.Sprintf("msg_s531b_%d", index))), nil
				}
				return claudePrevRequestSuccessResponse(req, fmt.Sprintf("req_s531b_%d", index)), nil
			})
			executor := NewClaudeExecutor(cfg)
			if errParent := claudePromptIDInvoke(stream, transport, executor, auth, parentRequest, parentOptions); errParent != nil {
				t.Fatalf("parent request error = %v", errParent)
			}
			if errChild := claudePromptIDInvoke(stream, transport, executor, auth, childRequest, childOptions); errChild != nil {
				t.Fatalf("child request error = %v", errChild)
			}
			if len(bodies) != 2 {
				t.Fatalf("captured bodies = %d, want 2", len(bodies))
			}
			parentID, parentCount := claudePromptIDBillingField(bodies[0], "cc_prompt_id")
			childID, childCount := claudePromptIDBillingField(bodies[1], "cc_prompt_id")
			if parentCount != 1 || childCount != 1 || parentID == "" || childID != parentID {
				t.Fatalf("parent/child prompt IDs = (%q,%d)/(%q,%d), want one shared ID", parentID, parentCount, childID, childCount)
			}
			if got, count := claudePromptIDBillingField(bodies[1], "cc_is_subagent"); got != "true" || count != 1 {
				t.Fatalf("child subagent marker = %q count=%d, want true exactly once", got, count)
			}
		})
	}
}

func TestClaudeExecutorPromptIDUnmarkedAgentRemainsIsolated(t *testing.T) {
	cfg, auth, parentRequest, parentOptions := claudePrevRequestFixture(t, "s5-3-1b-isolated", "66666666-7777-4888-9999-aaaaaaaaaaaa")
	childRequest := parentRequest
	childPayload := bytes.Replace(parentRequest.Payload, []byte(`"test"`), []byte(`"different unmarked child"`), 1)
	childRequest.Payload = childPayload
	childOptions := parentOptions
	childOptions.OriginalRequest = childPayload
	childOptions.Headers = parentOptions.Headers.Clone()
	childOptions.Headers.Set("X-Claude-Code-Agent-Id", "subagent-1")
	var bodies [][]byte
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body, errRead := io.ReadAll(req.Body)
		if errRead != nil {
			return nil, errRead
		}
		bodies = append(bodies, body)
		return claudePrevRequestSuccessResponse(req, fmt.Sprintf("req_s531b_iso_%d", len(bodies))), nil
	})
	executor := NewClaudeExecutor(cfg)
	if errParent := claudePromptIDInvoke(false, transport, executor, auth, parentRequest, parentOptions); errParent != nil {
		t.Fatalf("parent request error = %v", errParent)
	}
	if errChild := claudePromptIDInvoke(false, transport, executor, auth, childRequest, childOptions); errChild != nil {
		t.Fatalf("unmarked child request error = %v", errChild)
	}
	if len(bodies) != 2 {
		t.Fatalf("captured bodies = %d, want 2", len(bodies))
	}
	parentID, parentCount := claudePromptIDBillingField(bodies[0], "cc_prompt_id")
	childID, childCount := claudePromptIDBillingField(bodies[1], "cc_prompt_id")
	if parentCount != 1 || childCount != 1 || parentID == "" || childID == "" || childID == parentID {
		t.Fatalf("unmarked parent/child IDs = (%q,%d)/(%q,%d), want distinct IDs", parentID, parentCount, childID, childCount)
	}
}

func TestClaudeExecutorPromptIDRetryAfterRoundTripperFailureKeepsID(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			cfg, auth, request, options := claudePrevRequestFixture(t, "s5-3-failure-"+fmt.Sprint(stream), "44444444-5555-4666-8777-888888888888")
			var bodies [][]byte
			calls := 0
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				body, errRead := io.ReadAll(req.Body)
				if errRead != nil {
					return nil, errRead
				}
				bodies = append(bodies, body)
				if calls == 1 {
					return nil, fmt.Errorf("synthetic transport failure")
				}
				if stream {
					return claudePrevRequestStreamResponse(req, "req_s523_failure_2", claudePrevRequestCompleteSSE("msg_s523_failure_2")), nil
				}
				return claudePrevRequestSuccessResponse(req, "req_s523_failure_2"), nil
			})
			executor := NewClaudeExecutor(cfg)
			if errFirst := claudePromptIDInvoke(stream, transport, executor, auth, request, options); errFirst == nil {
				t.Fatal("first request error = nil, want synthetic transport failure")
			}
			if errSecond := claudePromptIDInvoke(stream, transport, executor, auth, request, options); errSecond != nil {
				t.Fatalf("retry request error = %v", errSecond)
			}
			if len(bodies) != 2 {
				t.Fatalf("captured bodies = %d, want 2", len(bodies))
			}
			firstID, firstCount := claudePromptIDBillingField(bodies[0], "cc_prompt_id")
			secondID, secondCount := claudePromptIDBillingField(bodies[1], "cc_prompt_id")
			if firstCount != 1 || secondCount != 1 || firstID == "" || firstID != secondID {
				t.Fatalf("retry prompt IDs = (%q,%d)/(%q,%d), want one stable UUID", firstID, firstCount, secondID, secondCount)
			}
		})
	}
}

func TestClaudeExecutorPromptIDRejectsInvalidBeforeRoundTrip(t *testing.T) {
	invalidFields := []struct {
		name   string
		fields string
	}{
		{name: "malformed", fields: " cc_prompt_id=bad value;"},
		{name: "duplicate", fields: " cc_prompt_id=" + claudePromptIDIntegrationValue + "; cc_prompt_id=" + claudePromptIDIntegrationValue + ";"},
	}
	for _, stream := range []bool{false, true} {
		for _, invalid := range invalidFields {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, invalid.name), func(t *testing.T) {
				cfg, auth, request, options := claudePrevRequestFixture(t, "s5-2-3-invalid-"+fmt.Sprint(stream)+"-"+invalid.name, "33333333-4444-4555-8666-777777777777")
				invalidRequest, invalidOptions := claudePromptIDWithBillingFields(t, request, options, invalid.fields)
				calls := 0
				transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					calls++
					if stream {
						return claudePrevRequestStreamResponse(req, "req_s523_seed", claudePrevRequestCompleteSSE("msg_s523_seed")), nil
					}
					return claudePrevRequestSuccessResponse(req, "req_s523_seed"), nil
				})
				executor := NewClaudeExecutor(cfg)
				if errSeed := claudePromptIDInvoke(stream, transport, executor, auth, request, options); errSeed != nil {
					t.Fatalf("seed request error = %v", errSeed)
				}
				if errInvalid := claudePromptIDInvoke(stream, transport, executor, auth, invalidRequest, invalidOptions); errInvalid == nil {
					t.Fatal("invalid caller prompt ID error = nil")
				}
				if calls != 1 {
					t.Fatalf("RoundTripper calls = %d, want 1 seed call only", calls)
				}
			})
		}
	}
}
