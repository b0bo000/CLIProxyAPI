package helps

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

func TestResolveClaudePromptBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		withHint    bool
		hint        cliproxyexecutor.PromptBoundaryHint
		want        ClaudePromptBoundary
		wantErrText string
	}{
		{
			name: "missing is absent",
			want: ClaudePromptBoundary{Kind: cliproxyexecutor.PromptBoundaryAbsent},
		},
		{
			name:     "explicit absent",
			withHint: true,
			hint:     cliproxyexecutor.PromptBoundaryHint{Kind: cliproxyexecutor.PromptBoundaryAbsent},
			want:     ClaudePromptBoundary{Kind: cliproxyexecutor.PromptBoundaryAbsent},
		},
		{
			name:     "ambiguous",
			withHint: true,
			hint:     cliproxyexecutor.PromptBoundaryHint{Kind: cliproxyexecutor.PromptBoundaryAmbiguous},
			want:     ClaudePromptBoundary{Kind: cliproxyexecutor.PromptBoundaryAmbiguous},
		},
		{
			name:     "new transaction",
			withHint: true,
			hint: cliproxyexecutor.PromptBoundaryHint{
				Kind:          cliproxyexecutor.PromptBoundaryNew,
				TransactionID: "transaction-1",
			},
			want: ClaudePromptBoundary{
				Kind:          cliproxyexecutor.PromptBoundaryNew,
				TransactionID: "transaction-1",
			},
		},
		{
			name:     "continue transaction",
			withHint: true,
			hint: cliproxyexecutor.PromptBoundaryHint{
				Kind:          cliproxyexecutor.PromptBoundaryContinue,
				TransactionID: "transaction-1",
			},
			want: ClaudePromptBoundary{
				Kind:          cliproxyexecutor.PromptBoundaryContinue,
				TransactionID: "transaction-1",
			},
		},
		{
			name:        "unknown kind",
			withHint:    true,
			hint:        cliproxyexecutor.PromptBoundaryHint{Kind: "retry", TransactionID: "transaction-1"},
			wantErrText: "unsupported Claude prompt boundary kind",
		},
		{
			name:        "new missing transaction",
			withHint:    true,
			hint:        cliproxyexecutor.PromptBoundaryHint{Kind: cliproxyexecutor.PromptBoundaryNew},
			wantErrText: "transaction ID is empty",
		},
		{
			name:        "continue whitespace transaction",
			withHint:    true,
			hint:        cliproxyexecutor.PromptBoundaryHint{Kind: cliproxyexecutor.PromptBoundaryContinue, TransactionID: "transaction 1"},
			wantErrText: "invalid whitespace",
		},
		{
			name:        "ambiguous with transaction",
			withHint:    true,
			hint:        cliproxyexecutor.PromptBoundaryHint{Kind: cliproxyexecutor.PromptBoundaryAmbiguous, TransactionID: "transaction-1"},
			wantErrText: "must not carry a transaction ID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			if test.withHint {
				ctx = cliproxyexecutor.WithPromptBoundary(ctx, test.hint)
			}
			got, errResolve := ResolveClaudePromptBoundary(ctx)
			if test.wantErrText == "" {
				if errResolve != nil || got != test.want {
					t.Fatalf("ResolveClaudePromptBoundary() = (%+v, %v), want (%+v, nil)", got, errResolve, test.want)
				}
				return
			}
			if errResolve == nil || !strings.Contains(errResolve.Error(), test.wantErrText) {
				t.Fatalf("ResolveClaudePromptBoundary() error = %v, want text %q", errResolve, test.wantErrText)
			}
			var status interface{ StatusCode() int }
			if !errors.As(errResolve, &status) || status.StatusCode() != http.StatusBadRequest {
				t.Fatalf("ResolveClaudePromptBoundary() error = %T %v, want HTTP 400", errResolve, errResolve)
			}
			var scoped interface{ IsRequestScoped() bool }
			if !errors.As(errResolve, &scoped) || !scoped.IsRequestScoped() {
				t.Fatalf("ResolveClaudePromptBoundary() error = %T %v, want request-scoped", errResolve, errResolve)
			}
		})
	}
}

func TestParseClaudePromptIDBillingText(t *testing.T) {
	t.Parallel()

	const promptID = "986813f9-290b-409e-8541-06cd11254627"
	tests := []struct {
		name      string
		text      string
		wantID    string
		wantFound bool
		wantErr   bool
	}{
		{
			name:      "valid UUID",
			text:      "x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli; cc_prompt_id=" + promptID + ";",
			wantID:    promptID,
			wantFound: true,
		},
		{
			name:      "valid ID with surrounding billing whitespace",
			text:      "  x-anthropic-billing-header: cc_entrypoint=sdk-cli; cc_prompt_id=prompt-1;  ",
			wantID:    "prompt-1",
			wantFound: true,
		},
		{
			name: "absent field",
			text: "x-anthropic-billing-header: cc_version=2.1.241.a; cc_entrypoint=sdk-cli;",
		},
		{
			name: "not a billing block",
			text: "ordinary system text",
		},
		{
			name:    "empty value",
			text:    "x-anthropic-billing-header: cc_entrypoint=sdk-cli; cc_prompt_id=;",
			wantErr: true,
		},
		{
			name:    "duplicate value",
			text:    "x-anthropic-billing-header: cc_prompt_id=one; cc_prompt_id=two;",
			wantErr: true,
		},
		{
			name:    "delimiter in value",
			text:    "x-anthropic-billing-header: cc_prompt_id=prompt;bad;",
			wantErr: true,
		},
		{
			name:    "malformed segment",
			text:    "x-anthropic-billing-header: cc_prompt_id=prompt-1; broken;",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotID, gotFound, errParse := ParseClaudePromptIDBillingText(test.text)
			if test.wantErr {
				if errParse == nil {
					t.Fatalf("error = nil, want validation error")
				}
				return
			}
			if errParse != nil {
				t.Fatalf("ParseClaudePromptIDBillingText() error = %v", errParse)
			}
			if gotID != test.wantID || gotFound != test.wantFound {
				t.Fatalf("got (%q, %v), want (%q, %v)", gotID, gotFound, test.wantID, test.wantFound)
			}
		})
	}
}
