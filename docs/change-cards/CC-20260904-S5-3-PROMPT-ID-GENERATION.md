# Change Card: CC-20260904-S5-3-PROMPT-ID-GENERATION

## Identity

- Signal ID: `B-12 cc_prompt_id`
- Scope: CPA-owned synthetic prompt-ID generation and lifecycle binding for the
  first-party Claude Messages path
- Branch: `feat/s3-resolved-profile`
- Baseline commit: `db04147e44aaedd8d6e75b989cf75022aae87dec`
- Card owner: CPA compatibility project
- Date: 2026-09-04 (America/Los_Angeles)

## Current Evidence

- Current classification from `STRICT-MATRIX.json`: `CONFIRMED_DIFFERENCE`.
- Official Claude Code 2.1.241 source evidence: a fresh UUID is installed when
  the current input does not contain a `tool_result`; a tool-result continuation
  keeps the active prompt ID; billing accepts only UUID-shaped IDs on the
  first-party path.
- CPA B native SDK evidence: `cc_prompt_id` is absent at the incoming boundary
  and CPA has no lifecycle generator.
- Observation boundary: CPA executor body immediately before the upstream
  RoundTripper, after all body mutations and before CCH finalization.
- Evidence paths: `artifacts/CPA-CLAUDECODE-DOCS-20260829/00-FIRST-READ.md`,
  `artifacts/CPA-CLAUDECODE-STRICT-ONE-TO-ONE-REANALYSIS-20260827/STRICT-MATRIX.json`,
  local Claude Code 2.1.241 binary (SHA-256
  `c49a05922a787c33478067a5164002932235f6611948523b55ae1fbdb303ac1f`),
  `docs/change-cards/CC-20260903-S5-2.3-PROMPT-ID-PRESERVATION.md`.
- Version / host / egress / account-cohort / workload: CPA 7.2.140 on the
  compatibility branch; unit/CI fixtures have no live OAuth or network account.
- Known confounders: provider receiver-side behavior and stock CLI custom-base
  URL routing remain outside this card; host and egress are not normalized.

## Hypothesis And Scope

- Hypothesis: a generated UUID remains stable for a retry and tool-result
  continuation of one prompt, changes at a detectable new prompt boundary,
  and cannot cross credential, session, or agent scopes.
- Allowed files:
  - `internal/runtime/executor/helps/claude_prompt_state.go`
  - `internal/runtime/executor/helps/claude_prompt_state_test.go`
  - `internal/runtime/executor/claude_executor_execute.go`
  - `internal/runtime/executor/claude_executor_stream.go`
  - this card and the S5-3 evidence artifacts
- Allowed behavior: generate/retain a UUID only for the first-party Messages
  path when the resolved native/profile policy is eligible; inject it into the
  existing first billing block or the fallback billing text before CCH signing.
- Prohibited behavior: caller-owned values may not be rewritten; no session ID
  substitution; no generation for count_tokens, title/helper, custom gateways,
  missing/ambiguous session scope, or non-first-party policy; no changes to
  CCH, diagnostics, `cc_prev_req`, model, Tools, system content, betas, IDs,
  TLS, transport cache, OAuth, proxy or retry policy.
- Why this is the owning scope: the existing parser/preservation helper owns
  the billing field and both executors share the same late body/CCH boundary.

## Verification Design

- A fixture: official 2.1.241 lifecycle rules extracted from the saved binary.
- B fixture: synthetic RoundTripper tests for Execute and ExecuteStream with a
  stable credential/session scope and controlled message histories.
- Controls: caller-owned UUID is preserved; tool-result without prior state
  remains absent; title/helper and custom-base requests remain absent; a second
  session and credential receive different IDs.
- Falsifying observation: ID changes on retry/tool continuation, is reused by a
  different scope, is generated without a non-tool prompt boundary, appears in
  count_tokens/helper/custom gateway traffic, or CCH verification fails.
