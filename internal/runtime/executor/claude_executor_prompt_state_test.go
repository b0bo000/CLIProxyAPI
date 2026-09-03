package executor

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/tidwall/gjson"
)

func TestClaudeExecutorsRejectMalformedPromptBoundaryIdentically(t *testing.T) {
	t.Parallel()

	ctx := cliproxyexecutor.WithPromptBoundary(context.Background(), cliproxyexecutor.PromptBoundaryHint{
		Kind:          cliproxyexecutor.PromptBoundaryContinue,
		TransactionID: "invalid transaction",
	})
	executor := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{}
	request := cliproxyexecutor.Request{Model: "claude-sonnet-4-6"}
	options := cliproxyexecutor.Options{}

	_, executeErr := executor.Execute(ctx, auth, request, options)
	_, streamErr := executor.ExecuteStream(ctx, auth, request, options)
	for name, errRun := range map[string]error{"Execute": executeErr, "ExecuteStream": streamErr} {
		if errRun == nil {
			t.Fatalf("%s error = nil, want malformed prompt-boundary rejection", name)
		}
		var status interface{ StatusCode() int }
		if !errors.As(errRun, &status) || status.StatusCode() != http.StatusBadRequest {
			t.Fatalf("%s error = %T %v, want HTTP 400", name, errRun, errRun)
		}
		var scoped interface{ IsRequestScoped() bool }
		if !errors.As(errRun, &scoped) || !scoped.IsRequestScoped() {
			t.Fatalf("%s error = %T %v, want request-scoped", name, errRun, errRun)
		}
	}
}

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
