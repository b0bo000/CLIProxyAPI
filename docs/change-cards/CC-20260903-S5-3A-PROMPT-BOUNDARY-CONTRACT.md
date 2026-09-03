# Change Card: CC-20260903-S5-3A-PROMPT-BOUNDARY-CONTRACT

## Identity

- Signal: `B-12 cc_prompt_id`
- Secondary graph signals: `G-03 same_process_multi_prompt`, `G-04 retry/recovery boundary`
- Scope: typed caller-adapter prompt-boundary contract only
- Branch: `feat/s3-resolved-profile`
- Baseline commit: `704f980a3eec8dc9bdf136c4ba376f8083be904f`
- Date: 2026-09-03 (America/Los_Angeles)

## Current Evidence

- Classification: `B-12=CONFIRMED_DIFFERENCE`, `G-03=CAPTURED_B_PARTIAL`,
  `G-04=CAPTURED_B_BOUNDARY_LIMITED`.
- Official A has `cc_prompt_id` on 162/162 strict successful SDK requests.
  One tool loop retains one value; three user prompts in one process have three
  distinct values; a parent/subagent loop retains one value.
- Retained B has 60 sanitized native upstream records and 240 gap body-stage
  files with no native prompt ID.
- The current request API exposes session/caller metadata but no explicit
  prompt transaction boundary. Session, request, message, PID, timing and body
  hashes are not valid substitutes.
- Evidence paths:
  `artifacts/CPA-CLAUDECODE-S5-2-PROMPT-BOUNDARY-AUDIT-20260903/` and
  `docs/change-cards/CC-20260903-S5-2-PROMPT-BOUNDARY.md`.
- A/B host, egress, credential cohort and workload remain intentionally
  different confounders. Receiver-side Anthropic raw H1 remains unobserved.

## Hypothesis And Scope

An adapter that actually owns a user-prompt lifecycle can pass an internal,
typed hint containing a boundary kind and stable transaction key. Because the
hint is carried in Go context rather than an HTTP header or request body, the
executor cannot accidentally treat arbitrary wire input as trusted process
identity. A repeated `new` hint with the same transaction key can later be made
idempotent, and `continue` can name the same transaction during tool loops.

Allowed files:

- `sdk/cliproxy/executor/prompt_boundary.go`
- `sdk/cliproxy/executor/prompt_boundary_test.go`
- `internal/runtime/executor/helps/claude_prompt_state.go`
- `internal/runtime/executor/helps/claude_prompt_state_test.go`
- `internal/runtime/executor/claude_executor_execute.go`
- `internal/runtime/executor/claude_executor_stream.go`
- `internal/runtime/executor/claude_executor_prompt_state_test.go`
- this card and its evidence artifact

Allowed behavior:

- represent `absent`, `new`, `continue` and `ambiguous` as an internal typed
  contract;
- validate kind/transaction combinations identically in Execute and
  ExecuteStream;
- reject malformed explicit hints as request-scoped HTTP 400 errors.

Prohibited behavior:

- no prompt-ID allocation, storage, insertion or commit;
- no header/body/PID/session/request/message/time/hash inference;
- no change to a request when the context hint is absent or valid;
- no changes to model, Tools, system prompt, body size, beta set, CCH,
  diagnostics, `cc_prev_req`, software/device identity, TLS, transport, OAuth,
  proxy, public configuration or non-Claude providers.

This is the smallest owning scope because the SDK context carries adapter-owned
provenance and the Claude helper owns prompt semantics. A body or header field
would be weaker provenance, while lifecycle state or generation would exceed
the evidence gate.

## Verification Design

- Baseline: exact `704f980a` source tree.
- Modified: context round-trip tests, table-driven Claude contract validation,
  and Execute/ExecuteStream malformed-hint parity.
- Control: no hint resolves to `absent`; explicit `ambiguous` remains
  non-generating; valid `new`/`continue` values are returned unchanged.
- Falsifier: a malformed hint reaches upstream, a missing hint changes wire
  behavior, a transaction crosses context boundaries, or any prompt ID appears.
- CI workflow: `.github/workflows/pr-test-build.yml` after branch push/PR.
- Local validation is limited to `gofmt`, static inspection,
  `git diff --check`, source hash checks and artifact validators.

## Rollback

- Source rollback: `git revert --no-edit <S5-3A-commit>`.
- Evidence rollback: the artifact `ROLLBACK.sh` applies the reverse patch to an
  independent copy and must restore the baseline hashes.

## Release Gate

S5-3A does not authorize S5-3B lifecycle state or missing-ID generation. It
does not close B-12 and cannot change the release state from
`NOT STRICTLY EQUIVALENT`.

## Implementation Record

- Commits: `b6493c9d` (contract, resolver and tests), `a8c51445` (executable
  rollback artifact).
- Files changed: the eight allowed source/test/card paths plus this step's
  evidence package and the S5 execution log.
- Default behavior: preserved; an absent hint resolves to `absent` and creates
  no body/header mutation.
- Malformed explicit hint: Execute and ExecuteStream return the same
  request-scoped HTTP 400 before translation or upstream I/O.
- Prompt generation/storage/insertion count: zero.
- Baseline static result:
  `CONTRACT_STATIC_OK MODE=BASELINE CONTEXT_API=0 RESOLVER=0 EXECUTOR_GUARDS=0 PROMPT_GENERATOR=0`.
- Modified static result:
  `CONTRACT_STATIC_OK MODE=MODIFIED CONTEXT_API=1 RESOLVER=1 EXECUTOR_GUARDS=2 PROMPT_GENERATOR=0`.
- Independent-copy rollback: forward apply, modified check, reverse apply,
  baseline check and clean-status check all exited 0; restored HEAD is
  `704f980a3eec8dc9bdf136c4ba376f8083be904f`.
- Local Go test/build: not run under repository validation policy. The
  authorized remote host reported no Go executable.
- GitHub CI: pending; `gh` is unavailable and the configured Git credential
  helper could not authenticate the branch push during this step.
- Secrets scan: OAuth token, e-mail and bearer-token pattern hits are zero.
- Release eligible: no.
