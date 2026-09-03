# S5-2 Prompt Boundary Audit

## Scope and result

This audit evaluates whether existing real captures identify when a user
prompt starts, continues through tools, retries, cancellation recovery,
resume, subagent execution or compact. It covers Claude Code 2.1.241. A is
the remote official CLI direct to Anthropic; B is local Claude Code through
the isolated CPA instance. Host, egress, credential cohort and workload
remain intentional confounders.

**Result:** A exposes a stable caller-owned prompt value within verified
request loops and changes it at observed new-prompt boundaries. B's retained
native upstream shape has no `cc_prompt_id`; the captures do not identify a
trusted boundary from which CPA could safely synthesize one. S5-2 is therefore
an evidence-complete, boundary-unproven step. S5-3 generation is blocked.

## A request-sequence ledger

`prompt_id_count` counts distinct SHA-256 values in the task's SDK records.
`new` and `continue` describe observed value transitions; they do not mean
CPA state was committed. `failed_or_incomplete` marks records that cannot be
used as a successful lifecycle commit.

| Task | Scenario | Requests | SDK | Prompt IDs | New | Continue | Status / special rule |
|---|---|---:|---:|---:|---:|---:|---|
| task-01 | cold-sequential | 9 | 8 | 1 | 1 | 7 | all 200 |
| task-02 | cold-sequential | 7 | 6 | 1 | 1 | 5 | all 200 |
| task-03 | cold-sequential | 8 | 7 | 1 | 1 | 6 | all 200 |
| task-04 | hot-single-process | 19 | 18 | 1 | 1 | 17 | all 200 |
| task-05 | concurrent-2 | 13 | 12 | 1 | 1 | 11 | all 200 |
| task-06 | concurrent-2 | 7 | 6 | 1 | 1 | 5 | all 200 |
| task-07 | concurrent-5 | 13 | 12 | 1 | 1 | 11 | all 200 |
| task-08 | concurrent-5 | 8 | 7 | 1 | 1 | 6 | all 200 |
| task-09 | concurrent-5 | 11 | 10 | 1 | 1 | 9 | all 200 |
| task-10 | concurrent-5 | 9 | 8 | 1 | 1 | 7 | all 200 |
| task-11 | concurrent-5 | 13 | 12 | 1 | 1 | 11 | all 200 |
| task-12 | resume-same-session | 8 | 7 | 1 | 1 | 6 | first lifecycle |
| task-13 | resume-same-session | 3 | 3 | 1 | 1 | 2 | same session, new process/lifecycle value; unresolved inheritance |
| task-14 | invalid-model | 3 | 2 | 1 | 1 | 1 | 1 title 200, 2 SDK 404; non-committing |
| task-15 | cancel-in-flight | 2 | 1 | 1 | 1 | 0 | SDK incomplete; non-committing |
| task-01 | resume-three-step | 8 | 7 | 1 | 1 | 6 | first lifecycle |
| task-02 | resume-three-step | 5 | 5 | 1 | 1 | 4 | same session, distinct lifecycle |
| task-03 | resume-three-step | 4 | 4 | 1 | 1 | 3 | same session, distinct lifecycle |
| task-04 | multiprompt-single-process | 8 | 7 | 3 | 3 | 4 | three prompt boundaries in one process |
| task-05 | subagent-parent | 5 | 2 SDK + 2 agent | 1 | 1 | 3 | parent/child loop retains value |
| task-06 | invalid-model-recovery | 3 | 2 | 1 | 1 | 1 | 404 and incomplete title; non-committing |
| task-07 | invalid-model-recovery | 2 | 2 | 1 | 1 | 1 | recovered lifecycle value |
| task-08 | cancel-resume | 2 | 1 | 1 | 1 | 0 | cancelled/incomplete; non-committing |
| task-09 | cancel-resume | 5 | 5 | 1 | 1 | 4 | recovery lifecycle value |
| task-10 | explicit-compact | 19 | 14 | 2 | 2 | 12 | prompt IDs absent on title/count_tokens/compact; 2 responses 529 |

