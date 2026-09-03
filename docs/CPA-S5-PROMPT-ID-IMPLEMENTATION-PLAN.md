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
