package executor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
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
		&cliproxyauth.Auth{Attributes: map[string]string{
			"header:User-Agent":       "evil-client/9.9",
			"header:X-Stainless-Os":   "Linux",
			"header:X-Stainless-Arch": "arm64",
		}},
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

func TestResolvedClaudeSoftwareProfileHeadersUseResolvedDeviceWithoutStabilization(t *testing.T) {
	profile := helps.ResolvedClaudeSoftwareProfile{
		Device: helps.ClaudeDeviceProfile{
			UserAgent:      "claude-cli/2.1.241 (external, sdk-cli)",
			PackageVersion: "0.112.1",
			RuntimeVersion: "v26.3.0",
			OS:             "Windows",
			Arch:           "x64",
		},
		Entrypoint: "sdk-cli",
		Confirmed:  true,
		Provenance: helps.ClaudeSoftwareProfileDetected,
	}
	req, errRequest := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(`{}`))
	if errRequest != nil {
		t.Fatalf("http.NewRequest() error = %v", errRequest)
	}
	if errHeaders := applyClaudeHeadersWithResolvedProfile(req, nil, "key-no-stabilization", false, nil, nil, &config.Config{}, nil, profile, false); errHeaders != nil {
		t.Fatalf("applyClaudeHeadersWithResolvedProfile() error = %v", errHeaders)
	}
	if req.Header.Get("User-Agent") != profile.Device.UserAgent ||
		req.Header.Get("X-Stainless-Package-Version") != profile.Device.PackageVersion ||
		req.Header.Get("X-Stainless-Runtime-Version") != profile.Device.RuntimeVersion ||
		req.Header.Get("X-Stainless-Os") != profile.Device.OS ||
		req.Header.Get("X-Stainless-Arch") != profile.Device.Arch {
		t.Fatalf("resolved device was not applied without stabilization: %#v", req.Header)
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

func TestResolvedClaudeSoftwareProfileExecuteStreamParity(t *testing.T) {
	const (
		userAgent = "claude-cli/2.1.241 (external, sdk-cli)"
		userID    = `{"device_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","account_uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","session_id":"11111111-2222-4333-8444-555555555555"}`
	)
	type observed struct {
		headers http.Header
		body    []byte
	}
	observedRequests := make(chan observed, 2)
	var requestNumber int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		observedRequests <- observed{headers: r.Header.Clone(), body: body}
		if atomic.AddInt32(&requestNumber, 1) == 2 {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","model":"claude-opus-4-6","role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()
	stabilize := true
	cfg := &config.Config{ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
		UserAgent:              userAgent,
		PackageVersion:         "0.112.1",
		RuntimeVersion:         "v26.3.0",
		OS:                     "Windows",
		Arch:                   "x64",
		StabilizeDeviceProfile: &stabilize,
	}}
	incoming := http.Header{
		"User-Agent":                  {userAgent},
		"X-App":                       {"cli"},
		"Anthropic-Beta":              {"claude-code-20250219"},
		"X-Claude-Code-Session-Id":    {"11111111-2222-4333-8444-555555555555"},
		"X-Stainless-Package-Version": {"0.112.1"},
		"X-Stainless-Runtime-Version": {"v26.3.0"},
		"X-Stainless-Os":              {"Windows"},
		"X-Stainless-Arch":            {"x64"},
	}
	encodedUserID, _ := json.Marshal(userID)
	payload := []byte(`{"model":"claude-opus-4-6","messages":[{"role":"user","content":"x"}],"metadata":{"user_id":` + string(encodedUserID) + `}}`)
	for _, stream := range []bool{false, true} {
		helps.ResetClaudeDeviceProfileCache()
		metadata := claudeOAuthTestMetadata()
		metadata["access_token"] = claudeRaceProbeOAuthKey
		auth := &cliproxyauth.Auth{
			ID:         "resolved-profile-parity",
			Attributes: map[string]string{"base_url": server.URL},
			Metadata:   metadata,
		}
		executor := NewClaudeExecutor(cfg)
		req := cliproxyexecutor.Request{Model: "claude-opus-4-6", Payload: payload}
		opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude, OriginalRequest: payload, Headers: incoming, Stream: stream}
		if stream {
			result, errStream := executor.ExecuteStream(context.Background(), auth, req, opts)
			if errStream != nil {
				t.Fatalf("ExecuteStream() error = %v", errStream)
			}
			for chunk := range result.Chunks {
				if chunk.Err != nil {
					t.Fatalf("stream chunk error = %v", chunk.Err)
				}
			}
			continue
		}
		if _, errExecute := executor.Execute(context.Background(), auth, req, opts); errExecute != nil {
			t.Fatalf("Execute() error = %v", errExecute)
		}
	}
	observations := map[string]observed{"execute": <-observedRequests, "stream": <-observedRequests}

	for _, mode := range []string{"execute", "stream"} {
		got, ok := observations[mode]
		if !ok {
			t.Fatalf("missing %s observation", mode)
		}
		if got.headers.Get("User-Agent") != userAgent ||
			got.headers.Get("X-Stainless-Package-Version") != "0.112.1" ||
			got.headers.Get("X-Stainless-Runtime-Version") != "v26.3.0" ||
			got.headers.Get("X-Stainless-Os") != "Windows" ||
			got.headers.Get("X-Stainless-Arch") != "x64" ||
			got.headers.Get("X-App") != "cli" {
			t.Fatalf("%s headers do not match resolved sdk-cli profile: %#v", mode, got.headers)
		}
		billing := gjson.GetBytes(got.body, "system.0.text").String()
		if !strings.Contains(billing, "cc_version=2.1.241.") ||
			!strings.Contains(billing, "cc_entrypoint=sdk-cli;") ||
			strings.Contains(billing, "cch=00000;") {
			t.Fatalf("%s billing = %q, want resolved sdk-cli version and signed CCH", mode, billing)
		}
	}
}

