package executor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

const claudePrevRequestProbeID = "req_cpa_prev_request_probe"

type claudePrevRequestState struct {
	key      string
	sequence uint64
}

type claudePrevRequestRequestError struct {
	cause error
}

func (e *claudePrevRequestRequestError) Error() string {
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func (e *claudePrevRequestRequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *claudePrevRequestRequestError) StatusCode() int {
	if e == nil {
		return 0
	}
	return http.StatusBadRequest
}

func (e *claudePrevRequestRequestError) IsRequestScoped() bool {
	return e != nil
}

func newClaudePrevRequestRequestError(err error) error {
	if err == nil {
		return nil
	}
	return &claudePrevRequestRequestError{cause: err}
}

func beginClaudePrevRequestExecute(
	body []byte,
	auth *cliproxyauth.Auth,
	apiKey string,
	sessionScope, baseURL string,
	softwareProfile helps.ResolvedClaudeSoftwareProfile,
	upstreamStream bool,
	injectPrevRequest bool,
) ([]byte, claudePrevRequestState, error) {
	// The policy gate is intentionally separate from diagnostics. A disabled
	// prev-request policy must not allocate or advance continuity state.
	if upstreamStream || !injectPrevRequest {
		return body, claudePrevRequestState{}, nil
	}
	return beginClaudePrevRequest(body, auth, apiKey, sessionScope, baseURL, softwareProfile, injectPrevRequest)
}

func beginClaudePrevRequest(
	body []byte,
	auth *cliproxyauth.Auth,
	apiKey string,
	sessionScope, baseURL string,
	softwareProfile helps.ResolvedClaudeSoftwareProfile,
	injectPrevRequest bool,
) ([]byte, claudePrevRequestState, error) {
	if !claudePrevRequestEligible(injectPrevRequest, baseURL, softwareProfile) {
		return body, claudePrevRequestState{}, nil
	}
	credentialIdentity := claudePrevRequestCredentialIdentity(auth, apiKey)
	if credentialIdentity == "" || strings.TrimSpace(sessionScope) == "" {
		return body, claudePrevRequestState{}, nil
	}

	// A probe through the same parser proves that a valid billing block exists
	// and is missing cc_prev_req. An unchanged result means either no billing
	// block or a caller-owned value, neither of which CPA may take over.
	probe, errProbe := insertClaudePrevRequestBilling(body, claudePrevRequestProbeID)
	if errProbe != nil {
		return nil, claudePrevRequestState{}, newClaudePrevRequestRequestError(errProbe)
	}
	if bytes.Equal(probe, body) {
		return body, claudePrevRequestState{}, nil
	}

	key, sequence, previousRequestID := helps.BeginClaudePrevRequest(credentialIdentity, sessionScope)
	state := claudePrevRequestState{key: key, sequence: sequence}
	if key == "" || previousRequestID == "" {
		return body, state, nil
	}
	updated, errInsert := insertClaudePrevRequestBilling(body, previousRequestID)
	if errInsert != nil {
		return nil, claudePrevRequestState{}, newClaudePrevRequestRequestError(errInsert)
	}
	return updated, state, nil
}

// claudePrevRequestEligible is deliberately independent from diagnostics
// eligibility. Confirmed native requests and the configured real Claude Code
// CLI wire profile may carry the continuity field; helper/title requests,
// unknown callers, count_tokens, and non-Anthropic gateways cannot create or
// advance this state.
func claudePrevRequestEligible(injectPrevRequest bool, baseURL string, softwareProfile helps.ResolvedClaudeSoftwareProfile) bool {
	if !injectPrevRequest || !isAnthropicUpstreamBase(baseURL) || softwareProfile.IsHelperProfile() {
		return false
	}
	return softwareProfile.Confirmed || softwareProfile.Provenance == helps.ClaudeSoftwareProfileConfiguredCLI
}

func claudePrevRequestCredentialIdentity(auth *cliproxyauth.Auth, apiKey string) string {
	if auth == nil {
		return ""
	}
	recordIdentity := strings.TrimSpace(helps.ClaudeCLIAuthIdentitySeed(auth))
	if recordIdentity == "" {
		return ""
	}
	apiKey = strings.TrimSpace(apiKey)

	// AuthKind is authoritative when present. An API-key record can retain
	// stale OAuth account metadata after a hot swap, so account_uuid must not
	// be consulted before the credential kind is known. API keys use the
	// current key digest; OAuth uses the account UUID and survives token
	// rotation.
	switch auth.AuthKind() {
	case cliproxyauth.AuthKindAPIKey:
		if apiKey == "" {
			return ""
		}
		digest := sha256.Sum256([]byte(apiKey))
		return fmt.Sprintf("%s\x00api-key:%x", recordIdentity, digest)
	case cliproxyauth.AuthKindOAuth:
		accountUUID := strings.TrimSpace(helps.ClaudeCredentialAccountUUID(auth))
		if accountUUID == "" {
			return ""
		}
		return recordIdentity + "\x00account:" + accountUUID
	}

	// Legacy records may not carry auth_kind. The OAuth token shape is the only
	// available discriminator in that case; unknown credential shapes do not
	// create a chain.
	if isClaudeOAuthToken(apiKey) {
		accountUUID := strings.TrimSpace(helps.ClaudeCredentialAccountUUID(auth))
		if accountUUID == "" {
			return ""
		}
		return recordIdentity + "\x00account:" + accountUUID
	}
	return ""
}

func commitClaudePrevRequestExecute(ctx context.Context, state claudePrevRequestState, headers http.Header, upstreamBody, translatedBody []byte) {
	if state.key == "" || state.sequence == 0 || len(translatedBody) == 0 || !gjson.ValidBytes(upstreamBody) || !gjson.ValidBytes(translatedBody) {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	root := gjson.ParseBytes(upstreamBody)
	messageType := root.Get("type")
	messageID := root.Get("id")
	if !root.IsObject() || messageType.Type != gjson.String || messageType.String() != "message" || messageID.Type != gjson.String || strings.TrimSpace(messageID.String()) == "" {
		return
	}
	commitClaudePrevRequestState(ctx, state, headers)
}

func commitClaudePrevRequestState(ctx context.Context, state claudePrevRequestState, headers http.Header) {
	if state.key == "" || state.sequence == 0 {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	requestID, errRequestID := claudePrevRequestIDHeader(headers)
	if errRequestID != nil {
		return
	}
	helps.CommitClaudePrevRequest(state.key, state.sequence, requestID)
}

func claudePrevRequestIDHeader(headers http.Header) (string, error) {
	if headers == nil {
		return "", fmt.Errorf("Claude request-id response header is missing")
	}
	var values []string
	for key, headerValues := range headers {
		if !strings.EqualFold(key, "request-id") {
			continue
		}
		values = append(values, headerValues...)
	}
	if len(values) != 1 {
		return "", fmt.Errorf("Claude request-id response header has %d values, want exactly one", len(values))
	}
	if errID := validateClaudePrevRequestID(values[0]); errID != nil {
		return "", errID
	}
	return values[0], nil
}

func (e *ClaudeExecutor) Execute(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (resp cliproxyexecutor.Response, err error) {
	if opts.Alt == "responses/compact" {
		return resp, statusErr{code: http.StatusNotImplemented, msg: "/responses/compact not supported"}
	}
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	upstreamModel := e.upstreamModel(baseModel)

	apiKey, baseURL := claudeCreds(auth)
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	url := fmt.Sprintf("%s/v1/messages?beta=true", baseURL)
	fp := resolveClaudeFingerprintPolicy(e.cfg, auth, apiKey)
	// Real Claude OAuth always signs CCH. An opted-in API key signs only where
	// native does, so a third-party gateway keeps a cache-stable billing header.
	// Default API-key and delegated-provider requests preserve the caller body.
	cchSigning := claudeCCHSigningEnabled(apiKey, claudeCCHUpstreamAnthropic, fp.ProfileClaudeCodeCLI, url)

	reporter := helps.NewExecutorUsageReporter(ctx, e, baseModel, auth)
	defer reporter.TrackFailure(ctx, &err)
	from := opts.SourceFormat
	responseFormat := cliproxyexecutor.ResponseFormatOrSource(opts)
	to := sdktranslator.FromString("claude")
	var replayScope claudeThinkingReplayScope
	if claudeThinkingReplayEnabled(auth, req, opts) {
		req, replayScope = prepareClaudeThinkingReplayRequest(ctx, auth, req, opts)
	}
	defer func() {
		if err != nil && replayScope.replayApplied && shouldClearKimiThinkingReplayAfterError(err) {
			clearClaudeThinkingReplayContent(ctx, replayScope)
		}
	}()
	// Use an upstream stream whenever the downstream response needs translation
	// from Claude events. Native Claude responses use the JSON response path.
	upstreamStream := responseFormat != to
	originalPayloadSource := req.Payload
	if len(opts.OriginalRequest) > 0 {
		originalPayloadSource = opts.OriginalRequest
	}
	originalPayload := originalPayloadSource
	incomingHeaders := resolveIncomingClaudeHeaders(ctx, opts.Headers)
	configuredCLI := fp.ProfileClaudeCodeCLI
	softwareProfile, errSoftwareProfile := helps.ResolveClaudeSoftwareProfile(ctx, auth, apiKey, incomingHeaders, originalPayload, false, e.cfg, configuredCLI)
	if errSoftwareProfile != nil {
		return resp, errSoftwareProfile
	}
	confirmedClaudeCode := softwareProfile.Confirmed
	claudeSessionID := ""
	claudePrevRequestScope := ""
	if confirmedClaudeCode || fp.ProfileClaudeCodeCLI {
		claudeSessionID = helps.ClaudeAgentSessionUUIDForRequest(incomingHeaders, originalPayload, req.Payload, confirmedClaudeCode, opts.Metadata, req.Metadata)
		if scope, ok := helps.ClaudeCodeExecutionScope(ctx, originalPayload, incomingHeaders); ok {
			claudePrevRequestScope = scope
		}
	}
	originalTranslated := helps.TranslateRequestWithAPIKeyModelCompatibility(ctx, opts.Headers, e.cfg, from, to, baseModel, originalPayload, upstreamStream, helps.APIKeyModelIsCompat(req))
	body := helps.TranslateRequestWithAPIKeyModelCompatibility(ctx, opts.Headers, e.cfg, from, to, baseModel, req.Payload, upstreamStream, helps.APIKeyModelIsCompat(req))
	body = helps.SetStringIfDifferent(body, "model", upstreamModel)

	body, err = helps.ApplyRequestThinking(body, req, opts, from.String(), to.String(), e.Identifier())
	if err != nil {
		return resp, err
	}
	if rebuildMidSystemMessageEnabled(e.cfg, auth) {
		body = rebuildMidSystemMessagesToTopLevel(body)
	}

	// Apply cloaking (system prompt injection, fake user ID, sensitive word obfuscation)
	// based on client type and configuration.
	bodyBeforeCloaking := body
	var cloaked bool
	body, cloaked, err = applyCloakingWithResolvedProfile(
		ctx,
		e.cfg,
		auth,
		body,
		apiKey,
		softwareProfile,
		cchSigning,
	)
	if err != nil {
		return resp, err
	}
	systemPlacementState := captureClaudeCodeSystemPlacement(bodyBeforeCloaking, body, cloaked)
	// Only the Messages endpoint on Anthropic itself was captured; count_tokens
	// keeps its own shape and other gateways never see this field.
	diagnosticsState := claudeDiagnosticsRequestState{}
	contextManagementState := claudeCodeContextManagementState{
		eligible:    cloaked && isAnthropicUpstreamBase(baseURL),
		callerOwned: gjson.GetBytes(body, "context_management").Exists(),
	}
	if contextManagementState.eligible {
		body, contextManagementState.automaticallyInjected = injectClaudeCodeContextManagement(body)
	}
	// Diagnostics has an independent eligibility boundary. Native Claude Code
	// requests are not cloaked, so tying this call to context-management
	// eligibility would silently omit the managed diagnostics chain from the
	// confirmed-native wire path.
	body, diagnosticsState = beginClaudeDiagnostics(
		body, auth, claudeSessionID, baseURL, softwareProfile, fp.InjectDiagnostics,
	)

	requestedModel := helps.PayloadRequestedModel(opts, req.Model)
	requestPath := helps.PayloadRequestPath(opts)
	body, contextManagementState.payloadRuleTouched = helps.ApplyPayloadConfigWithRequestTracked(e.cfg, baseModel, to.String(), from.String(), "", body, originalTranslated, requestedModel, requestPath, opts.Headers, "context_management")
	body = reconcileClaudeCodeSystemPlacementAfterPayload(body, systemPlacementState)
	body = ensureModelMaxTokens(body, baseModel)

	// Disable thinking if tool_choice forces tool use (Anthropic API constraint)
	body = disableThinkingIfToolChoiceForced(body)
	body = reconcileClaudeCodeContextManagement(body, contextManagementState)
	body = normalizeClaudeSamplingForUpstream(body, confirmedClaudeCode)

	// Default cache_control for translated entrypoints (Responses/Chat/Gemini) and other
	// non-native callers. Confirmed native Claude Code owns its marker placement and must
	// not be rewritten. Cloaked requests always run section-independent ensure so cloaking's
	// first-user marker cannot suppress system/latest-user breakpoints.
	// cloaked and confirmedClaudeCode are mutually exclusive: resolveClaudeWirePolicy
	// forces Cloak off for a confirmed native client.
	cpaOwnsCacheControl := shouldEnsureCacheControl(body, cloaked, confirmedClaudeCode)
	if cpaOwnsCacheControl {
		body = ensureCacheControl(body)
	}

	// Enforce Anthropic's cache_control block limit (max 4 breakpoints per request).
	// Cloaking and ensureCacheControl may push the total over 4 when the client
	// already sends multiple cache_control blocks.
	body = enforceCacheControlLimit(body, 4)

	// Native selects the 1h cache pool only for OAuth credentials and pairs it with
	// extended-cache-ttl-2025-04-11, which claudeCodeCLIBetas emits on exactly the
	// same credential condition. Upgrading after placement is settled mirrors the
	// native ttl helper.
	//
	// This runs only while CPA owns placement, and it then owns the ttl of every
	// breakpoint it can reach: a marker carrying no ttl is the wire default, not an
	// opt-in to 5m, so a cloaked caller's bare {"type":"ephemeral"} is upgraded too.
	// Only a ttl the caller wrote out explicitly survives, because
	// upgradeClaudeCacheControlTTL skips any block that already has one.
	// claude-code-cli fingerprint profiles emit extended-cache-ttl and must use the same 1h pool.
	if cpaOwnsCacheControl && fp.ProfileClaudeCodeCLI {
		body = upgradeClaudeCacheControlTTL(body, claudeCacheControlTTL1h)
	}

	// Normalize TTL values to prevent ordering violations under prompt-caching-scope-2026-01-05.
	// A 1h-TTL block must not appear after a 5m-TTL block in evaluation order (tools→system→messages).
	body = normalizeCacheControlTTL(body)
	// Payload rules and other request processing may rewrite stream. Keep the
	// upstream body, transport headers, and response parser on one authority.
	// Native non-stream Haiku helper requests omit stream rather than sending
	// false, so preserve that measured wire shape when the transport agrees.
	streamField := gjson.GetBytes(body, "stream")
	if !softwareProfile.IsHelperProfile() || streamField.Exists() || upstreamStream {
		body = helps.SetBoolIfDifferent(body, "stream", upstreamStream)
	}

	// Extract betas from body and convert to header
	var extraBetas []string
	extraBetas, body = extractAndRemoveBetas(body)
	bodyForTranslation := body
	bodyForUpstream := body
	var oauthToolNamesReverseMap map[string]string
	if fp.MCPAlias && cloaked {
		mcpAliases := resolveClaudeMCPAliasOptions(ctx)
		bodyForUpstream, oauthToolNamesReverseMap = prepareClaudeOAuthToolNamesForUpstream(bodyForUpstream, mcpAliases)
	}
	bodyForUpstream = sanitizeClaudeMessagesForClaudeUpstreamWithDebug(ctx, bodyForUpstream, baseModel, helps.APIKeyModelIsCompat(req))
	if fp.ApplyCLIIdentity {
		bodyForUpstream, err = applyClaudeCLIIdentity(bodyForUpstream, auth, apiKey, url, claudeSessionID, fp.SynthesizeIdentity)
		if err != nil {
			return resp, err
		}
	}
	var prevRequestState claudePrevRequestState
	bodyForUpstream, prevRequestState, err = beginClaudePrevRequestExecute(
		bodyForUpstream,
		auth,
		apiKey,
		claudePrevRequestScope,
		baseURL,
		softwareProfile,
		upstreamStream,
		claudePrevRequestPolicyEnabled(fp, confirmedClaudeCode),
	)
	if err != nil {
		return resp, err
	}
	cchBilling := ""
	if cchSigning {
		if !softwareProfile.IsHelperProfile() || claudeBodyNeedsBillingFallback(bodyForUpstream) {
			cchBilling = claudeCCHFallbackBillingHeaderWithProfile(ctx, e.cfg, bodyForUpstream, softwareProfile)
		}
		bodyForUpstream, err = finalizeAnthropicMessagesBodyCCH(bodyForUpstream, cchBilling)
		if err != nil {
			return resp, fmt.Errorf("finalize Claude CCH: %w", err)
		}
	}
	bodyForUpstream = stripDefaultKimiClaudeCodeAttribution(auth, url, fp.ProfileClaudeCodeCLI, bodyForUpstream)
	if errIdentity := helps.ValidateClaudeBillingSoftwareIdentity(bodyForUpstream, softwareProfile, e.cfg); errIdentity != nil {
		return resp, errIdentity
	}
	// Runs on the finished body: payload rules can rewrite model and messages
	// long after translation, so an earlier check would not describe the request
	// that is about to be sent.
	if errMidSystem := validateClaudeMidSystemMessageModel(bodyForUpstream, confirmedClaudeCode, isAnthropicUpstreamBase(baseURL)); errMidSystem != nil {
		return resp, errMidSystem
	}
	reporter.SetTranslatedReasoningEffort(bodyForUpstream, to.String())
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyForUpstream))
	if err != nil {
		return resp, err
	}
	if errHeaders := applyClaudeHeadersWithResolvedProfile(
		httpReq,
		auth,
		apiKey,
		upstreamStream,
		extraBetas,
		bodyForUpstream,
		e.cfg,
		incomingHeaders,
		softwareProfile,
		softwareProfile.IsHelperProfile(),
		claudeSessionID,
	); errHeaders != nil {
		return resp, errHeaders
	}
	fastRequest := isAnthropicUpstreamBase(baseURL) && claudeRequestIsFast(httpReq, bodyForUpstream)
	authID, authLabel, authType, authValue := claudeAuthLogIdentity(auth)
	helps.RecordAPIRequest(ctx, e.cfg, helps.UpstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      bodyForUpstream,
		Provider:  e.upstreamRequestLogProvider(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := helps.NewUtlsHTTPClient(ctx, e.cfg, auth, 0)
	httpClient = reporter.TrackHTTPClient(httpClient)
	httpResp, err := doClaudeUpstreamRequest(httpClient, httpReq)
	if err != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, err)
		return resp, wrapClaudeFastRequestError(fastRequest, 0, err)
	}
	helps.RecordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		// Decompress error responses — pass the Content-Encoding value (may be empty)
		// and let decodeResponseBody handle both header-declared and magic-byte-detected
		// compression.  This keeps error-path behaviour consistent with the success path.
		errBody, decErr := decodeResponseBody(httpResp.Body, claudeResponseContentEncoding(httpResp.Header))
		if decErr != nil {
			helps.RecordAPIResponseError(ctx, e.cfg, decErr)
			msg := fmt.Sprintf("failed to decode error response body: %v", decErr)
			helps.LogWithRequestID(ctx).Warn(msg)
			errClassified := classifyClaudeUpstreamError(httpResp.StatusCode, httpResp.Header, []byte(msg))
			if fastRequest {
				return resp, wrapClaudeFastRequestError(fastRequest, httpResp.StatusCode, errClassified)
			}
			return resp, errClassified
		}
		b, readErr := io.ReadAll(errBody)
		if readErr != nil {
			helps.RecordAPIResponseError(ctx, e.cfg, readErr)
			msg := fmt.Sprintf("failed to read error response body: %v", readErr)
			helps.LogWithRequestID(ctx).Warn(msg)
			b = []byte(msg)
		}
		helps.AppendAPIResponseChunk(ctx, e.cfg, b)
		helps.LogWithRequestID(ctx).Debugf("request error, error status: %d, error message: %s", httpResp.StatusCode, helps.SummarizeErrorBody(httpResp.Header.Get("Content-Type"), b))
		if errClose := errBody.Close(); errClose != nil {
			log.Errorf("response body close error: %v", errClose)
		}
		if fastRequest {
			return resp, newClaudeFastDirectResponseError(httpResp, b)
		}
		return resp, classifyClaudeUpstreamError(httpResp.StatusCode, httpResp.Header, b)
	}
	decodedBody, err := decodeResponseBody(httpResp.Body, claudeResponseContentEncoding(httpResp.Header))
	if err != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, err)
		if errClose := httpResp.Body.Close(); errClose != nil {
			log.Errorf("response body close error: %v", errClose)
		}
		return resp, wrapClaudeFastRequestError(fastRequest, httpResp.StatusCode, err)
	}
	defer func() {
		if errClose := decodedBody.Close(); errClose != nil {
			log.Errorf("response body close error: %v", errClose)
		}
	}()
	data, err := io.ReadAll(decodedBody)
	if err != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, err)
		return resp, wrapClaudeFastRequestError(fastRequest, httpResp.StatusCode, err)
	}
	helps.AppendAPIResponseChunk(ctx, e.cfg, data)
	var diagnosticsMessageID string
	if upstreamStream {
		if errValidate := validateClaudeStreamingResponse(data); errValidate != nil {
			helps.RecordAPIResponseError(ctx, e.cfg, errValidate)
			return resp, wrapClaudeFastRequestError(fastRequest, httpResp.StatusCode, errValidate)
		}
		diagnosticsMessageID = claudeMessageIDFromSSE(data)
		lines := bytes.Split(data, []byte("\n"))
		for i, line := range lines {
			if detail, ok := helps.ParseClaudeStreamUsage(line); ok {
				reporter.Publish(ctx, detail)
			}
			restoredLine, errRestore := restoreClaudeOAuthToolNamesFromStreamLine(line, oauthToolNamesReverseMap)
			if errRestore != nil {
				errRestore = fmt.Errorf("restore Claude OAuth tool name from streaming response: %w", errRestore)
				helps.RecordAPIResponseError(ctx, e.cfg, errRestore)
				return resp, wrapClaudeFastRequestError(fastRequest, httpResp.StatusCode, errRestore)
			}
			lines[i] = restoredLine
		}
		data = bytes.Join(lines, []byte("\n"))
	} else {
		diagnosticsMessageID = claudeMessageIDFromResponse(data)
		reporter.Publish(ctx, helps.ParseClaudeUsage(data))
		var errRestore error
		data, errRestore = restoreClaudeOAuthToolNamesFromResponse(data, oauthToolNamesReverseMap)
		if errRestore != nil {
			errRestore = fmt.Errorf("restore Claude OAuth tool name from response: %w", errRestore)
			helps.RecordAPIResponseError(ctx, e.cfg, errRestore)
			return resp, wrapClaudeFastRequestError(fastRequest, httpResp.StatusCode, errRestore)
		}
	}
	data = e.restoreResponseModel(data, req.Model)
	cacheClaudeThinkingReplayResponse(ctx, replayScope, data)
	var param any
	out := sdktranslator.TranslateNonStream(
		ctx,
		to,
		responseFormat,
		req.Model,
		opts.OriginalRequest,
		bodyForTranslation,
		data,
		&param,
	)
	if responseFormat == sdktranslator.FormatOpenAIResponse {
		out = helps.EnsureResponsesUsageDetails(out)
	}
	// Commit diagnostics only after restoration and downstream translation have
	// completed, so upstream 2xx alone cannot advance visible continuity.
	if len(out) > 0 {
		commitClaudeDiagnostics(diagnosticsState, diagnosticsMessageID)
	}
	commitClaudePrevRequestExecute(ctx, prevRequestState, httpResp.Header, data, out)
	resp = cliproxyexecutor.Response{Payload: out, Headers: httpResp.Header.Clone()}
	return resp, nil
}
