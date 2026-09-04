package helps

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

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

func TestResolveClaudeSoftwareProfileReadsNonCanonicalHeaderNames(t *testing.T) {
	cfg := &config.Config{ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
		UserAgent:      "claude-cli/2.1.241 (external, sdk-cli)",
		PackageVersion: "0.112.1",
		RuntimeVersion: "v26.3.0",
		OS:             "Windows",
		Arch:           "x64",
	}}
	headers := http.Header{
		"user-agent":                  {"claude-cli/2.1.241 (external, sdk-cli)"},
		"x-app":                       {"cli"},
		"anthropic-beta":              {"claude-code-20250219"},
		"x-stainless-package-version": {"0.112.1"},
		"x-stainless-runtime-version": {"v26.3.0"},
		"x-stainless-os":              {"Linux"},
		"x-stainless-arch":            {"arm64"},
	}
	payload := []byte(`{"metadata":{"user_id":"{\"device_id\":\"0000000000000000000000000000000000000000000000000000000000000000\",\"account_uuid\":\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\",\"session_id\":\"11111111-2222-4333-8444-555555555555\"}"}}`)

	profile, errResolve := ResolveClaudeSoftwareProfile(context.Background(), nil, "", headers, payload, false, cfg, false)
	if errResolve != nil {
		t.Fatalf("ResolveClaudeSoftwareProfile() error = %v", errResolve)
	}
	if !profile.Confirmed || profile.Entrypoint != "sdk-cli" || profile.Device.UserAgent != headers["user-agent"][0] {
		t.Fatalf("resolved profile = %+v, want confirmed lowercase sdk-cli profile", profile)
	}
	if profile.Device.OS != "Linux" || profile.Device.Arch != "arm64" {
		t.Fatalf("resolved platform = %s/%s, want lowercase header values", profile.Device.OS, profile.Device.Arch)
	}
}

func TestResolveClaudeSoftwareProfileNormalizesUnstabilizedSoftwareTuple(t *testing.T) {
	cfg := &config.Config{ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
		UserAgent:      "claude-cli/2.1.241 (external, cli)",
		PackageVersion: "0.112.1",
		RuntimeVersion: "v26.3.0",
		OS:             "Windows",
		Arch:           "x64",
	}}
	headers := confirmedSoftwareProfileHeaders("sdk-cli")
	headers.Set("User-Agent", "claude-cli/2.1.241 (external, sdk-cli)")
	headers.Set("X-Stainless-Package-Version", "9.9.9")
	headers.Set("X-Stainless-Runtime-Version", "v9.9.9")
	headers.Set("X-Stainless-Os", "Linux")
	headers.Set("X-Stainless-Arch", "arm64")
	payload := []byte(`{"metadata":{"user_id":"{\"device_id\":\"0000000000000000000000000000000000000000000000000000000000000000\",\"account_uuid\":\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\",\"session_id\":\"11111111-2222-4333-8444-555555555555\"}"}}`)

	profile, errResolve := ResolveClaudeSoftwareProfile(context.Background(), nil, "", headers, payload, false, cfg, false)
	if errResolve != nil {
		t.Fatalf("ResolveClaudeSoftwareProfile() error = %v", errResolve)
	}
	if profile.Device.UserAgent != headers.Get("User-Agent") || profile.Entrypoint != "sdk-cli" {
		t.Fatalf("resolved identity = %+v, want detected sdk-cli User-Agent and entrypoint", profile)
	}
	if profile.Device.PackageVersion != "0.112.1" || profile.Device.RuntimeVersion != "v26.3.0" {
		t.Fatalf("resolved software tuple = %s/%s, want configured baseline", profile.Device.PackageVersion, profile.Device.RuntimeVersion)
	}
	if profile.Device.OS != "Linux" || profile.Device.Arch != "arm64" {
		t.Fatalf("resolved platform = %s/%s, want caller platform without stabilization", profile.Device.OS, profile.Device.Arch)
	}
}

