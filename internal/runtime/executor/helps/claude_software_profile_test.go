package helps

import (
	"context"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
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
