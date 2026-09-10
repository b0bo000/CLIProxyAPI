# Change Card: S6-D Compact/Count-Tokens and 529 Recovery Audit

## Identity

- Signals: `G-07 compact_count_tokens_graph`, `G-08 transient_529_retry_recovery`
- Branch: `feat/s6b-404-recovery`
- Baseline commit: `586e4360`
- Date: 2026-09-08
- Observation boundaries: real B-group Claude Code capture, local controlled-fault boundary, CPA unit tests

## Evidence

The B-group run `artifacts/CPA-ClaudeCode-risk-harness-20260823/runs/remaining-live-20260906-074304/`
contains 49/49 complete eight-stage HTTP 200 captures. Its `automatic_compact`
scenario includes successful count_tokens requests and subsequent SDK requests
with a reorganized message body while preserving context-management state.

The fault-injection artifact
`artifacts/CPA-CLAUDECODE-FAULT-INJECTION-20260906/` records a 2xx-only local
response boundary. With a valid Messages 2xx, the three recovery modes were
validated using Claude Code 2.1.241: one 529 conversion recovered as
`200,529,200,200,200`; response-before-exposure reset recovered as
`error,200,200,200,200`; and mid-SSE truncation recovered as
`unexpected EOF,200,200,200,200`. The sidecar records the original upstream
status as 200, so these are controlled local faults, not Anthropic-native faults.

## Scope and non-claims

- Compact/count_tokens success and state continuity are `CAPTURED_B` for the
  observed workload.
- 529 retry/recovery is `CONTROLLED_LOCAL_FAULT_VALIDATED`.
- Anthropic-native 529, reset, or mid-SSE behavior remains `UNTESTED` because
  no client-side experiment can force the provider to emit those exact faults.
- Provider-side raw H1, strict A/B equivalence, and account lifetime are outside
  this card.
- No source, production configuration, OAuth, proxy, or service behavior was
  changed by this audit.

## Acceptance and falsifiers

Acceptance requires the 49/49 B capture, the three controlled-fault sequences,
and focused compact/cooldown tests to pass. Falsifiers are a missing count_tokens
stage, loss of compact continuity, retry after a non-2xx injection boundary, or
credential/model cooling caused solely by cancellation/controlled transport
fault handling.

## Decision

`S6-D AUDIT CLOSED FOR CAPTURED_B_AND_CONTROLLED_LOCAL_FAULT_SCOPE`.

The overall release remains `NOT STRICTLY EQUIVALENT`; provider-native fault
emission and the independent transport/TLS/open-boundary signals remain open.
