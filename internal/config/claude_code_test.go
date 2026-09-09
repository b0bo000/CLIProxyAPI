package config

import "testing"

func TestParseConfigBytesClaudeCodeModelListCloaking(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{
			name: "defaults to enabled cloaking",
			yaml: "port: 8317\n",
			want: false,
		},
		{
			name: "disables model list cloaking",
			yaml: "claude-code:\n  disable-cloaking-model-list: true\n",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, errParse := ParseConfigBytes([]byte(tt.yaml))
			if errParse != nil {
				t.Fatalf("ParseConfigBytes() error = %v", errParse)
			}
			if got := cfg.ClaudeCode.DisableCloakingModelList; got != tt.want {
				t.Fatalf("DisableCloakingModelList = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestParseConfigBytesClaudeCodeTLSSessionResumption(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want *bool
	}{
		{name: "omitted inherits", yaml: "port: 8317\n", want: nil},
		{name: "enabled", yaml: "claude-code:\n  tls-session-resumption: true\n", want: claudeBoolPtr(true)},
		{name: "disabled", yaml: "claude-code:\n  tls-session-resumption: false\n", want: claudeBoolPtr(false)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, errParse := ParseConfigBytes([]byte(tt.yaml))
			if errParse != nil {
				t.Fatalf("ParseConfigBytes() error = %v", errParse)
			}
			got := cfg.ClaudeCode.TLSSessionResumption
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Fatalf("TLSSessionResumption = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseConfigBytesClaudeCodeSessionScopedTransport(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{name: "omitted is disabled", yaml: "port: 8317\n", want: false},
		{name: "enabled", yaml: "claude-code:\n  session-scoped-transport: true\n", want: true},
		{name: "explicitly disabled", yaml: "claude-code:\n  session-scoped-transport: false\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, errParse := ParseConfigBytes([]byte(tt.yaml))
			if errParse != nil {
				t.Fatalf("ParseConfigBytes() error = %v", errParse)
			}
			if got := cfg.ClaudeCode.SessionScopedTransport; got != tt.want {
				t.Fatalf("SessionScopedTransport = %t, want %t", got, tt.want)
			}
		})
	}
}

func claudeBoolPtr(v bool) *bool { return &v }
