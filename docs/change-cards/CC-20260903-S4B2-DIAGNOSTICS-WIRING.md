# Change Card: CC-20260903-S4B2-DIAGNOSTICS-WIRING

## Identity

- Signal group: `B-03/B-10` diagnostics request wiring
- Scope: connect the S4-B.1 diagnostics boundary to both executor request paths
- Branch: `feat/s3-resolved-profile`
- Baseline commit: `18e039bdee9cf48c78728f38e7a1269bdecfe2d1`
- Date: 2026-09-03 (America/Los_Angeles)

## Evidence and hypothesis

The existing diagnostics call sites were nested under the context-management
eligibility branch. That made diagnostics depend on an unrelated context
management decision and omitted the managed diagnostics chain for a confirmed
native request when context management was not eligible. S4-B.2 tests the
bounded hypothesis that diagnostics must use the independent S4-B.1 request
boundary in both `Execute` and `ExecuteStream`.

The observation boundary remains the local CPA request boundary. Anthropic
receiver-side raw H1 is not claimed. A/B host, egress, account cohort,
workspace, model entitlement and timing remain intentional confounders.

## Allowed change

`ClaudeExecutor.Execute` and `ClaudeExecutor.ExecuteStream` now call
`beginClaudeDiagnostics(body, auth, claudeSessionID, baseURL,
softwareProfile, fp.InjectDiagnostics)` after context-management handling and
before payload configuration/CCH finalization. The old nested call is removed.

The change preserves:

- S4-B.1 eligibility, confirmed-native requirement and caller-owned diagnostics;
- diagnostics tracker state, generation and success-commit logic;
- prev-request state and CCH ordering;
- TLS, HTTP transport/connection cache, OAuth, proxy, model, Tools, prompts,
  headers and public configuration schema.

Tests update the confirmed-native fixture and add ExecuteStream continuity;
configured-but-unconfirmed profiles remain explicitly non-injecting.

## Verification design

Baseline: detached worktree at `18e039bd` with the S4-B.1 helper present but
without the new call-site wiring. Modified: this worktree after the change.

Focused tests exercise diagnostics injection, eligibility, caller ownership,
non-stream continuity and streaming continuity. The complete executor package
is run in both trees, and the helper tracker regression is repeated ten times.

Expected falsifier: a confirmed-native request fails to receive diagnostics or
the previous completed message ID on its next request; a caller-owned or
unconfirmed request is modified; or unrelated executor tests regress.

## Implementation record

- Source files: `internal/runtime/executor/claude_executor_execute.go`,
  `internal/runtime/executor/claude_executor_stream.go`.
- Test files: `internal/runtime/executor/claude_executor_diagnostics_test.go`,
  `internal/runtime/executor/claude_fingerprint_policy_test.go`.
- No public configuration field or default was added.
- No real OAuth request or secret material was used.
- Commit: resolve with `git rev-parse HEAD` after atomic commit.

## Artifact set

Canonical evidence: `artifacts/CPA-CLAUDECODE-S4B2-DIAGNOSTICS-WIRING-20260903`.
The set contains `MODIFIED_FILE.md`, `DIFF_FILE.md`, `VERIFICATION.txt`, an
executable `ROLLBACK.sh`, literal test logs, source patch and SHA-256 manifest.

## Rollback and release gate

`ROLLBACK.sh` is executed against an independent worktree and must restore the
baseline tree exactly with a clean status. S4-B.2 is an intermediate wiring
step; it does not prove strict A/B equivalence, receiver-side observability or
production release eligibility. CI status remains pending until a branch CI
run is available.