- Expected true result: generated ID is one-per-prompt, stable within the
  prompt graph, isolated by scope, present before final CCH signing, and parity
  holds for Execute/ExecuteStream.
- Expected false result: focused CI fails and this card does not authorize a
  release or broader lifecycle changes.
- GitHub Actions workflow: `.github/workflows/claude-prompt-state-test.yml`
- Expected artifacts: `artifacts/CPA-CLAUDECODE-S5-3-PROMPT-ID-GENERATION-20260904/`

## Implementation Record

- Commit: `3c2faad43dce84fc4e75212886c76f760c144cae`.
- Official binary confirmation: Claude Code `2.1.241` contains a UUID regex for
  `cc_prompt_id`; the captured wire order is `cc_entrypoint`, `cch`,
  `cc_prev_req`, then `cc_prompt_id`.
- Production implementation:
  - `helps.BeginClaudePromptID` hashes credential identity plus Claude session/
    agent scope and stores only the digest as the map key.
  - A non-tool prompt gets one `crypto/rand` UUID; an identical retry and a
    `tool_result` continuation reuse it. A new message graph, expired entry,
    different session, or different credential gets a different UUID.
  - `InsertClaudePromptIDBilling` preserves caller-owned values and inserts a
    generated value after existing billing fields. CCH is finalized afterward.
  - `Execute` and `ExecuteStream` share the same resolver and fallback billing
    path. Helper/title/count_tokens and non-Anthropic paths remain ineligible.
- Regression additions cover strict UUID syntax, ambiguous billing blocks,
  expiration, concurrent calls, caller preservation, failed-request retry,
  tool continuation, session/credential isolation, and CCH idempotence.
- Static checks: `gofmt` and `git diff --check` completed with exit status 0.
- Local baseline focused executor test exited 0 (`ok .../executor 3.969s`).
- Local modified focused executor/helpers tests exited 0 (`ok .../executor
  3.908s`; `ok .../executor/helps 3.380s`).
- GitHub focused workflow `33863098458` and build workflow `33863098452` both
  concluded `success`.
- Default behavior preserved: generation is restricted to the existing native
  first-party/profile policy; caller-owned and non-eligible paths are unchanged.
- Experimental flag/config: none; this is a lifecycle implementation behind
  existing eligibility gates, not a global client impersonation switch.
- Secrets scan result: pending final artifact scan; no raw OAuth, email, proxy
  credential or GitHub token is permitted.

## Rollback

- Rollback command: `bash artifacts/CPA-CLAUDECODE-S5-3-PROMPT-ID-GENERATION-20260904/ROLLBACK.sh <independent-copy-root>`
- Separate-copy rollback test: `ROLLBACK_OK` on an independent baseline
  worktree; restored commit `db04147e44aaedd8d6e75b989cf75022aae87dec`.
- Restored hash/status: `git diff --exit-code` exit 0, empty `git status
  --short`, and restored baseline focused test exit 0. Windows
  `core.autocrlf=true` means the baseline worktree byte hash differs from the
  Git blob hash; source-hashes.txt records the blob values and Git diff is the
  restore authority.

## Review Decision

- CI run URL / run ID: focused
  `https://github.com/b0bo000/CLIProxyAPI/actions/runs/33863098458`; build
  `https://github.com/b0bo000/CLIProxyAPI/actions/runs/33863098452`.
- Evidence classification after change: `B-12=CONFIRMED_DIFFERENCE` remains;
  the implementation closes CPA-side generation behavior but does not prove
  provider-side strict equivalence. Release remains `NOT STRICTLY EQUIVALENT`.
- `FIRST-READ` updated: section 75 records the implementation and verification.
- The `agents-md-guard` run `33863098457` failed on the pre-existing cumulative
  AGENTS.md guard condition; it is not a source-test or build failure.
- `STRICT-MATRIX` updated: no classification change expected.
- Release eligible: `no`.
- Reviewer: user review required.