func TestResolvedClaudeSoftwareProfileRejectsFinalBodyMutation(t *testing.T) {
	const userID = `{"device_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","account_uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","session_id":"11111111-2222-4333-8444-555555555555"}`
	encodedUserID, _ := json.Marshal(userID)
	payload := []byte(`{"model":"claude-opus-4-6","messages":[{"role":"user","content":"x"}],"metadata":{"user_id":` + string(encodedUserID) + `}}`)
	mutatedBilling := "x-anthropic-billing-header: cc_version=2.1.220.test; cc_entrypoint=cli; cch=00000;"
	cfg := &config.Config{
		Payload: config.PayloadConfig{
			Override: []config.PayloadRule{
				{
					Models: []config.PayloadModelRule{{Name: "claude-opus-4-6", Protocol: "claude"}},
					Params: map[string]any{
						"system": []any{map[string]any{"type": "text", "text": mutatedBilling}},
					},
				},
			},
		},
	}
	incoming := http.Header{
		"User-Agent":     {"claude-cli/2.1.220 (external, sdk-cli)"},
		"X-App":          {"cli"},
		"Anthropic-Beta": {"claude-code-20250219"},
	}
	executor := NewClaudeExecutor(cfg)
	auth := &cliproxyauth.Auth{ID: "resolved-profile-final-mutation", Attributes: map[string]string{
		"api_key":  claudeRaceProbeOAuthKey,
		"base_url": "https://api.anthropic.com",
	}, Metadata: claudeOAuthTestMetadata()}
	_, errExecute := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{Model: "claude-opus-4-6", Payload: payload}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FormatClaude, OriginalRequest: payload, Headers: incoming,
	})
	if errExecute == nil || !strings.Contains(errExecute.Error(), "conflicts with resolved entrypoint") {
		t.Fatalf("Execute() error = %v, want final body entrypoint conflict", errExecute)
	}
}

func boolPointer(value bool) *bool {
	return &value
}
