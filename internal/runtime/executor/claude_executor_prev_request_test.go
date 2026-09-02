package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestInsertClaudePrevRequestBilling(t *testing.T) {
	t.Parallel()

	const previous = "req_previous_123"
	tests := []struct {
		name     string
		body     string
		wantPath string
		wantText string
		wantSame bool
		wantErr  string
	}{
		{
			name:     "first array billing block",
			body:     `{"model":"m","system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cch=00000;"},{"type":"text","text":"keep"}],"messages":[{"role":"user","content":"hi"}]}`,
			wantPath: "system.0.text",
			wantText: "x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cch=00000; cc_prev_req=req_previous_123;",
		},
		{
			name:     "non-first billing block",
			body:     `{"system":[{"type":"text","text":"ordinary"},{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cch=00000;"}]}`,
			wantPath: "system.1.text",
			wantText: "x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cch=00000; cc_prev_req=req_previous_123;",
		},
		{
			name:     "string billing preserves whitespace",
			body:     `{"system":"  x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cch=00000;  ","messages":[]}`,
			wantPath: "system",
			wantText: "  x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cch=00000; cc_prev_req=req_previous_123;  ",
		},
		{
			name:     "missing cch follows entrypoint",
			body:     `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cc_workload=agent;"}]}`,
			wantPath: "system.0.text",
			wantText: "x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cc_prev_req=req_previous_123; cc_workload=agent;",
		},
		{
			name:     "caller value remains byte identical",
			body:     `{"model":"m","system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cch=00000; cc_prev_req=req_caller;"}],"tail":{"n":1}}`,
			wantSame: true,
		},
		{
			name:     "no billing block is unchanged",
			body:     `{"system":[{"type":"text","text":"ordinary"}],"messages":[{"role":"user","content":"hi"}]}`,
			wantSame: true,
		},
		{
			name:    "malformed segment",
			body:    `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241.a; broken; cc_entrypoint=sdk-cli;"}]}`,
			wantErr: "malformed billing segment",
		},
		{
			name:    "missing entrypoint",
			body:    `{"system":"x-anthropic-billing-header: cc_version=2.1.241.a; cch=00000;"}`,
			wantErr: "no cc_entrypoint",
		},
		{
			name:    "malformed json",
			body:    `{"system":[`,
			wantErr: "malformed JSON",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(test.body)
			got, errInsert := insertClaudePrevRequestBilling(body, previous)
			if test.wantErr != "" {
				if errInsert == nil || !strings.Contains(errInsert.Error(), test.wantErr) {
					t.Fatalf("error = %v, want substring %q", errInsert, test.wantErr)
				}
				if !bytes.Equal(got, nil) {
					t.Fatalf("error result body = %q, want nil", got)
				}
				return
			}
			if errInsert != nil {
				t.Fatalf("insertClaudePrevRequestBilling() error = %v", errInsert)
			}
			if test.wantSame {
				if !bytes.Equal(got, body) {
					t.Fatalf("body changed for caller/no-op case\n got: %s\nwant: %s", got, body)
				}
				return
			}
			if gotText := gjson.GetBytes(got, test.wantPath).String(); gotText != test.wantText {
				t.Fatalf("billing text = %q, want %q", gotText, test.wantText)
			}
			if !gjson.ValidBytes(got) {
				t.Fatalf("updated body is invalid JSON: %s", got)
			}
			if gjson.GetBytes(got, "model").String() != gjson.GetBytes(body, "model").String() ||
				gjson.GetBytes(got, "messages").Raw != gjson.GetBytes(body, "messages").Raw {
				t.Fatalf("non-billing fields changed\n got: %s\nwant: %s", got, body)
			}
		})
	}
}

func TestInsertClaudePrevRequestBillingRejectsUnsafeID(t *testing.T) {
	t.Parallel()
	body := []byte(`{"system":"x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cch=00000;"}`)
	for _, id := range []string{"req;bad", "req=bad", "req\nnext", " req"} {
		if got, errInsert := insertClaudePrevRequestBilling(body, id); errInsert == nil || got != nil {
			t.Fatalf("id %q: got body=%q err=%v, want nil body and error", id, got, errInsert)
		}
	}
}
