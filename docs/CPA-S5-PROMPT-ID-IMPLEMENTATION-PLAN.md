# S5 Implementation Plan: Prompt Boundary and `cc_prompt_id`

## Authority

- Canonical baseline: `artifacts/CPA-CLAUDECODE-DOCS-20260829/00-FIRST-READ.md`
- Signal matrix: `artifacts/CPA-CLAUDECODE-DEVELOPMENT-PLAN-20260830/STEP-EVIDENCE-MATRIX.md`
- Change card: `docs/change-cards/CC-20260903-S5-CC-PROMPT-ID.md`
- Source baseline: `52017a63b044595fc9457a05ea2edd3223e4957a`
- Release status: `NOT STRICTLY EQUIVALENT`

## Why this is a separate step

The official A evidence shows a prompt ID that is stable across one tool loop
and changes at a new prompt boundary. CPA currently has session, previous
request and diagnostics state, but no prompt state. Session identity cannot
stand in for prompt identity because one session contains multiple prompts.
The current B incoming shape also lacks the field, so a blind generator would
create an unverified signal rather than restore a proven lifecycle.

## Atomic sequence

### S5-0: planning and source audit (this step)

Inputs:

- A strict SDK billing evidence and B incoming/final retained evidence.
- Existing `cc_prev_req` parser and S4 Execute/ExecuteStream state boundaries.
- Existing session/agent scope resolver.

Actions:

1. Register `B-12` and `G-03` in the change card.
2. List every current location that can preserve or rewrite the billing text.
3. Confirm that no current production path creates `cc_prompt_id`.
4. Define the state machine and falsifiers below.

Outputs:

- Change card and planning artifact only.
- No source behavior change and no release claim.

Stop conditions:

- A caller-provided field would be overwritten.
- The only candidate boundary is session ID, request ID, timing, or a random
  per-request value.

### S5-1: caller-owned field parser/preserver (one source commit)

Allowed files:

- `internal/runtime/executor/helps/claude_prompt_state.go`
- `internal/runtime/executor/helps/claude_prompt_state_test.go`
- The billing parser/insertion owner in
  `internal/runtime/executor/claude_executor_request.go`

Actions:

1. Parse exactly one `cc_prompt_id` from one valid billing block.
2. Reject empty, duplicated, malformed or control-bearing values without
   changing the body.
3. Preserve a valid caller-owned value and its relative billing order.
4. Prove that S4 `cc_prev_req` insertion and CCH finalization do not alter the
   prompt ID.

No missing-ID generation is allowed in S5-1. A missing field remains absent.

Acceptance:

- String and array system shapes round-trip the caller value.
- Duplicate/invalid values fail closed.
- Execute and ExecuteStream use the same parser result.
- Existing S4 focused tests remain green.

### S5-2: trusted prompt-boundary adapter (separate approval and commit)

This step is blocked until a real caller boundary signal is identified in a
captured request or an explicit caller adapter contract. Candidate signals
must be recorded with source and observation boundary; no header is trusted
as process identity merely because it is caller-supplied.

Required distinctions:

- first prompt in a session;
- same prompt tool-use continuation;
- new user prompt in an existing session;
- idempotent retry and cancellation recovery;
- resume after process restart;
- separate agent, session and credential;
- parallel requests belonging to one prompt.

The adapter must return `absent`, `continue(existing)`, or `new(boundary)`;
`ambiguous` is fail-closed and does not generate a value.

### S5-3: lifecycle state and commit (separate source commit)

Only after S5-2 is proven:

1. Key state by the resolved credential identity plus session and agent scope.
2. Generate one opaque prompt ID only on `new(boundary)`.
3. Reuse it for the verified tool-loop and idempotent retry set.
4. Commit it only after the same success conditions used by S4 state.
5. Never advance after non-2xx, malformed/incomplete SSE, cancellation,
   translation failure or an ambiguous boundary.
6. Keep Execute and ExecuteStream transitions identical.

Required tests:

- fresh prompt, same-loop tool continuation, two prompts in one session;
- retry, cancel/recovery, resume and process restart;
- parallel requests for one prompt;
- two sessions, two agents and two credentials;
- caller-owned ID preservation and missing/ambiguous input;
- state expiry/eviction and rollback.

### S5-4: evidence review and release decision

Collect a redacted state trace and raw local incoming/final body diff. Review
whether the observed field came from the caller or CPA, and keep the provider
receiver boundary explicitly unobserved. Only the reviewed result can change
the matrix classification or open S6.

## State contract

