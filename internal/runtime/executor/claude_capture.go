package executor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tls "github.com/refraction-networking/utls"
	"github.com/tidwall/gjson"
)

const claudeCaptureDirEnv = "CPA_CLAUDE_CAPTURE_DIR"
const claudeCaptureDisableDiagnosticsEnv = "CPA_CLAUDE_CAPTURE_DISABLE_DIAGNOSTICS"

var claudeCaptureSequence uint64

func claudeCaptureDiagnosticsEnabled(enabled bool) bool {
	return enabled && strings.TrimSpace(os.Getenv(claudeCaptureDisableDiagnosticsEnv)) != "1"
}

type claudeCaptureContextKey struct{}

type claudeCaptureState struct {
	Sequence    uint64                   `json:"sequence"`
	Endpoint    string                   `json:"endpoint"`
	StartedAt   string                   `json:"started_at"`
	RequestDir  string                   `json:"-"`
	Association claudeCaptureAssociation `json:"association"`
}

type claudeCaptureAssociation struct {
	SessionSHA256           string `json:"session_sha256,omitempty"`
	ClientRequestSHA256     string `json:"client_request_sha256,omitempty"`
	CredentialSHA256        string `json:"credential_sha256,omitempty"`
	CCPreviousReqSHA256     string `json:"cc_prev_req_sha256,omitempty"`
	DiagnosticsPrevSHA256   string `json:"diagnostics_previous_message_sha256,omitempty"`
	TransportKeySHA256      string `json:"transport_cache_key_sha256,omitempty"`
	TransportInstanceSHA256 string `json:"transport_instance_sha256,omitempty"`
}

type claudeCapturedStage struct {
	Sequence       uint64                   `json:"sequence"`
	Stage          string                   `json:"stage"`
	CapturedAt     string                   `json:"captured_at"`
	BodyBytes      int                      `json:"body_bytes"`
	BodySHA256     string                   `json:"body_sha256"`
	TopLevelFields []string                 `json:"top_level_fields,omitempty"`
	Model          string                   `json:"model,omitempty"`
	Messages       int                      `json:"messages"`
	SystemBlocks   int                      `json:"system_blocks"`
	Tools          int                      `json:"tools"`
	HasThinking    bool                     `json:"has_thinking"`
	HasContext     bool                     `json:"has_context_management"`
	HasDiagnostics bool                     `json:"has_diagnostics"`
	Headers        http.Header              `json:"headers,omitempty"`
	Association    claudeCaptureAssociation `json:"association"`
}

type claudeCapturedRequest struct {
	Sequence         uint64                   `json:"sequence"`
	StartedAt        string                   `json:"started_at"`
	Method           string                   `json:"method"`
	URL              string                   `json:"url"`
	Host             string                   `json:"host"`
	Proto            string                   `json:"proto"`
	ContentLength    int64                    `json:"content_length"`
	TransferEncoding []string                 `json:"transfer_encoding,omitempty"`
	Headers          http.Header              `json:"headers"`
	Association      claudeCaptureAssociation `json:"association"`
}

type claudeCapturedResponse struct {
	Sequence      uint64      `json:"sequence"`
	ReceivedAt    string      `json:"received_at"`
	Status        string      `json:"status"`
	StatusCode    int         `json:"status_code"`
	Proto         string      `json:"proto"`
	ContentLength int64       `json:"content_length"`
	Headers       http.Header `json:"headers"`
	Close         bool        `json:"close"`
}

type claudeCaptureResult struct {
	Sequence      uint64 `json:"sequence"`
	CompletedAt   string `json:"completed_at"`
	ElapsedMillis int64  `json:"elapsed_millis"`
	ResponseBytes int64  `json:"response_bytes"`
	ReadResult    string `json:"read_result"`
}

