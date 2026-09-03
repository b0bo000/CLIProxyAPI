# S4-A.3 Non-Streaming `cc_prev_req` Integration

## Signal And Classification

- Signal ID: `body/cc_prev_req`
- Current classification: `CONFIRMED_DIFFERENCE`
- Official A evidence: 145 of 162 successful SDK requests carry a value linked
  to the preceding successful upstream response `request-id`.
- CPA B evidence: 0 of 78 retained native SDK requests carry the dynamic field.
- Canonical sources:
  - `artifacts/CPA-CLAUDECODE-DOCS-20260829/00-FIRST-READ.md`
  - `artifacts/CPA-ClaudeCode-STRICT-ONE-TO-ONE-REANALYSIS-20260827/STRICT-MATRIX.json`
  - `artifacts/CPA-CLAUDECODE-S4A-CC-PREV-REQ-PLAN-20260902/MODIFIED_FILE.md`

## Atomic Scope

S4-A.3 integrates the independent S4-A.1 state and S4-A.2 billing mutation into
`ClaudeExecutor.Execute` only. It begins continuity before CCH finalization,
injects only the real prior successful upstream `request-id`, and commits the
current response header only after a complete successful non-stream JSON path.

Allowed source files:

- `internal/runtime/executor/claude_executor_execute.go`
- `internal/runtime/executor/claude_executor_prev_request_execute_test.go`

Supporting correctness fixes required by the A.3 commit contract:

- `internal/runtime/executor/claude_executor_request.go`: bound and restrict
  response `request-id` values to printable ASCII before they can enter the
  billing field; the observed official value is 28 bytes and the parser limit
  is 128 bytes.
- `internal/runtime/executor/helps/claude_prev_request.go` and its test: reject
  a direct late commit after the bounded state entry has expired.

The test may use the existing executor test helpers. No production file other
than `claude_executor_execute.go` may change in this step.

## Eligibility

All conditions must hold:

- the resolved caller is confirmed native Claude Code;
- the request targets Anthropic Messages;
- the request is not a detected helper/title profile;
- `Execute` uses the non-stream upstream JSON response path;
- a stable credential identity and Claude session scope are both present;
- an existing, valid billing block is present and does not already contain a
  caller-owned `cc_prev_req`.

Configured-but-unconfirmed callers, helper/title requests, CountTokens, custom
gateways, caller-owned values and missing scopes do not create or advance state.
S4-A.5 will separately decide any broader policy and feature-switch behavior.

## Ordering And Commit Contract

1. Translation, payload rules, sanitization and credential identity finish.
2. The eligible request begins its independent prev-request generation.
3. A prior value, when present, is inserted into the billing text.
4. CCH is finalized over that exact body.
5. The request is sent through the existing transport.
6. A 2xx response is fully decoded and read.
7. The response is valid JSON; tool-name restoration and final translation
   complete.
8. A syntactically valid response `request-id` advances the generation.

Transport errors, cancellation, non-2xx responses, decode/read failures,
invalid JSON, conversion/restoration errors and missing or invalid request IDs
do not commit.

## Tests And Falsification

Synthetic executor tests must prove:

- fresh request omission, then second- and third-request advancement;
- CCH is valid after insertion;
- 4xx/5xx, transport error, read error and invalid JSON do not advance state;
- missing or invalid response `request-id` does not advance state;
- caller-owned `cc_prev_req` is byte/value preserved and does not seed CPA state;
- different credentials and sessions cannot consume each other's prior value;
- helper/title and non-Anthropic requests remain ineligible.

The change is falsified by any early commit, cross-scope value, caller-value
overwrite, CCH mismatch, Stream/diagnostics mutation, or advancement from an
invalid response.

## Confounders And Observation Boundary

The behavior test uses an in-process synthetic RoundTripper and observes the
final CPA request body plus synthetic upstream response headers. It does not
claim Anthropic receiver-side raw H1 visibility. Historical A and B hosts,
egress, credentials, workspaces and cohorts remain intentionally different and
are not normalized by this source test.

## Unchanged Behavior

This step does not change ExecuteStream, diagnostics, `cc_prompt_id`, CCH
algorithm/value generation, TLS/JA3/ALPN/PSK, RoundTripper/cache ownership,
model, tools, system prompt content, beta policy, OAuth refresh, proxy routing,
production configuration, or S2-B owner integration.

## Policy Gate Status

This branch contains the A.3 wiring as an implementation intermediate. It does
not add the independent `InjectPrevRequest` configuration gate; that remains the
separate S4-A.5 step defined by the canonical plan. Consequently this commit
must not be deployed as a release until A.5 supplies and verifies the policy
gate. It does not reuse `InjectDiagnostics` as an implicit substitute.

## Validation And Rollback

- Local workstation: `gofmt`, `git diff --check`, static source checks only.
- Behavioral validation: focused Go tests and server compile on a non-local
  runner because local compilation is disabled by repository instructions.
- GitHub PR: `https://github.com/b0bo000/CLIProxyAPI/pull/1`.
- Evidence target:
  `artifacts/CPA-CLAUDECODE-S4A3-EXECUTE-PREV-REQ-20260902`.
- Rollback source command after the atomic commit:
  `git revert --no-edit <S4-A.3-commit>`.
- An independent-copy rollback must restore the pre-step hashes while the live
  worktree remains changed.

## Pre-Edit Gate

- [x] Canonical FIRST-READ was read at the start of the turn.
- [x] Branch is `feat/s3-resolved-profile`; `main` and baselines are untouched.
- [x] Signal and classification were copied from `STRICT-MATRIX.json`.
- [x] A/B host and egress differences remain recorded as confounders.
- [x] Observation boundary, allowed files and prohibited behavior are explicit.
- [x] Same-scenario controls and falsifying results are defined.
- [x] The validation runner and evidence paths are named.
- [x] Rollback is defined before implementation.

Pre-step source HEAD: `9b34df4c061aa668d6e4df23c4f4147b1df4d824`.
