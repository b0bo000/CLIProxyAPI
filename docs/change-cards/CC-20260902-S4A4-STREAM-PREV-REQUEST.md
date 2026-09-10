# S4-A.4 Streaming `cc_prev_req` Integration

## Signal And Classification

- Signal ID: `body/cc_prev_req`
- Classification: `CONFIRMED_DIFFERENCE` from the canonical A/B evidence.
- Scope: `ClaudeExecutor.ExecuteStream`, direct Claude SSE and translated SSE.
- Baseline: `6256c8e9b9bc0ff7d4baa588fdd3fca2b7efbef4`.

## Atomic Scope

S4-A.4 reuses the independently keyed previous-request state from S4-A.1 and
the billing insertion from S4-A.2 in the streaming execution path. It begins
after final body preparation and before CCH signing. It inserts only a real
previous successful upstream `request-id`, and only when the caller has not
provided `cc_prev_req`.

The direct Claude and translated response goroutines both validate the same
upstream SSE sequence. The state commit occurs only after clean EOF and a
complete `message_start`/`message_delta`/`message_stop` sequence, with a
non-empty string message ID and model, a live context, and one valid `req_`
request ID from the response headers.

## Non-Commit Conditions

Non-2xx responses, malformed SSE data, upstream error events, data after
`message_stop`, missing stop, read/scanner/decode errors, cancellation, invalid
request IDs, missing scope, ineligible callers, and caller-owned billing text
do not advance CPA state.

## Tests

`claude_executor_prev_request_stream_test.go` covers direct and translated
success, CCH coverage, all non-commit conditions, cancellation, caller
ownership, and independent agent/credential/session scopes. The focused Stream
suite passed once and with `-count=10`; the Execute regression suite,
helper/state regression suite, and full executor package also exited 0.

## Unchanged And Deferred

Diagnostics, `cc_prompt_id`, TLS/JA3/ALPN/PSK, RoundTripper/cache ownership,
model/tools/prompts/betas, the CCH algorithm, OAuth, proxy routing, production
configuration, and S2-B owner integration are unchanged. The independent
`InjectPrevRequest` policy/config gate is S4-A.5 and is not enabled by this
commit. This intermediate state is not deployable and does not establish strict
Claude Code equivalence; release remains `NOT STRICTLY EQUIVALENT`.

## Evidence And Rollback

Evidence is under
`artifacts/CPA-CLAUDECODE-S4A4-STREAM-PREV-REQ-20260902`. It contains the
source patch, baseline and modified test records, SHA-256 manifest, and an
executable reverse-patch script. The script was run against an independent copy
and restored the exact baseline `6256c8e9` while the live branch remained at
the S4-A.4 implementation commit recorded in the evidence manifest.

## Approval Gate

S4-A.4 is complete for its intermediate Stream implementation. The next
independent approval gate is S4-A.5: add and test `InjectPrevRequest` policy
gating without coupling it to diagnostics. No S4-A.5 source change is included
here.
