package executor

import (
	"strings"
	"testing"

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
