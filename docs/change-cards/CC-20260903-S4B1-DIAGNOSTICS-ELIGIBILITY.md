# Change Card: CC-20260903-S4B1-DIAGNOSTICS-ELIGIBILITY

## Identity

- Signal ID: `B-03`, `B-10` (bounded diagnostics shape/continuity eligibility group)
- Scope: Define the request-boundary eligibility and caller-ownership rules for managed Claude diagnostics state; do not wire those rules into `Execute` or `ExecuteStream` in this step.
- Branch: `feat/s3-resolved-profile`
- Baseline commit: `8919a51575ce3ac34c77c7ea8a1784317299f62b`
- Card owner: CPA Claude Code compatibility workstream
- Date: 2026-09-03

## Current Evidence

- Current classification from `STRICT-MATRIX.json`: `CONFIRMED_DIFFERENCE`
- Observation boundary: Official A semantic request capture and CPA B incoming/final request stages; Anthropic receiver-side raw H1 remains unobservable from the client.
- Evidence paths: `artifacts/CPA-CLAUDECODE-DOCS-20260829/00-FIRST-READ.md`, `artifacts/CPA-CLAUDECODE-DOCS-20260829/02-RECONCILIATION.json`, `artifacts/CPA-CLAUDECODE-DOCS-20260829/03-AUDIT-PLAN-63-SIGNALS.md`.
- Version / host / egress / account-cohort / workload: A uses Claude Code 2.1.241 on authorized host `12.17.211.81` direct to Anthropic; B uses local Claude Code 2.1.241 through isolated CPA. Host, egress, account/cohort, workspace and timing differ.
- Known confounders: A/B host, egress, account/cohort, workspace, model entitlement and request timing are intentionally not normalized. HTTP 401, quota 429 and credential lifetime are excluded from CPA attribution.

## Hypothesis And Scope

- Hypothesis: Managed diagnostics continuity may begin only for an Anthropic Messages request with confirmed native Claude Code signals, an enabled diagnostics policy and a non-helper profile. A configured-but-unconfirmed profile or caller-owned diagnostics value must not create CPA state.
- Allowed files: `internal/runtime/executor/claude_executor_diagnostics.go`, `internal/runtime/executor/claude_executor_diagnostics_test.go`, and this card.
- Allowed fields/behavior to change: Add a side-effect-free eligibility predicate and a request-boundary begin helper; preserve caller-owned diagnostics without allocating managed state.
- Prohibited files: Execute/Stream production integration files, TLS/transport/cache packages, auth persistence, translators and configuration schema.
- Prohibited fields/behavior: model, Tools, system prompt, workspace paths, beta selection, CCH, `cc_prev_req`, `cc_prompt_id`, session/request/device IDs, OAuth, proxy behavior, TLS and connection ownership.
- Why this is the smallest owning scope: It separates the eligibility decision from the existing tracker and from the later Execute/Stream call-site change, allowing policy and ownership rules to be falsified independently.

## Verification Design

- Same-scenario A fixture: Confirmed native, non-helper Claude profile targeting `https://api.anthropic.com`, diagnostics policy enabled and no caller diagnostics field.
- Same-scenario B fixture: The same body and upstream with a configured-but-unconfirmed CLI profile.
- Control: Existing diagnostics field-order and continuity tests must remain unchanged and pass.
- Falsifying observation: Any configured-but-unconfirmed, custom-upstream, helper, disabled-policy or caller-owned request allocates diagnostics state or changes body bytes.
- Expected result if hypothesis is true: Only the confirmed-native Anthropic fixture is eligible; all negative fixtures return the original body and zero request state.
- Expected result if hypothesis is false: At least one negative fixture mutates the body or creates a continuity generation.
- GitHub Actions workflow: Repository Go test/build workflows triggered by pushing `feat/s3-resolved-profile`; workflow run URL/ID is recorded after completion.
- Expected artifact paths: `artifacts/CPA-CLAUDECODE-S4B1-DIAGNOSTICS-ELIGIBILITY-20260903`.

## Implementation Record

- Commit(s): This atomic S4-B.1 commit; resolve its immutable ID with `git rev-parse HEAD`.
- Files changed: This card plus `claude_executor_diagnostics.go` and `claude_executor_diagnostics_test.go`.
- Default behavior preserved: Yes. This step introduces no Execute/ExecuteStream call-site change, so production request behavior remains unchanged.
- Experimental flag/config (if any): Uses the existing resolved `InjectDiagnostics` policy input; no public configuration field is added.
- Secrets scan result: Required before closure; raw tokens, email addresses, SSH passwords and proxy credentials must have zero hits in the artifact set.

## Rollback

- Rollback command: Run the artifact `ROLLBACK.sh` against an independent worktree created at this commit.
- Separate-copy rollback test: Required before closure.
- Restored hash/status: Must restore `8919a51575ce3ac34c77c7ea8a1784317299f62b` with a clean tracked worktree.

## Review Decision

- CI run URL / run ID: Pending branch push.
- Evidence classification after change: `CONFIRMED_DIFFERENCE`; B.1 defines eligibility only and does not activate the wire behavior.
- `FIRST-READ` updated: Required before closure.
- `STRICT-MATRIX` updated: No classification change in B.1.
- Release eligible: `no`
- Reviewer: Pending
