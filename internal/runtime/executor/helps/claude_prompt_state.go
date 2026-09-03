package helps

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

const ClaudePromptIDBillingField = "cc_prompt_id"

// ClaudePromptBoundary is a validated adapter-owned prompt lifecycle event.
// TransactionID is a correlation key only; it is never an upstream prompt ID.
type ClaudePromptBoundary struct {
	Kind          cliproxyexecutor.PromptBoundaryKind
	TransactionID string
}

type claudePromptBoundaryRequestError struct {
	cause error
}

func (e *claudePromptBoundaryRequestError) Error() string {
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func (e *claudePromptBoundaryRequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *claudePromptBoundaryRequestError) StatusCode() int {
	if e == nil {
		return 0
	}
	return http.StatusBadRequest
}

func (e *claudePromptBoundaryRequestError) IsRequestScoped() bool {
	return e != nil
}

// ResolveClaudePromptBoundary validates an internal caller-adapter contract.
// An absent context value and an explicit absent value have the same result.
// This function does not allocate, store, or serialize a prompt ID.
func ResolveClaudePromptBoundary(ctx context.Context) (ClaudePromptBoundary, error) {
	hint, found := cliproxyexecutor.PromptBoundaryFromContext(ctx)
	if !found {
		return ClaudePromptBoundary{Kind: cliproxyexecutor.PromptBoundaryAbsent}, nil
	}

	boundary := ClaudePromptBoundary{Kind: hint.Kind, TransactionID: hint.TransactionID}
	switch hint.Kind {
	case cliproxyexecutor.PromptBoundaryAbsent, cliproxyexecutor.PromptBoundaryAmbiguous:
		if hint.TransactionID != "" {
			return ClaudePromptBoundary{}, newClaudePromptBoundaryRequestError(
				fmt.Errorf("Claude prompt boundary %q must not carry a transaction ID", hint.Kind),
			)
		}
		return boundary, nil
	case cliproxyexecutor.PromptBoundaryNew, cliproxyexecutor.PromptBoundaryContinue:
		if errValidate := validateClaudePromptTransactionID(hint.TransactionID); errValidate != nil {
			return ClaudePromptBoundary{}, newClaudePromptBoundaryRequestError(errValidate)
		}
		return boundary, nil
	default:
		return ClaudePromptBoundary{}, newClaudePromptBoundaryRequestError(
			fmt.Errorf("unsupported Claude prompt boundary kind %q", hint.Kind),
		)
	}
}

func newClaudePromptBoundaryRequestError(err error) error {
	if err == nil {
		return nil
	}
	return &claudePromptBoundaryRequestError{cause: err}
}

func validateClaudePromptTransactionID(value string) error {
	if value == "" {
		return fmt.Errorf("Claude prompt transaction ID is empty")
	}
	if len(value) > 128 {
		return fmt.Errorf("Claude prompt transaction ID is too long")
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("Claude prompt transaction ID is not valid UTF-8")
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return fmt.Errorf("Claude prompt transaction ID contains invalid whitespace or control characters")
		}
	}
	return nil
}

// ParseClaudePromptIDBillingText reads a caller-owned prompt ID from one
// Claude billing block. It does not generate a value. The boolean distinguishes
// an absent field from a present field whose value is empty or invalid.
func ParseClaudePromptIDBillingText(text string) (string, bool, error) {
	trimmed := strings.TrimSpace(text)
	const prefix = "x-anthropic-billing-header:"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", false, nil
	}

	seen := false
	value := ""
	for _, segment := range strings.Split(trimmed[len(prefix):], ";") {
		part := strings.TrimSpace(segment)
		if part == "" {
			continue
		}
		key, fieldValue, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return "", false, fmt.Errorf("parse Claude cc_prompt_id: malformed billing segment %q", part)
		}
		if strings.TrimSpace(key) != ClaudePromptIDBillingField {
			continue
		}
		if seen {
			return "", false, fmt.Errorf("parse Claude cc_prompt_id: duplicate %s", ClaudePromptIDBillingField)
		}
		seen = true
		value = strings.TrimSpace(fieldValue)
		if errValidate := validateClaudePromptID(value); errValidate != nil {
			return "", false, errValidate
		}
	}
	if !seen {
		return "", false, nil
	}
	return value, true, nil
}

func validateClaudePromptID(value string) error {
	if value == "" {
		return fmt.Errorf("parse Claude cc_prompt_id: empty prompt ID")
	}
	if len(value) > 128 {
		return fmt.Errorf("parse Claude cc_prompt_id: prompt ID is too long")
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("parse Claude cc_prompt_id: prompt ID is not valid UTF-8")
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) || r == ';' || r == '=' || r == ',' {
			return fmt.Errorf("parse Claude cc_prompt_id: invalid prompt ID")
		}
	}
	return nil
}
