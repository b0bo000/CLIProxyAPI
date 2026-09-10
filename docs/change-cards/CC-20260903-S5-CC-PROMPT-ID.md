# Change Card: CC-20260903-S5-CC-PROMPT-ID

## Identity

- Signal: `B-12 cc_prompt_id`
- Secondary graph signal: `G-03 same_process_multi_prompt`
- Scope: establish a lifecycle-aware prompt boundary contract and the smallest
  source change needed to read or forward `cc_prompt_id` without inventing it
- Branch: `feat/s3-resolved-profile`
- Baseline commit: `52017a63b044595fc9457a05ea2edd3223e4957a`
- Date: 2026-09-03 (America/Los_Angeles)

## Evidence and current classification

- Official A strict SDK evidence contains `cc_prompt_id` in `162/162` successful
  SDK requests. The same value remains stable during one tool loop; separate
  prompt lifecycles produce separate values.
- CPA B native SDK evidence has no formed native prompt-ID chain in the
  retained incoming/final boundary. The precise statement is that the caller
  shape entering CPA lacks the field and CPA has no lifecycle model to restore
  it; this card does not claim that CPA deleted a caller field.
- The signal remains `CONFIRMED_DIFFERENCE` at the observed client/CPA
  boundary, not proof of provider-side enforcement or account action.

## Hypothesis

`cc_prompt_id` identifies a user prompt transaction, not a session. A prompt
ID must be stable across the request set belonging to one prompt, including
tool-use continuation and an idempotent retry/resume, and must change at a
new prompt boundary. It must never be copied across credentials, sessions,
agents or unrelated processes.

## Atomic implementation boundary

The first source step may do only the following:

1. Define a prompt-state owner keyed by the already resolved credential,
   session and agent scope.
2. Read and validate an existing caller-owned `cc_prompt_id` from the billing
   block when present, preserving its value and placement byte-for-byte except
   for the separately owned `cc_prev_req` insertion already covered by S4.
3. Expose a lifecycle operation that can carry a prompt ID across a confirmed
   tool-loop/retry boundary once the caller or a separately verified boundary
   signal identifies that continuation.
4. Add focused tests for parse/preserve, same-loop stability, new-prompt
   isolation, failed-response non-commit, and Execute/ExecuteStream parity.

Generation for a missing ID is not authorized by this card until a trusted
prompt-boundary signal is identified. A later card is required if source
evidence proves that CPA can safely generate a new ID at that boundary.

## Allowed files

- `internal/runtime/executor/helps/claude_prompt_state.go`
- `internal/runtime/executor/helps/claude_prompt_state_test.go`
- `internal/runtime/executor/claude_executor_request.go` only for a
  billing-field parser/preserver owned by this signal
- `internal/runtime/executor/claude_executor_execute.go` and
  `internal/runtime/executor/claude_executor_stream.go` only to pass the
  prompt-state lifecycle value after the boundary contract is proven
- This change card and its evidence artifacts

## Prohibited changes

No changes to model, Tools, system prompt, workspace/path data, beta sets,
CCH algorithm or timing, UA, billing entrypoint, `cc_prev_req` semantics,
diagnostics, session/request/device IDs, TLS, transport/connection cache,
OAuth, proxy behavior, public configuration, retry policy, or non-Claude
providers. The session ID must not be used as the prompt ID, and a random
value per HTTP request is prohibited.

## Control matrix

- First request in a session: no prior prompt state; preserve a caller value
  or remain absent.
- Same prompt with tool-use continuation: the ID remains unchanged.
- New user prompt in the same session: a distinct boundary is required before
  a new ID may be generated.
- Retry, cancellation recovery and resume: do not commit a new ID unless the
  boundary is explicitly new; failed or cancelled requests do not advance
  state.
- Parallel requests for one prompt: they may observe the same prompt state;
  another session or agent must not.
- Caller-owned ID: preserve exactly and do not create a CPA-owned generation.
- Missing/ambiguous boundary: keep the field absent and emit an audit trace;
  do not guess.

## Acceptance and falsifiers

Acceptance requires one state owner to produce stable IDs within a verified
prompt/tool loop, distinct IDs for two prompts in one session, isolation for
two sessions/agents and credentials, no advancement after a failed or
cancelled response, and identical Execute/ExecuteStream rules. Existing
caller-owned IDs must be unchanged.

The card is falsified and must stop if IDs cross a session/credential/agent
boundary, change on an idempotent retry, appear without a verified boundary,
are altered by CCH or `cc_prev_req`, or Execute and ExecuteStream diverge.

## Validation and rollback

- Baseline: the exact `52017a63` tree and its focused executor/helper tests.
- Modified: focused prompt-state tests, existing S4 regression tests and
  `git diff --check`; CI workflow is required before release consideration.
- Rollback: an executable independent-copy rollback must restore the exact
  baseline commit and remove every S5-only path. No OAuth, email, proxy
  credential or raw secret may enter source or artifacts.

## Release gate

This card does not claim strict A/B equivalence. It does not authorize a
missing-ID generator, a production default change, or S6 lifecycle work. The
release conclusion remains `NOT STRICTLY EQUIVALENT` until the card's evidence
and the later prompt-boundary generation decision are independently approved.
