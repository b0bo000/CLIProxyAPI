# Change Card: Credential-Scoped Claude Transport Cache

- Signal ID: `pool/ownership` (credential-boundary subset)
- Card owner: CPA Claude compatibility workstream
- Branch: `feat/s3-resolved-profile`
- Baseline commit: `e2d2661a0f9064c487f971506f6759ef796febe5`
- Baseline source: `internal/runtime/executor/helps/utls_client.go`
- Evidence baseline: `artifacts/CPA-CLAUDECODE-DOCS-20260829/00-FIRST-READ.md`, sections 5.2 and 8; `artifacts/CPA-ClaudeCode-STRICT-ONE-TO-ONE-REANALYSIS-20260827/STRICT-MATRIX.json`

## Current Classification

The CPA Claude RoundTripper cache is keyed primarily by `proxyURL`. The
observed B topology therefore permits different credential contexts on the
same proxy to reach one transport and its connection/session state. The
classification is `VISIBLE_CONFOUNDED`; this card addresses only the source
confirmed credential boundary. It does not claim process identity or strict
official equivalence.

## Hypothesis

Including a non-secret, stable credential scope in the cache key prevents
different credentials from sharing a Claude transport, connection pool, or TLS
session cache, while preserving reuse for the same credential across sessions.

## Allowed Changes

- Add a private cache-key/scope helper under `internal/runtime/executor/helps/`.
- Change Claude transport cache lookup in `utls_client.go` to include the
  resolved credential scope.
- Add focused unit tests for scope precedence, isolation, reuse, LRU bounds,
  and the unknown-identity compatibility path.
- Add this card's verification artifacts under
  `artifacts/CPA-CLAUDECODE-T1-CREDENTIAL-TRANSPORT-SCOPE-20260905/`.

## Prohibited Changes

- No changes to TLS ClientHello, JA3, ALPN, PSK policy, HTTP headers, body,
  model, Tools, system prompt, beta values, CCH, session/request/prompt IDs,
  OAuth, proxy selection, retry policy, or production configuration.
- Do not derive scope from access/refresh tokens, email, account UUID, session
  ID, request body, headers, or caller-provided metadata.
- Do not add process-owner behavior; that is a separate T2 card.

## Expected Behavior

| Context | Expected transport behavior |
|---|---|
| Same credential, same proxy | Same cached transport |
| Same credential, different session | Same cached transport |
| Different credentials, same proxy | Different cached transports |
| Same credential, different proxy | Different cached transports |
| Credential ID changes only by OAuth token refresh | Same scope when auth ID is stable |
| Nil auth | Existing proxy-only compatibility lookup |
| Non-nil auth with no stable ID/index/file | Uncached transport; no unsafe sharing |

The scope is a SHA-256 digest with a fixed domain prefix. Only the digest is
used as the key; the source identity is never logged or persisted by this
change.

## Validation

- Baseline and modified focused `helps` tests.
- Baseline and modified `internal/runtime/executor` package tests.
- `git diff --check`.
- Apply the exact patch in a separate copy, run the modified focused test,
  execute `ROLLBACK.sh`, then run the baseline focused test.
- Falsifying result: any cross-credential transport reuse, same-credential
  reuse regression, LRU bound regression, or change outside the allowed files
  stops the card.

## Rollback

```text
bash artifacts/CPA-CLAUDECODE-T1-CREDENTIAL-TRANSPORT-SCOPE-20260905/ROLLBACK.sh <test-copy>
```

## Baseline Evidence

- Branch: `feat/s3-resolved-profile`
- Commit: `e2d2661a0f9064c487f971506f6759ef796febe5`
- `utls_client.go` SHA-256: `2FE33C1531591A72BD2A0F24CCB14E301F4B47041C34AA76F482297A22AECC1B`
- `utls_client_test.go` SHA-256: `E7E7C0592AA8A9C7C045CF81DF2FEB8A5F7B4C979586A3EB71EA0E2BA4ED07EE`
- `go test ./internal/runtime/executor/helps -count=1`: exit `0`
- `go test ./internal/runtime/executor -count=1`: exit `0`
- `git diff --check`: exit `0`
