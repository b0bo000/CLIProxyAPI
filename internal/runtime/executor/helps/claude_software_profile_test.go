package helps

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestResolveClaudeSoftwareProfileProvenance(t *testing.T) {
	cfg := &config.Config{}
	validID := `{"metadata":{"user_id":"{\"device_id\":\"0000000000000000000000000000000000000000000000000000000000000000\",\"account_uuid\":\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\",\"session_id\":\"11111111-2222-4333-8444-555555555555\"}"}}`
	confirmedPayload := []byte(validID)
	cases := []struct {
		name          string
		headers       http.Header
		payload       []byte
		configured    bool
		wantProv      ClaudeSoftwareProfileProvenance
		wantConfirmed bool
		wantEntry     string
		wantSubclient string
	}{
		{
			name:          "confirmed native cli",
			headers:       confirmedSoftwareProfileHeaders("cli"),
			payload:       confirmedPayload,
			wantProv:      ClaudeSoftwareProfileDetected,
			wantConfirmed: true,
			wantEntry:     "cli",
			wantSubclient: "claude-code-cli",
		},
		{
			name:          "confirmed native sdk cli",
			headers:       confirmedSoftwareProfileHeaders("sdk-cli"),
			payload:       confirmedPayload,
			wantProv:      ClaudeSoftwareProfileDetected,
			wantConfirmed: true,
			wantEntry:     "sdk-cli",
			wantSubclient: "claude-code-cli-sdk",
		},
		{
			name:          "confirmed native vscode",
			headers:       confirmedSoftwareProfileHeaders("claude-vscode"),
			payload:       confirmedPayload,
			wantProv:      ClaudeSoftwareProfileDetected,
			wantConfirmed: true,
			wantEntry:     "claude-vscode",
			wantSubclient: "claude-code-vscode",
		},
		{
			name:          "configured cli policy remains unconfirmed",
			configured:    true,
			wantProv:      ClaudeSoftwareProfileConfiguredCLI,
			wantConfirmed: false,
			wantEntry:     "cli",
			wantSubclient: "claude-code-cli",
		},
		{
			name:          "unknown caller",
			headers:       http.Header{"User-Agent": {"curl/8.0"}},
			configured:    false,
			wantProv:      ClaudeSoftwareProfileUnknown,
			wantConfirmed: false,
		},
		{
			name: "forged ua without strong signals",
			headers: http.Header{
				"User-Agent": {"claude-cli/2.1.220 (external, sdk-cli)"},
			},
			configured:    false,
			wantProv:      ClaudeSoftwareProfileUnknown,
			wantConfirmed: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, errResolve := ResolveClaudeSoftwareProfile(context.Background(), nil, "", tc.headers, tc.payload, false, cfg, tc.configured)
			if errResolve != nil {
				t.Fatalf("ResolveClaudeSoftwareProfile() error = %v", errResolve)
			}
			if got.Provenance != tc.wantProv || got.Confirmed != tc.wantConfirmed || got.Entrypoint != tc.wantEntry || got.Subclient != tc.wantSubclient {
				t.Fatalf("resolved profile = %+v, want provenance=%q confirmed=%v entrypoint=%q subclient=%q", got, tc.wantProv, tc.wantConfirmed, tc.wantEntry, tc.wantSubclient)
			}
			if got.Device.UserAgent == "" || got.Device.PackageVersion == "" || got.Device.RuntimeVersion == "" || got.Device.OS == "" || got.Device.Arch == "" {
				t.Fatalf("resolved device profile is incomplete: %+v", got.Device)
			}
		})
	}
}

func confirmedSoftwareProfileHeaders(entrypoint string) http.Header {
	return http.Header{
		"User-Agent":     {"claude-cli/2.1.220 (external, " + entrypoint + ")"},
		"X-App":          {"cli"},
		"Anthropic-Beta": {"claude-code-20250219"},
	}
}

func TestClaudeDeviceProfileVersionFallsBackToConfiguredBaseline(t *testing.T) {
	cfg := &config.Config{}
	cfg.ClaudeHeaderDefaults.UserAgent = "claude-cli/2.1.241 (external, sdk-cli)"
	profile := ClaudeDeviceProfile{UserAgent: "not-a-claude-user-agent"}
	if got := ClaudeDeviceProfileVersion(profile, cfg); got != "2.1.241" {
		t.Fatalf("ClaudeDeviceProfileVersion() = %q, want configured baseline 2.1.241", got)
	}
}

