# Change Card: CC-20260903-S5-2-PROMPT-BOUNDARY

## Identity

- Signal: `B-12 cc_prompt_id`
- Secondary graph signal: `G-03 same_process_multi_prompt`
- Step: S5-2, trusted prompt-boundary audit
- Branch: `feat/s3-resolved-profile`
- Source baseline: `52017a63b044595fc9457a05ea2edd3223e4957a`
- Documentation parent: `8705828ede4d0a875fce30da38f098b33db30461`
- Date: 2026-09-03 (America/Los_Angeles)

## Purpose

Determine whether the retained A/B observations identify a trustworthy user-
prompt boundary for `cc_prompt_id`. This card is an audit and contract step;
it does not generate a missing field and does not change production behavior.

## Evidence boundary

- A source: `artifacts/ClaudeCode-A-remote-official-gap-suite-20260824/`.
  The inventory contains 194 requests, 190 complete records and 184 HTTP 200
  responses. The strict native SDK denominator is 162/162 with a prompt ID.
- B source: `artifacts/CPA-ClaudeCode-2.1.241-B-prepared-20260823/runs/`
  and `artifacts/CPA-ClaudeCode-risk-harness-20260823/runs/`.
  The retained CPA upstream corpus has 60 sanitized records and the gap
  stages have 240 body-stage records; native `cc_prompt_id` is absent at the
  retained incoming/final boundary.
- A/B host, egress, credential cohort and workload are different by design.
  They are confounders and are not normalized away.
- Anthropic receiver-side raw H1 remains `UNOBSERVABLE_FROM_CLIENT /
  UNTESTED`; local relay evidence is not receiver evidence.

## Observed contract

| Result | Meaning | Evidence rule |
|---|---|---|
| `absent` | No native prompt ID is present for this operation | title, count_tokens and compact operations; all retained B native incoming/final records |
| `new(boundary)` | A caller value changes at a demonstrated user-prompt boundary | first SDK request of a fresh prompt; each new prompt in the A multiprompt task; post-cancel/recovery and post-compact prompt values are observed but commit semantics remain separate |
| `continue(existing)` | The same value remains across one verified request set | A tool-loop SDK sequence and A parent/child subagent sequence |
| `ambiguous` | A lifecycle event cannot identify whether a new prompt exists | process-restart resume, failure-only or incomplete records, and any request with no trusted caller boundary |

The labels are observations, not an algorithm. Session ID, request ID,
message ID, timing, PID, body length and random allocation are not accepted as
prompt-boundary substitutes.

## Acceptance criteria

1. The ledger records every A task and the B native capture denominator with
   redacted hashes only.
2. A same-prompt tool loop is stable, and the A same-process three-prompt
   task has three distinct values at three observed boundaries.
3. Title/count_tokens/compact absence is kept separate from SDK presence.
4. Failed, cancelled or incomplete operations are marked non-committing.
5. Process-restart resume is explicitly unresolved rather than inferred.
6. B absence is stated as incoming-shape absence; the record does not claim
   that CPA deleted a caller field.
7. No production source, public configuration, CCH, `cc_prev_req`, diagnostics,
   transport, TLS, OAuth, proxy, model, Tools or prompt content changes.

## Stop conditions and falsifiers

Stop S5-2 if an ID crosses credential/session/agent scope, changes during an
idempotent retry, is synthesized without an identified boundary, or if
Execute and ExecuteStream would require different rules. A caller-provided
valid ID must remain byte-for-byte caller-owned.

## Next gate

S5-2 closes only the evidence and boundary-definition scope. S5-3 remains
blocked until a trusted caller adapter contract or captured caller boundary
is available. Only then may a separate source commit consider lifecycle state
or missing-ID generation.

Canonical evidence: `artifacts/CPA-CLAUDECODE-S5-2-PROMPT-BOUNDARY-AUDIT-20260903/`.

## S5-2.2 promptId routing correction

This addendum supersedes only the earlier interpretation of local JSONL; it
does not rewrite or discard the original S5-2 measurements. The runner's
`claude.stdout.jsonl` has zero `promptId` fields, but it is not the complete
Claude Code session transcript.

The official A session transcripts contain 14 prompt hashes. Thirteen match
all 13 prompt hashes observed in the official upstream billing header; the
remaining hash belongs only to the explicit compact transcript event. B
session transcripts contain 17 prompt hashes, but a complete scan of all 688
retained CPA stage/capture files finds none of those exact values and no
`promptId` or `cc_prompt_id` field names. All five B session IDs do occur in
the capture corpus, so the missing prompt value is not evidence that session
association itself disappeared.

Static inspection of the saved official 2.1.241 binary confirms that the
billing builder validates a UUID prompt value and emits `cc_prompt_id` only
when provider mode is `firstParty` and the base-URL gate accepts
`api.anthropic.com`. `cc_prev_req` has the same gate. Thus B's official client
creates caller-owned prompt state but suppresses these billing values before
the custom-base-URL request reaches CPA.

CPA cannot reconstruct the exact opaque caller value from another ID or from
request content. S5-3 remains `BLOCKED_FOR_CPA_ONLY_GENERATION`; any future
work requires a separately approved trusted caller-to-CPA forwarding contract.
The withdrawn S5-3A attempt remains withdrawn. Canonical correction evidence:
`C:/Users/Administrator/gpt-5.6-instruct/artifacts/CPA-CLAUDECODE-S5-2.2-PROMPTID-ROUTING-AUDIT-20260903/`.
