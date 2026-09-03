package helps

import "testing"

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
