# Change Card: S6-C In-Flight Cancel and Recovery Release Audit

## Identity

- Signals: `G-06 inflight_cancel_recovery`, `R-02 successful_sse_message_stop` (cancellation subset)
- Branch: `feat/s6b-404-recovery`
- Baseline commit: `153d812f`
- Date: 2026-09-08
- Observation boundaries: CPA manager/executor tests and local eight-stage capture

## Evidence

The valid B-GAP run `gap-only-20260828-060618` contains one intentional in-flight
`context canceled` request followed by a same-session HTTP 200 recovery. The
2026-09-08 `reset-before-response` and `mid-sse-once` runs independently exercise
the same recovery boundary after a real upstream 2xx is observed; the synthetic
fault is consumed only at the local CPA response boundary and is not described as
an Anthropic-originated fault.

Focused tests cover preparation cancellation, refresh cancellation, stream-tail
cancellation, connection-lifecycle classification, and the negative control that
a real HTTP 500 still cools the model.

## Scope and invariants

This card audits existing behavior only. No source/configuration change is made.

- Cancellation and connection-lifecycle failures remain availability-neutral.
- A real provider failure retains normal cooldown semantics.
- Failed or canceled requests do not advance `cc_prev_req`.
- Recovery remains in the original session and may reuse the existing transport.
- No new retry loop, body rewrite, TLS change, or OAuth mutation is introduced.

## Acceptance / falsifiers

Acceptance requires all focused tests to exit 0 and the existing B evidence to
show cancel/close followed by a successful same-session request. A falsifier is
credential/model cooling after cancellation, duplicate side effects, continuity
advancement from the canceled request, or a source diff outside this card.

## Verification and decision

- Artifact directory: `artifacts/CPA-CLAUDECODE-S6C-CANCEL-RELEASE-20260908/`.
- Baseline and modified commands, literal outputs, and rollback result are in
  `VERIFICATION.txt`.
- Independent-copy rollback returned `RESTORED=1; HASH_MATCH=True`.
- Production `CLIProxyAPI` remained `Running`; no production source or config was
  modified.

Decision: `S6-C RELEASE-GRADE-AUDIT-CLOSED_BY_EXISTING_EVIDENCE_AND_TESTS`.
The overall release remains `NOT STRICTLY EQUIVALENT`.