type claudeHTTPTraceRecord struct {
	Sequence             uint64                   `json:"sequence"`
	WrittenAt            string                   `json:"written_at"`
	GetConnAt            string                   `json:"get_conn_at,omitempty"`
	GotConnAt            string                   `json:"got_conn_at,omitempty"`
	GotFirstResponseAt   string                   `json:"got_first_response_byte_at,omitempty"`
	WroteRequestAt       string                   `json:"wrote_request_at,omitempty"`
	WroteRequestError    string                   `json:"wrote_request_error,omitempty"`
	LocalAddr            string                   `json:"local_addr,omitempty"`
	RemoteAddr           string                   `json:"remote_addr,omitempty"`
	ConnectionSHA256     string                   `json:"connection_sha256,omitempty"`
	Reused               bool                     `json:"reused"`
	WasIdle              bool                     `json:"was_idle"`
	IdleMillis           int64                    `json:"idle_millis"`
	TLSHandshakeComplete bool                     `json:"tls_handshake_complete"`
	TLSDidResume         bool                     `json:"tls_did_resume"`
	TLSVersion           uint16                   `json:"tls_version,omitempty"`
	TLSCipherSuite       uint16                   `json:"tls_cipher_suite,omitempty"`
	TLSALPN              string                   `json:"tls_alpn,omitempty"`
	Association          claudeCaptureAssociation `json:"association"`
}

type claudeHTTPTraceCollector struct {
	sync.Mutex
	record claudeHTTPTraceRecord
}

var claudeCaptureStageOrder = map[string]string{
	"incoming":                 "01-incoming",
	"translated":               "02-translated",
	"after_cloak":              "03-after_cloak",
	"after_context_management": "04-after_context_management",
	"after_diagnostics":        "05-after_diagnostics",
	"after_identity":           "06-after_identity",
	"after_cch":                "07-after_cch",
	"final_roundtrip":          "08-final_roundtrip",
}

func beginClaudeCapture(ctx context.Context, endpoint string, headers http.Header, body []byte, sessionID, credentialIdentity, transportCacheKey string) context.Context {
	captureRoot := strings.TrimSpace(os.Getenv(claudeCaptureDirEnv))
	if captureRoot == "" {
		return ctx
	}
	sequence := atomic.AddUint64(&claudeCaptureSequence, 1)
	requestDir := filepath.Join(captureRoot, fmt.Sprintf("%06d", sequence))
	if err := os.MkdirAll(requestDir, 0o700); err != nil {
		return ctx
	}
	state := &claudeCaptureState{
		Sequence:   sequence,
		Endpoint:   endpoint,
		StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		RequestDir: requestDir,
		Association: claudeCaptureAssociation{
			SessionSHA256:       claudeCaptureHash(sessionID),
			ClientRequestSHA256: claudeCaptureHash(headerValueFold(headers, "x-client-request-id")),
			CredentialSHA256:    claudeCaptureHash(credentialIdentity),
			TransportKeySHA256:  claudeCaptureHash(transportCacheKey),
		},
	}
	writeClaudeCaptureJSON(filepath.Join(requestDir, "capture.json"), state)
	ctx = context.WithValue(ctx, claudeCaptureContextKey{}, state)
	captureClaudeStage(ctx, "incoming", body, headers)
	return ctx
}

