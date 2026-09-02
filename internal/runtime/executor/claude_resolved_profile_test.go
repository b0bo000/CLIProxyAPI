package executor

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	"github.com/tidwall/gjson"
)

func TestResolvedClaudeSoftwareProfileDrivesHeaderAndBillingAuthorities(t *testing.T) {
	profile := helps.ResolvedClaudeSoftwareProfile{
		Device: helps.ClaudeDeviceProfile{
			UserAgent:      "claude-cli/2.1.241 (external, sdk-cli)",
			PackageVersion: "0.112.1",
			RuntimeVersion: "v26.3.0",
			OS:             "Windows",
			Arch:           "x64",
		},
		Entrypoint: "sdk-cli",
		Subclient:  "claude-code-cli-sdk",
		Confirmed:  true,
		Provenance: helps.ClaudeSoftwareProfileDetected,
	}
	cfg := &config.Config{}
	cfg.ClaudeHeaderDefaults.StabilizeDeviceProfile = boolPointer(true)
	req, errRequest := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages?beta=true", strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"x"}]}`))
	if errRequest != nil {
		t.Fatalf("http.NewRequest() error = %v", errRequest)
	}
	if errHeaders := applyClaudeHeadersWithResolvedProfile(
		req,
		nil,
		"key-resolved-profile",
		false,
		nil,
		[]byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"x"}]}`),
		cfg,
		http.Header{},
		profile,
		false,
		"11111111-2222-4333-8444-555555555555",
	); errHeaders != nil {
		t.Fatalf("applyClaudeHeadersWithResolvedProfile() error = %v", errHeaders)
	}
	if got := req.Header.Get("User-Agent"); got != profile.Device.UserAgent {
		t.Fatalf("User-Agent = %q, want %q", got, profile.Device.UserAgent)
	}
	if got := req.Header.Get("X-Stainless-Package-Version"); got != profile.Device.PackageVersion {
		t.Fatalf("package version = %q, want %q", got, profile.Device.PackageVersion)
	}
	if got := req.Header.Get("X-Stainless-Os"); got != profile.Device.OS {
		t.Fatalf("OS = %q, want %q", got, profile.Device.OS)
	}
	if got := req.Header.Get("X-Stainless-Arch"); got != profile.Device.Arch {
		t.Fatalf("arch = %q, want %q", got, profile.Device.Arch)
	}
	billing := claudeCCHFallbackBillingHeaderWithProfile(context.Background(), cfg, []byte(`{"messages":[{"role":"user","content":"x"}]}`), profile)
	if !strings.Contains(billing, "cc_version=2.1.241.") || !strings.Contains(billing, "cc_entrypoint=sdk-cli;") {
		t.Fatalf("billing = %q, want resolved version and entrypoint", billing)
	}
}

func TestUnknownClaudeSoftwareProfileDoesNotBorrowEntrypoint(t *testing.T) {
	profile := helps.ResolvedClaudeSoftwareProfile{
		Device:     helps.ClaudeDeviceProfile{UserAgent: "claude-cli/2.1.241 (external, sdk-cli)"},
		Entrypoint: "sdk-cli",
		Provenance: helps.ClaudeSoftwareProfileUnknown,
	}
	billing := claudeCCHFallbackBillingHeaderWithProfile(context.Background(), &config.Config{}, []byte(`{"messages":[{"role":"user","content":"x"}]}`), profile)
	if !strings.Contains(billing, "cc_entrypoint=cli;") {
		t.Fatalf("unknown billing = %q, want cli fallback", billing)
	}
	if strings.Contains(billing, "cc_entrypoint=sdk-cli;") {
		t.Fatalf("unknown profile borrowed sdk-cli entrypoint: %q", billing)
	}
}

func TestResolvedClaudeSoftwareProfileCCHRecomputedAfterBodyMutation(t *testing.T) {
	profile := helps.ResolvedClaudeSoftwareProfile{
		Device:     helps.ClaudeDeviceProfile{UserAgent: "claude-cli/2.1.241 (external, claude-vscode)"},
		Entrypoint: "claude-vscode",
		Confirmed:  true,
		Provenance: helps.ClaudeSoftwareProfileDetected,
	}
	fallback := claudeCCHFallbackBillingHeaderWithProfile(context.Background(), &config.Config{}, []byte(`{"messages":[{"role":"user","content":"before"}]}`), profile)
	body := []byte(`{"system":[{"type":"text","text":"` + fallback + `"}],"messages":[{"role":"user","content":"after"}]}`)
	signed, errSign := finalizeAnthropicMessagesBodyCCH(body, fallback)
	if errSign != nil {
		t.Fatalf("finalizeAnthropicMessagesBodyCCH() error = %v", errSign)
	}
	billing := strings.TrimSpace(gjson.GetBytes(signed, "system.0.text").String())
	if strings.Contains(billing, "cch=00000;") || !strings.Contains(billing, "cc_entrypoint=claude-vscode;") {
		t.Fatalf("signed billing = %q, want recomputed CCH and preserved entrypoint", billing)
	}
}

func boolPointer(value bool) *bool {
	return &value
}