func TestResolveConfiguredClaudeSoftwareProfileRequiresCoherentNativeUserAgent(t *testing.T) {
	for _, tc := range []struct {
		name      string
		userAgent string
		wantEntry string
		wantErr   bool
	}{
		{name: "cli", userAgent: "claude-cli/2.1.241 (external, cli)", wantEntry: "cli"},
		{name: "sdk cli", userAgent: "claude-cli/2.1.241 (external, sdk-cli)", wantEntry: "sdk-cli"},
		{name: "vscode", userAgent: "claude-cli/2.1.241 (external, claude-vscode)", wantEntry: "claude-vscode"},
		{name: "unsupported subclient", userAgent: "claude-cli/2.1.241 (external, sdk-ts)", wantErr: true},
		{name: "foreign agent", userAgent: "custom-client/2.1.241", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{UserAgent: tc.userAgent}}
			profile, errResolve := ResolveClaudeSoftwareProfile(context.Background(), nil, "", nil, nil, false, cfg, true)
			if (errResolve != nil) != tc.wantErr {
				t.Fatalf("ResolveClaudeSoftwareProfile() error = %v, wantErr=%v", errResolve, tc.wantErr)
			}
			if tc.wantErr {
				assertClaudeSoftwareProfileOperatorError(t, errResolve)
				return
			}
			if profile.Provenance != ClaudeSoftwareProfileConfiguredCLI || profile.Confirmed || profile.Entrypoint != tc.wantEntry {
				t.Fatalf("resolved profile = %+v, want configured unconfirmed entrypoint %q", profile, tc.wantEntry)
			}
		})
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
		{name: "duplicate version", billing: "x-anthropic-billing-header: cc_version=2.1.220.04c; cc_version=2.1.220.04c; cc_entrypoint=sdk-cli; cch=abcde;", wantErr: true},
		{name: "duplicate entrypoint", billing: "x-anthropic-billing-header: cc_version=2.1.220.04c; cc_entrypoint=sdk-cli; cc_entrypoint=sdk-cli; cch=abcde;", wantErr: true},
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
			if tc.wantErr {
				assertClaudeSoftwareProfileRequestError(t, errValidate)
			}
		})
	}
}

func TestValidateClaudeBillingSoftwareIdentityRejectsLaterBillingBlock(t *testing.T) {
	profile := ResolvedClaudeSoftwareProfile{
		Device:     defaultClaudeDeviceProfile(nil),
		Entrypoint: "cli",
		Provenance: ClaudeSoftwareProfileConfiguredCLI,
	}
	payload := []byte(`{"system":[{"type":"text","text":"ordinary"},{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.220.test; cc_entrypoint=cli; cch=abcde;"}]}`)
	errValidate := ValidateClaudeBillingSoftwareIdentity(payload, profile, nil)
	if errValidate == nil || !strings.Contains(errValidate.Error(), "first system block") {
		t.Fatalf("ValidateClaudeBillingSoftwareIdentity() error = %v, want later-block rejection", errValidate)
	}
	assertClaudeSoftwareProfileRequestError(t, errValidate)
}

func TestValidateClaudeBillingSoftwareIdentityHandlesStringAndWhitespace(t *testing.T) {
	profile := ResolvedClaudeSoftwareProfile{
		Device:     defaultClaudeDeviceProfile(nil),
		Entrypoint: "cli",
		Provenance: ClaudeSoftwareProfileConfiguredCLI,
	}
	matching := "  x-anthropic-billing-header: cc_version=2.1.220.test; cc_entrypoint=cli; cch=abcde;  "
	if errValidate := ValidateClaudeBillingSoftwareIdentity([]byte(`{"system":`+quoteJSON(matching)+`}`), profile, nil); errValidate != nil {
		t.Fatalf("string billing validation error = %v", errValidate)
	}
	conflicting := " x-anthropic-billing-header: cc_version=2.1.220.test; cc_entrypoint=sdk-cli; cch=abcde;"
	errValidate := ValidateClaudeBillingSoftwareIdentity([]byte(`{"system":`+quoteJSON(conflicting)+`}`), profile, nil)
	if errValidate == nil || !strings.Contains(errValidate.Error(), "conflicts with resolved entrypoint") {
		t.Fatalf("string billing conflict error = %v", errValidate)
	}
	assertClaudeSoftwareProfileRequestError(t, errValidate)
}