func captureClaudeStage(ctx context.Context, stage string, body []byte, headers http.Header) {
	state := claudeCaptureStateFromContext(ctx)
	filePrefix, ok := claudeCaptureStageOrder[stage]
	if state == nil || !ok {
		return
	}
	association := state.Association
	association.CCPreviousReqSHA256 = claudeCaptureHash(claudeCaptureBillingField(body, "cc_prev_req"))
	association.DiagnosticsPrevSHA256 = claudeCaptureHash(strings.TrimSpace(gjson.GetBytes(body, "diagnostics.previous_message_id").String()))
	if value := headerValueFold(headers, "x-client-request-id"); value != "" {
		association.ClientRequestSHA256 = claudeCaptureHash(value)
	}
	if value := headerValueFold(headers, "x-claude-code-session-id"); value != "" {
		association.SessionSHA256 = claudeCaptureHash(value)
	}
	root := gjson.ParseBytes(body)
	fields := make([]string, 0, 16)
	if root.IsObject() {
		root.ForEach(func(key, _ gjson.Result) bool {
			fields = append(fields, key.String())
			return true
		})
		sort.Strings(fields)
	}
	record := claudeCapturedStage{
		Sequence:       state.Sequence,
		Stage:          stage,
		CapturedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		BodyBytes:      len(body),
		BodySHA256:     claudeCaptureHashBytes(body),
		TopLevelFields: fields,
		Model:          root.Get("model").String(),
		Messages:       len(root.Get("messages").Array()),
		SystemBlocks:   claudeCaptureJSONBlockCount(root.Get("system")),
		Tools:          len(root.Get("tools").Array()),
		HasThinking:    root.Get("thinking").Exists(),
		HasContext:     root.Get("context_management").Exists(),
		HasDiagnostics: root.Get("diagnostics").Exists(),
		Association:    association,
	}
	if headers != nil {
		record.Headers = redactClaudeCaptureHeaders(headers)
	}
	_ = os.WriteFile(filepath.Join(state.RequestDir, filePrefix+".body.raw"), body, 0o600)
	writeClaudeCaptureJSON(filepath.Join(state.RequestDir, filePrefix+".json"), record)
}

func claudeCaptureStateFromContext(ctx context.Context) *claudeCaptureState {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(claudeCaptureContextKey{}).(*claudeCaptureState)
	return state
}

func claudeCaptureJSONBlockCount(value gjson.Result) int {
	if value.IsArray() {
		return len(value.Array())
	}
	if value.Exists() {
		return 1
	}
	return 0
}

func claudeCaptureBillingField(body []byte, name string) string {
	var billing string
	gjson.GetBytes(body, "system").ForEach(func(_, block gjson.Result) bool {
		text := block.Get("text").String()
		if strings.HasPrefix(text, "x-anthropic-billing-header:") {
			billing = text
			return false
		}
		return true
	})
	needle := name + "="
	start := strings.Index(billing, needle)
	if start < 0 {
		return ""
	}
	value := billing[start+len(needle):]
	if end := strings.IndexByte(value, ';'); end >= 0 {
		value = value[:end]
	}
	return strings.TrimSpace(value)
}

func headerValueFold(headers http.Header, name string) string {
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

func claudeCaptureHash(value string) string {
	if value == "" {
		return ""
	}
	return claudeCaptureHashBytes([]byte(value))
}

func claudeCaptureHashBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return fmt.Sprintf("%x", digest[:])
}

