# Change Card: S7 Configurable Claude Code TLS Session Policy

## Identity

- Signal group: `T-04 tls_session_resumption` and `T-05 transport cache policy`
- Branch: `feat/s6b-404-recovery`
- Baseline commit: `586e4360`
- Date: 2026-09-08
- Observation boundary: CPA source, unit tests, and local TLS handshake fixtures

## Evidence and current classification

The current implementation always attaches a per-transport uTLS client-session
cache to the Claude inference transport. Existing A/B captures show different
PSK/resumption counts, but host, egress, account, and workload are confounded;
the captures do not justify changing the default or claiming strict parity.

Current classification: `VISIBLE_CONFOUNDED`; implementation action is an
opt-in policy switch, not a hard-coded baseline rewrite.

## Hypothesis

Operators need to run a controlled no-resumption comparison without changing
the existing default. The policy must be part of the transport-cache key so an
enabled and disabled transport can never share a TLS session cache or idle
connection pool.

## Allowed changes

- Add `claude-code.tls-session-resumption` as an optional boolean.
- Preserve the current enabled behavior when the field is omitted or `true`.
- When explicitly `false`, construct the Claude inference transport without a
  client session cache and therefore without resumable PSK state.
- Include the effective policy in the opaque transport-cache key.
- Add configuration, cache-key, and handshake-focused tests and documentation.

## Prohibited changes

- No model, tools, system prompt, body, beta, CCH, device/account/session/request
  ID, User-Agent, proxy, OAuth, retry, or caller-classification changes.
- No changes to the Claude OAuth control-plane transport.
- No production configuration or service restart.
- No claim that the switch proves Anthropic receiver-side behavior or strict
  A/B equivalence.

## Acceptance criteria

1. Omitted and explicit `true` resolve to enabled and preserve existing tests.
2. Explicit `false` resolves to disabled; its cache key differs from enabled.
3. Same policy + same credential/proxy/owner reuses a transport; different
   policy never reuses it.
4. The disabled TLS config has a nil session cache; the enabled config has a
   bounded cache and keeps the existing ClientHello ordering safeguards.
5. Focused baseline/modified tests and an independently tested rollback pass.

## Falsifiers and confounders

Falsifiers are a shared transport across policy values, a changed default when
the field is omitted, or any source diff outside this card. Wire-level PSK
counts remain workload/network observations and are not used as causal proof.

## Artifacts

- `artifacts/CPA-CLAUDECODE-S7-TLS-SESSION-POLICY-20260908/`
- `artifacts/CPA-CLAUDECODE-S7-TLS-SESSION-POLICY-20260908/VERIFICATION.txt`
- `artifacts/CPA-CLAUDECODE-S7-TLS-SESSION-POLICY-20260908/ROLLBACK.sh`

## Rollback

Rollback restores each source file from the independent-copy originals in the
artifact directory. The command and hash result are recorded in `VERIFICATION.txt`.