The full per-request rows, including session and request-ID hashes, are in
`PROMPT-BOUNDARY-LEDGER.json`. The source inventory is
`artifacts/ClaudeCode-A-remote-official-gap-suite-20260824/OFFICIAL-A-COMPLETE-REQUEST-INVENTORY.json`.
For `task-05`, the `Continue=3` count includes both prompt-bearing agent rows;
the `SDK` column deliberately reports `2 SDK + 2 agent` rather than folding
agent traffic into the SDK denominator.

## Exact observed transitions

- `task-04-multiprompt-single-process`: sequence 2 starts value
  `6e86bf23f6a2`, sequence 3 continues it; sequence 4 starts
  `f722b4d26aff`, sequence 5 continues it; sequence 6 starts
  `8956ab0ccdc8`, sequences 7-8 continue it. All three values are in one
  session and one process.
- `task-05-subagent-parent`: parent SDK and agent requests retain the same
  value. This proves loop-level stability at the A observation boundary, not
  a provider identity rule.
- `task-12` followed by `task-13`: the session hash remains the same but the
  prompt hash changes after the resumed process. This is evidence of a new
  observed lifecycle value, not proof that resume always creates or always
  preserves a value.
- `task-10-explicit-compact`: count_tokens and compact records are absent;
  the post-compact SDK records use a second value. Compact itself is not a
  prompt-ID commit.
- 404 and cancellation records contain candidate values but are failed or
  incomplete. They cannot advance a successful state.

## B boundary audit

The main B corpus contains 60 sanitized upstream records: 5 title and 55 SDK
requests, 5 sessions and peak concurrency 6. Every retained record has
`cc_prompt_id_sha256: null`. The gap suite contains 240 body-stage records
from incoming through final RoundTrip; native `cc_prompt_id` is absent at all
retained stages. The B-02 stream runner still shows one session, four requests,
body growth and unique request IDs, but its local `promptId` JSONL field is
debug/process metadata and is not the upstream billing field.

Evidence paths:

- `artifacts/CPA-ClaudeCode-2.1.241-B-prepared-20260823/runs/B-via-CPA-20260823-122414/B-CAPTURE-ANALYSIS.json`
- `artifacts/CPA-ClaudeCode-2.1.241-B-prepared-20260823/runs/B-via-CPA-20260823-122414/upstream-http-sanitized/`
- `artifacts/CPA-ClaudeCode-risk-harness-20260823/runs/gap-only-20260828-060618/upstream-http-raw-private/`
- `artifacts/CPA-ClaudeCode-risk-harness-20260823/runs/b06-restart-resume-20260828-060917/`
- `artifacts/CPA-ClaudeCode-risk-harness-20260823/runs/b07-pid-ledger-20260828-060944/`

This supports `B-12 = CONFIRMED_DIFFERENCE` and `G-03 = CAPTURED_B_PARTIAL`.
It does **not** support the statement “CPA deleted `cc_prompt_id`”; the field
is already absent from the captured B incoming shape.

## Boundary decisions

| Lifecycle event | Decision | Why |
|---|---|---|
| first SDK prompt with caller value | `new(boundary)` observed; preserve | caller owns the value |
| same prompt tool loop / verified subagent loop | `continue(existing)` | value is stable across request set |
| second user prompt in same process | `new(boundary)` observed only when caller value changes | the transition must come from caller/adapter |
| title, count_tokens, compact | `absent` | no native prompt field in A records |
| idempotent retry | `ambiguous` until caller marks same prompt | request ID/timing cannot decide |
| cancellation recovery | `ambiguous` for generation; failed record never commits | captured recovery is a new lifecycle but no CPA boundary contract |
| process restart resume | `ambiguous` | same session does not determine prompt inheritance |
| parallel requests | `ambiguous` without caller grouping | concurrency does not identify prompt ownership |
| B native request | `absent` | verified at retained incoming/final boundary |

## Disposition

No missing-ID generator is authorized. Do not use session ID, request ID,
message ID, PID, timestamp, body hash, timing heuristic or random value. A
future S5-3 card must first define a trusted caller adapter that returns
`absent`, `continue(existing)`, `new(boundary)` or fail-closed `ambiguous`,
then prove scope isolation, retry/cancel semantics and Execute/Stream parity.
Release remains `NOT STRICTLY EQUIVALENT`.
