package helps

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const ClaudePromptIDBillingField = "cc_prompt_id"

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