func captureClaudeUpstreamRequest(client *http.Client, req *http.Request) (*http.Response, error) {
	captureRoot := strings.TrimSpace(os.Getenv(claudeCaptureDirEnv))
	if captureRoot == "" || client == nil || req == nil {
		return client.Do(req)
	}

	state := claudeCaptureStateFromContext(req.Context())
	if state == nil {
		sequence := atomic.AddUint64(&claudeCaptureSequence, 1)
		requestDir := filepath.Join(captureRoot, fmt.Sprintf("%06d", sequence))
		if err := os.MkdirAll(requestDir, 0o700); err != nil {
			return client.Do(req)
		}
		state = &claudeCaptureState{
			Sequence:   sequence,
			Endpoint:   req.URL.Path,
			StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
			RequestDir: requestDir,
		}
		req = req.WithContext(context.WithValue(req.Context(), claudeCaptureContextKey{}, state))
		writeClaudeCaptureJSON(filepath.Join(requestDir, "capture.json"), state)
	}
	sequence := state.Sequence
	requestDir := state.RequestDir
	if identifiable, ok := client.Transport.(interface{ ClaudeTransportCaptureIdentity() string }); ok {
		state.Association.TransportInstanceSHA256 = claudeCaptureHash(identifiable.ClaudeTransportCaptureIdentity())
	}

	started := time.Now().UTC()
	body, errRead := readAndRestoreClaudeCaptureBody(req)
	if errRead == nil {
		_ = os.WriteFile(filepath.Join(requestDir, "request-body.raw"), body, 0o600)
	}
	captureClaudeStage(req.Context(), "final_roundtrip", body, req.Header)
	association := state.Association
	association.CCPreviousReqSHA256 = claudeCaptureHash(claudeCaptureBillingField(body, "cc_prev_req"))
	association.DiagnosticsPrevSHA256 = claudeCaptureHash(strings.TrimSpace(gjson.GetBytes(body, "diagnostics.previous_message_id").String()))
	if value := headerValueFold(req.Header, "x-client-request-id"); value != "" {
		association.ClientRequestSHA256 = claudeCaptureHash(value)
	}
	if value := headerValueFold(req.Header, "x-claude-code-session-id"); value != "" {
		association.SessionSHA256 = claudeCaptureHash(value)
	}
	requestRecord := claudeCapturedRequest{
		Sequence:         sequence,
		StartedAt:        started.Format(time.RFC3339Nano),
		Method:           req.Method,
		URL:              req.URL.String(),
		Host:             req.Host,
		Proto:            req.Proto,
		ContentLength:    req.ContentLength,
		TransferEncoding: append([]string(nil), req.TransferEncoding...),
		Headers:          redactClaudeCaptureHeaders(req.Header),
		Association:      association,
	}
	writeClaudeCaptureJSON(filepath.Join(requestDir, "request.json"), requestRecord)

	collector := newClaudeHTTPTraceCollector(sequence, association)
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), collector.clientTrace()))
	resp, errDo := client.Do(req)
	collector.write(filepath.Join(requestDir, "httptrace.json"))
	if errDo != nil {
		writeClaudeCaptureJSON(filepath.Join(requestDir, "result.json"), claudeCaptureResult{
			Sequence:      sequence,
			CompletedAt:   time.Now().UTC().Format(time.RFC3339Nano),
			ElapsedMillis: time.Since(started).Milliseconds(),
			ReadResult:    errDo.Error(),
		})
		return nil, errDo
	}

	responseRecord := claudeCapturedResponse{
		Sequence:      sequence,
		ReceivedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Proto:         resp.Proto,
		ContentLength: resp.ContentLength,
		Headers:       redactClaudeCaptureHeaders(resp.Header),
		Close:         resp.Close,
	}
	writeClaudeCaptureJSON(filepath.Join(requestDir, "response.json"), responseRecord)

	responseFile, errFile := os.OpenFile(filepath.Join(requestDir, "response-body.raw"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if errFile == nil && resp.Body != nil {
		resp.Body = &claudeCaptureReadCloser{
			body:       resp.Body,
			file:       responseFile,
			resultPath: filepath.Join(requestDir, "result.json"),
			sequence:   sequence,
			started:    started,
		}
	} else {
		if responseFile != nil {
			_ = responseFile.Close()
		}
		writeClaudeCaptureJSON(filepath.Join(requestDir, "result.json"), claudeCaptureResult{
			Sequence:      sequence,
			CompletedAt:   time.Now().UTC().Format(time.RFC3339Nano),
			ElapsedMillis: time.Since(started).Milliseconds(),
			ReadResult:    "response body capture unavailable",
		})
	}
	return resp, nil
}

func newClaudeHTTPTraceCollector(sequence uint64, association claudeCaptureAssociation) *claudeHTTPTraceCollector {
	return &claudeHTTPTraceCollector{record: claudeHTTPTraceRecord{
		Sequence:    sequence,
		Association: association,
	}}
}

func (collector *claudeHTTPTraceCollector) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GetConn: func(_ string) {
			collector.Lock()
			collector.record.GetConnAt = time.Now().UTC().Format(time.RFC3339Nano)
			collector.Unlock()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			collector.Lock()
			defer collector.Unlock()
			collector.record.GotConnAt = time.Now().UTC().Format(time.RFC3339Nano)
			collector.record.Reused = info.Reused
			collector.record.WasIdle = info.WasIdle
			collector.record.IdleMillis = info.IdleTime.Milliseconds()
			if info.Conn == nil {
				return
			}
			collector.record.LocalAddr = captureNetAddr(info.Conn.LocalAddr())
			collector.record.RemoteAddr = captureNetAddr(info.Conn.RemoteAddr())
			collector.record.ConnectionSHA256 = claudeCaptureHash(fmt.Sprintf("%p\x00%s\x00%s", info.Conn, collector.record.LocalAddr, collector.record.RemoteAddr))
			conn := unwrapClaudeCaptureConn(info.Conn)
			if tlsConn, ok := conn.(*tls.UConn); ok {
				state := tlsConn.ConnectionState()
				collector.record.TLSHandshakeComplete = state.HandshakeComplete
				collector.record.TLSDidResume = state.DidResume
				collector.record.TLSVersion = state.Version
				collector.record.TLSCipherSuite = state.CipherSuite
				collector.record.TLSALPN = state.NegotiatedProtocol
			}
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			collector.Lock()
			collector.record.WroteRequestAt = time.Now().UTC().Format(time.RFC3339Nano)
			if info.Err != nil {
				collector.record.WroteRequestError = info.Err.Error()
			}
			collector.Unlock()
		},
		GotFirstResponseByte: func() {
			collector.Lock()
			collector.record.GotFirstResponseAt = time.Now().UTC().Format(time.RFC3339Nano)
			collector.Unlock()
		},
	}
}