func TestValidateClaudeBillingSoftwareIdentityRejectsConflict(t *testing.T) {
	validMetadata := `"{\"device_id\":\"0000000000000000000000000000000000000000000000000000000000000000\",\"account_uuid\":\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\",\"session_id\":\"11111111-2222-4333-8444-555555555555\"}"`
	cases := []struct {
		name    string
		billing string
		wantErr bool
	}{
		{name: "matching", billing: "x-anthropic-billing-header: cc_version=2.1.220.04c; cc_entrypoint=sdk-cli; cch=abcde;"},
		{name: "entrypoint mismatch", billing: "x-anthropic-billing-header: cc_version=2.1.220.04c; cc_entrypoint=cli; cch=abcde;", wantErr: true},
		{name: "version mismatch", billing: "x-anthropic-billing-header: cc_version=2.1.241.04c; cc_entrypoint=sdk-cli; cch=abcde;", wantErr: true},
		{name: "missing entrypoint", billing: "x-anthropic-billing-header: cc_version=2.1.220.04c; cch=abcde;", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(`{"system":[{"type":"text","text":"` + tc.billing + `"}],"metadata":{"user_id":` + validMetadata + `}}`)
			profile, errResolve := ResolveClaudeSoftwareProfile(context.Background(), nil, "", confirmedSoftwareProfileHeaders("sdk-cli"), []byte(`{"metadata":{"user_id":`+validMetadata+`}}`), false, &config.Config{}, false)
			if errResolve != nil {
				t.Fatalf("ResolveClaudeSoftwareProfile() error = %v", errResolve)
			}
			errValidate := ValidateClaudeBillingSoftwareIdentity(payload, profile, &config.Config{})
			if (errValidate != nil) != tc.wantErr {
				t.Fatalf("ValidateClaudeBillingSoftwareIdentity() error = %v, wantErr=%v", errValidate, tc.wantErr)
			}
		})
	}
}

func TestConfiguredClaudeSoftwareProfileDoesNotBorrowDetectedCache(t *testing.T) {
	ResetClaudeDeviceProfileCache()
	client := newFakeClaudeDeviceProfileKVClient()
	useFakeClaudeDeviceProfileKVClient(t, client, false, nil)
	stabilize := true
	cfg := &config.Config{ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
		UserAgent:              "claude-cli/2.1.241 (external, sdk-cli)",
		PackageVersion:         "0.112.1",
		RuntimeVersion:         "v26.3.0",
		OS:                     "Windows",
		Arch:                   "x64",
		StabilizeDeviceProfile: &stabilize,
	}}
	incoming := confirmedSoftwareProfileHeaders("cli")
	incoming.Set("User-Agent", "claude-cli/2.1.241 (external, cli)")
	incoming.Set("X-Stainless-Package-Version", "0.112.1")
	incoming.Set("X-Stainless-Runtime-Version", "v26.3.0")
	auth := &cliproxyauth.Auth{ID: "software-profile-cache-order"}
	validPayload := []byte(`{"metadata":{"user_id":"{\"device_id\":\"0000000000000000000000000000000000000000000000000000000000000000\",\"account_uuid\":\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\",\"session_id\":\"11111111-2222-4333-8444-555555555555\"}"}}`)
	if _, errResolve := ResolveClaudeSoftwareProfile(context.Background(), auth, "key", incoming, validPayload, false, cfg, false); errResolve != nil {
		t.Fatalf("detected ResolveClaudeSoftwareProfile() error = %v", errResolve)
	}
	configured, errConfigured := ResolveClaudeSoftwareProfile(context.Background(), auth, "key", nil, nil, false, cfg, true)
	if errConfigured != nil {
		t.Fatalf("configured ResolveClaudeSoftwareProfile() error = %v", errConfigured)
	}
	if configured.Device.UserAgent != cfg.ClaudeHeaderDefaults.UserAgent || configured.Entrypoint != "sdk-cli" {
		t.Fatalf("configured profile borrowed detected cache: %+v", configured)
	}
}

func TestResolveClaudeSoftwareProfileRejectsStabilizedEntrypointDrift(t *testing.T) {
	ResetClaudeDeviceProfileCache()
	client := newFakeClaudeDeviceProfileKVClient()
	useFakeClaudeDeviceProfileKVClient(t, client, false, nil)
	stabilize := true
	cfg := &config.Config{ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
		UserAgent:              "claude-cli/2.1.241 (external, cli)",
		PackageVersion:         "0.112.1",
		RuntimeVersion:         "v26.3.0",
		OS:                     "Windows",
		Arch:                   "x64",
		StabilizeDeviceProfile: &stabilize,
	}}
	headers := confirmedSoftwareProfileHeaders("sdk-cli")
	headers.Set("User-Agent", "claude-cli/2.1.241 (external, sdk-cli)")
	headers.Set("X-Stainless-Package-Version", "0.1.0")
	payload := []byte(`{"metadata":{"user_id":"{\"device_id\":\"0000000000000000000000000000000000000000000000000000000000000000\",\"account_uuid\":\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\",\"session_id\":\"11111111-2222-4333-8444-555555555555\"}"}}`)
	_, errResolve := ResolveClaudeSoftwareProfile(context.Background(), &cliproxyauth.Auth{ID: "entrypoint-drift"}, "key", headers, payload, false, cfg, false)
	if errResolve == nil || !strings.Contains(errResolve.Error(), "resolved device entrypoint") {
		t.Fatalf("ResolveClaudeSoftwareProfile() error = %v, want stabilized entrypoint drift", errResolve)
	}
}
