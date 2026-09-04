package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureClaudeUpstreamRequest(t *testing.T) {
	t.Setenv(claudeCaptureDirEnv, t.TempDir())
	claudeCaptureSequence = 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"stream":true}` {
			t.Fatalf("request body = %q", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/messages?beta=true", strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := captureClaudeUpstreamRequest(server.Client(), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	if err = resp.Body.Close(); err != nil {
		t.Fatal(err)
	}

	requestDir := filepath.Join(os.Getenv(claudeCaptureDirEnv), "000001")
	for _, name := range []string{"request.json", "request-body.raw", "response.json", "response-body.raw", "result.json"} {
		if _, err = os.Stat(filepath.Join(requestDir, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	responseBody, err := os.ReadFile(filepath.Join(requestDir, "response-body.raw"))
	if err != nil {
		t.Fatal(err)
	}
	if string(responseBody) != "data: {\"type\":\"message_stop\"}\n\n" {
		t.Fatalf("captured response body = %q", responseBody)
	}
	requestJSON, err := os.ReadFile(filepath.Join(requestDir, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(requestJSON, []byte("test-token")) || !bytes.Contains(requestJSON, []byte("[redacted sha256=")) {
		t.Fatalf("captured request did not redact authorization: %s", requestJSON)
	}
}

func TestClaudeCaptureStagesAndAssociations(t *testing.T) {
	captureRoot := t.TempDir()
	t.Setenv(claudeCaptureDirEnv, captureRoot)
	claudeCaptureSequence = 0
	headers := http.Header{
		"X-Claude-Code-Session-Id": []string{"session-a"},
		"X-Client-Request-Id":      []string{"request-a"},
	}
	incoming := []byte(`{"model":"m","system":[{"type":"text","text":"base"}],"messages":[{"role":"user","content":"hello"}]}`)
	ctx := beginClaudeCapture(context.Background(), "messages-stream", headers, incoming, "session-a", "credential-a", "direct")
	stages := []string{"translated", "after_cloak", "after_context_management", "after_diagnostics", "after_identity", "after_cch"}
	body := []byte(`{"model":"m","system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.241.abc; cc_entrypoint=cli; cch=00000; cc_prev_req=req-prev;"}],"messages":[{"role":"user","content":"hello"}],"tools":[{"name":"Read"}],"context_management":{},"diagnostics":{"previous_message_id":"msg-prev"}}`)
	for _, stage := range stages {
		captureClaudeStage(ctx, stage, body, nil)
	}

	requestDir := filepath.Join(captureRoot, "000001")
	for _, prefix := range []string{"01-incoming", "02-translated", "03-after_cloak", "04-after_context_management", "05-after_diagnostics", "06-after_identity", "07-after_cch"} {
		for _, suffix := range []string{".json", ".body.raw"} {
			if _, err := os.Stat(filepath.Join(requestDir, prefix+suffix)); err != nil {
				t.Fatalf("stage artifact %s: %v", prefix+suffix, err)
			}
		}
	}
	var record claudeCapturedStage
	data, err := os.ReadFile(filepath.Join(requestDir, "07-after_cch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.Association.SessionSHA256 != claudeCaptureHash("session-a") || record.Association.ClientRequestSHA256 != claudeCaptureHash("request-a") {
		t.Fatalf("request association = %#v", record.Association)
	}
	if record.Association.CCPreviousReqSHA256 != claudeCaptureHash("req-prev") || record.Association.DiagnosticsPrevSHA256 != claudeCaptureHash("msg-prev") {
		t.Fatalf("chain association = %#v", record.Association)
	}
}

func TestCaptureClaudeHTTPTraceMapsConnectionReuse(t *testing.T) {
	captureRoot := t.TempDir()
	t.Setenv(claudeCaptureDirEnv, captureRoot)
	claudeCaptureSequence = 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg-test"}`)
	}))
	defer server.Close()

	client := server.Client()
	for index := 0; index < 2; index++ {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/messages?beta=true", strings.NewReader(`{"stream":false}`))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := captureClaudeUpstreamRequest(client, req)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.Copy(io.Discard, resp.Body); err != nil {
			t.Fatal(err)
		}
		if err = resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}
	readTrace := func(sequence int) claudeHTTPTraceRecord {
		path := filepath.Join(captureRoot, fmt.Sprintf("%06d", sequence), "httptrace.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var record claudeHTTPTraceRecord
		if err = json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		return record
	}
	first := readTrace(1)
	second := readTrace(2)
	if first.Reused {
		t.Fatal("first request unexpectedly reused a connection")
	}
	if !second.Reused || first.ConnectionSHA256 == "" || first.ConnectionSHA256 != second.ConnectionSHA256 {
		t.Fatalf("reuse mapping: first=%#v second=%#v", first, second)
	}
	if _, err := os.Stat(filepath.Join(captureRoot, "000002", "08-final_roundtrip.json")); err != nil {
		t.Fatalf("final roundtrip stage: %v", err)
	}
}