func (collector *claudeHTTPTraceCollector) write(path string) {
	collector.Lock()
	record := collector.record
	collector.Unlock()
	record.WrittenAt = time.Now().UTC().Format(time.RFC3339Nano)
	writeClaudeCaptureJSON(path, record)
}

func captureNetAddr(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	return addr.String()
}

func redactClaudeCaptureHeaders(headers http.Header) http.Header {
	redacted := headers.Clone()
	for key, values := range redacted {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "authorization", "proxy-authorization", "x-api-key", "cookie", "set-cookie":
			for index, value := range values {
				values[index] = "[redacted sha256=" + claudeCaptureHash(value) + "]"
			}
			redacted[key] = values
		}
	}
	return redacted
}

func unwrapClaudeCaptureConn(conn net.Conn) net.Conn {
	for depth := 0; depth < 8 && conn != nil; depth++ {
		unwrapper, ok := conn.(interface{ Unwrap() net.Conn })
		if !ok {
			return conn
		}
		next := unwrapper.Unwrap()
		if next == nil || next == conn {
			return conn
		}
		conn = next
	}
	return conn
}

func readAndRestoreClaudeCaptureBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	return body, nil
}

func writeClaudeCaptureJSON(path string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return
	}
	data = append(data, '\n')
	_ = os.WriteFile(path, data, 0o600)
}

type claudeCaptureReadCloser struct {
	body       io.ReadCloser
	file       *os.File
	resultPath string
	sequence   uint64
	started    time.Time
	bytesRead  int64
	once       sync.Once
}

func (c *claudeCaptureReadCloser) Read(p []byte) (int, error) {
	n, err := c.body.Read(p)
	if n > 0 {
		written, errWrite := c.file.Write(p[:n])
		c.bytesRead += int64(written)
		if errWrite != nil && err == nil {
			err = errWrite
		}
	}
	if err != nil {
		c.finish(err)
	}
	return n, err
}

func (c *claudeCaptureReadCloser) Close() error {
	err := c.body.Close()
	c.finish(err)
	return err
}

func (c *claudeCaptureReadCloser) finish(readErr error) {
	c.once.Do(func() {
		_ = c.file.Sync()
		_ = c.file.Close()
		result := "closed"
		if readErr == io.EOF {
			result = "EOF"
		} else if readErr != nil {
			result = readErr.Error()
		}
		writeClaudeCaptureJSON(c.resultPath, claudeCaptureResult{
			Sequence:      c.sequence,
			CompletedAt:   time.Now().UTC().Format(time.RFC3339Nano),
			ElapsedMillis: time.Since(c.started).Milliseconds(),
			ResponseBytes: c.bytesRead,
			ReadResult:    result,
		})
	})
}