```text
scope = credential_identity + session_id + agent_id
state = { prompt_id, boundary_generation, committed, expires_at }

missing/ambiguous -> absent, no allocation
caller-owned       -> preserve, no CPA allocation
new(boundary)      -> allocate one prompt_id, pending until success
continue           -> reuse pending/committed prompt_id
success            -> commit current prompt_id
failure/cancel     -> retain last committed prompt_id; do not advance
new session/agent/credential -> independent scope
```

## Explicit non-goals

This plan does not change model, Tools, system prompt, body size, beta set,
CCH, UA, billing entrypoint, session/request/device IDs, `cc_prev_req`,
diagnostics, TLS, transport/cache, OAuth, proxy, retry policy or public
configuration. It does not claim that a matching prompt ID proves strict
client equivalence.

## S5-2 execution log (2026-09-03)

S5-2 is complete as a documentation-only trusted-boundary audit. The detailed
card is `docs/change-cards/CC-20260903-S5-2-PROMPT-BOUNDARY.md`; the redacted
ledger and verification package are under
`artifacts/CPA-CLAUDECODE-S5-2-PROMPT-BOUNDARY-AUDIT-20260903/`.

The official A inventory has 194 requests, with 162/162 strict successful SDK
requests carrying `cc_prompt_id`. A same-process three-prompt task has three
distinct values, each stable across its tool loop; parent/subagent rows retain
the same value. Prompt IDs are absent on title, count_tokens and compact rows. Same-session
process-restart resume changes the observed prompt hash, so resume inheritance
is unresolved. Failed, cancelled and incomplete rows are transition evidence,
not successful state commits.

The retained B native corpus has 60 sanitized records and zero native prompt
IDs. A separate scan of 240 B gap body-stage files found zero `cc_prompt_id`
and zero local `promptId` occurrences. A local CLI JSONL `promptId` is not the
native billing field. The precise conclusion is that the B incoming shape lacks
the field and CPA has no verified lifecycle model to restore it; this does not
prove CPA deleted a caller field.

S5-2 defines `absent`, `continue(existing)`, `new(boundary)` and fail-closed
`ambiguous`. Session/request/message IDs, PID, timing, body length/hash and
per-request randomness are explicitly rejected as boundary substitutes. No
missing-ID generator is authorized. S5-3 remains blocked pending a trusted
caller boundary or adapter contract and parity tests for fresh prompts,
tool-loops, retries, cancellation recovery, restart/resume, subagents and
parallel requests. B-12 remains `CONFIRMED_DIFFERENCE`, G-03 remains
`CAPTURED_B_PARTIAL`, G-04 remains `CAPTURED_B_BOUNDARY_LIMITED`, and release
remains `NOT STRICTLY EQUIVALENT`.

## S5-3-A execution log (2026-09-03)

S5-3-A adds only the explicit adapter-owned boundary contract. The SDK now
offers a private-context-key carrier with `absent`, `new`, `continue` and
`ambiguous` kinds plus an opaque adapter transaction key. The Claude helper
validates the combinations and returns a request-scoped HTTP 400 for unknown,
malformed or contradictory hints. Execute and ExecuteStream run the same
pre-upstream validation guard.

This step does not allocate, persist, insert or commit `cc_prompt_id`; it does
not infer boundaries from headers, body, session/request/message IDs, PID,
timing or hashes; and an absent hint leaves the wire request unchanged. The
change card is `docs/change-cards/CC-20260903-S5-3A-PROMPT-BOUNDARY-CONTRACT.md`.

Validation on the modified tree returned
`CONTRACT_STATIC_OK MODE=MODIFIED CONTEXT_API=1 RESOLVER=1 EXECUTOR_GUARDS=2 PROMPT_GENERATOR=0`.
The exact `704f980a` baseline returned
`CONTRACT_STATIC_OK MODE=BASELINE CONTEXT_API=0 RESOLVER=0 EXECUTOR_GUARDS=0 PROMPT_GENERATOR=0`.
The independent-copy forward application, modified check, reverse rollback,
restored baseline check, `git diff --check` and clean-status check all exited
0. Local Go tests/builds were not run because the repository validation policy
prohibits compilation-heavy local commands; the authorized remote host had no
`go` executable. The evidence package is
`artifacts/CPA-CLAUDECODE-S5-3A-PROMPT-BOUNDARY-CONTRACT-20260903/`.

S5-3-B remains unopened: a trusted adapter must still supply real lifecycle
events, and only a separately approved step may add lifecycle state or decide
whether a missing prompt ID can ever be generated. Release remains
`NOT STRICTLY EQUIVALENT`.