func quoteJSON(value string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
}

func assertClaudeSoftwareProfileRequestError(t *testing.T, err error) {
	t.Helper()
	var status interface{ StatusCode() int }
	if !errors.As(err, &status) || status.StatusCode() != http.StatusBadRequest {
		t.Fatalf("error = %T %v, want HTTP 400", err, err)
	}
	var scoped interface{ IsRequestScoped() bool }
	if !errors.As(err, &scoped) || !scoped.IsRequestScoped() {
		t.Fatalf("error = %T %v, want request-scoped", err, err)
	}
}

func assertClaudeSoftwareProfileOperatorError(t *testing.T, err error) {
	t.Helper()
	var status interface{ StatusCode() int }
	if errors.As(err, &status) {
		t.Fatalf("operator error = %T %v unexpectedly exposes HTTP status %d", err, err, status.StatusCode())
	}
	var scoped interface{ IsRequestScoped() bool }
	if errors.As(err, &scoped) && scoped.IsRequestScoped() {
		t.Fatalf("operator error = %T %v unexpectedly request-scoped", err, err)
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
	headers.Set("X-Stainless-Package-Version", "0.112.1")
	payload := []byte(`{"metadata":{"user_id":"{\"device_id\":\"0000000000000000000000000000000000000000000000000000000000000000\",\"account_uuid\":\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\",\"session_id\":\"11111111-2222-4333-8444-555555555555\"}"}}`)
	// Prime the sdk-cli scope with a conflicting cached CLI tuple to verify the
	// final authority check remains fail-closed even if persistence is corrupted.
	cacheKey := claudeDeviceProfileCacheKey(&cliproxyauth.Auth{ID: "entrypoint-drift"}, "key", ClaudeDeviceProfile{UserAgent: "claude-cli/2.1.241 (external, sdk-cli)"})
	claudeDeviceProfileCacheMu.Lock()
	claudeDeviceProfileCache[cacheKey] = claudeDeviceProfileCacheEntry{
		profile: defaultClaudeDeviceProfile(cfg),
		expire:  time.Now().Add(time.Hour),
	}
	claudeDeviceProfileCacheMu.Unlock()
	_, errResolve := ResolveClaudeSoftwareProfile(context.Background(), &cliproxyauth.Auth{ID: "entrypoint-drift"}, "key", headers, payload, false, cfg, false)
	if errResolve == nil || !strings.Contains(errResolve.Error(), "resolved device entrypoint") {
		t.Fatalf("ResolveClaudeSoftwareProfile() error = %v, want stabilized entrypoint drift", errResolve)
	}
	assertClaudeSoftwareProfileRequestError(t, errResolve)
}

func TestResolveClaudeSoftwareProfileMarksTitleRequestSeparately(t *testing.T) {
	profile, errResolve := ResolveClaudeSoftwareProfile(
		context.Background(),
		nil,
		"",
		measuredClaudeCodeTitleHeaders(),
		measuredClaudeCodeTitlePayload(),
		false,
		measuredClaudeCodeTitleConfig(),
		false,
	)
	if errResolve != nil {
		t.Fatalf("ResolveClaudeSoftwareProfile() error = %v", errResolve)
	}
	if !profile.Confirmed || !profile.IsTitleRequest() || profile.IsHelperProfile() {
		t.Fatalf("profile = %#v, want confirmed title request separate from helper", profile)
	}
}
