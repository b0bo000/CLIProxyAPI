# Change Card: CC-20260903-S4B3-DIAGNOSTICS-COMMIT

## Identity

- Signal group: `B-03/B-10` diagnostics completion and continuity
- Scope: tighten diagnostics success validation and commit timing after S4-B.2 wiring
- Branch: `feat/s3-resolved-profile`
- Baseline commit: `d544860f3a4f8a13b15c34be9ed1ced6a3d65047`
- Date: 2026-09-03 (America/Los_Angeles)

## Hypothesis

Diagnostics state must advance only when the upstream Claude response is a
complete successful message. A `2xx` status alone is insufficient: malformed,
non-message, non-string-ID, incomplete, error-event, duplicate-event,
post-stop or cancelled streams must not become the next
`previous_message_id`.

## Allowed change

- Add a diagnostics-owned strict response/SSE completion validator.
- Use it in `Execute` and `ExecuteStream` before committing diagnostics state.
- Move non-stream diagnostics commit after response restoration and translation
  have completed successfully.
- Keep the existing S4-B.1 eligibility boundary and tracker unchanged.

## Prohibited change

No changes to `cc_prev_req`, prompt IDs, CCH, body schema, model, Tools,
system prompts, beta selection, headers, TLS, HTTP transport/connection cache,
OAuth, proxy, public configuration or diagnostics state storage/TTL.

## Verification

Baseline is the detached `d544860f` tree. Modified tests cover valid and
invalid JSON message responses, valid and invalid SSE event sequences, both
Execute and ExecuteStream continuity, and preservation of the previous state
after a failed/incomplete response. Full executor and helper regression tests
are run in both trees. An executable rollback must restore the exact baseline
tree in an independent worktree.

## Release gate

This closes only diagnostics success-commit strictness. It does not establish
strict A/B equivalence or Anthropic receiver-side observability; release remains
`NOT STRICTLY EQUIVALENT` until later gates are independently completed.

