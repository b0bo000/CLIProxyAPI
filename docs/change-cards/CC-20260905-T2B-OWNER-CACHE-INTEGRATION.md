# Change Card: Claude Transport Owner Cache Integration

- Signal ID: `pool/ownership` (process-owner boundary subset)
- Card owner: CPA Claude compatibility workstream
- Branch: `feat/s3-resolved-profile`
- Baseline commit: `41ec820451ebc21484e762ad65f2a9a4635d32e6`
- Evidence baseline: `artifacts/CPA-CLAUDECODE-DOCS-20260829/00-FIRST-READ.md`, sections 5.3, 8, 91 and 92

## Current Classification

T2-A defines an opaque owner capability but does not affect transport lookup.
Before this card, two contexts with the same proxy and credential scope could
still select one cached Claude transport. The signal remains
`VISIBLE_CONFOUNDED`; this card changes only the source-level owner dimension
and does not claim that every caller currently supplies one.

## Hypothesis

When a verified downstream owner attaches `ClaudeCodeTransportOwner` to the
request context, Claude transports and their connection/TLS session state are
reused only within that owner. Requests without an owner retain T1 credential
scoping, so existing callers do not silently become globally process-scoped.

## Allowed Changes

- Add an owner pointer to the private Claude transport cache key.
- Route `NewUtlsHTTPClient` context owners into Claude transport lookup.
- Preserve the existing helper signature as an ownerless compatibility path.
- Add focused tests for same-owner reuse, different-owner isolation, ownerless
  fallback, and context-to-client propagation.
- Add this card's verification artifacts under
  `artifacts/CPA-CLAUDECODE-T2B-OWNER-CACHE-INTEGRATION-20260905/`.

## Prohibited Changes

- Do not auto-create an owner in `NewClaudeExecutor`, `NewUtlsHTTPClient`, or a
  shared executor; a missing owner must remain the T1 fallback.
- Do not derive ownership from headers, body fields, session/request IDs,
  OAuth values, device metadata, PIDs, process names, or arbitrary strings.
- Do not change TLS ClientHello, JA3, ALPN, PSK policy, HTTP headers/body,
  model, Tools, system prompt, beta values, CCH, IDs, OAuth, proxy selection,
  retry policy, or production configuration.

## Expected Behavior

| Context | Expected transport behavior |
|---|---|
| Same credential, same proxy, same owner | Same cached transport |
| Same credential, same proxy, different owner | Different cached transports |
| Different credential, same proxy, same owner | Different cached transports |
| Same credential, different session, same owner | Same cached transport |
| Same credential, same proxy, no owner | T1 credential-scoped fallback |
| Nil auth, no owner | Existing proxy-only compatibility lookup |
| Nil auth, distinct owners | Distinct owner-scoped transports |

The owner key is a process-local pointer to the opaque T2-A token. It is
comparable for cache lookup, is never serialized or logged, and does not add
identity material to the key.

## Validation

- Baseline and modified focused `helps` tests.
- Modified full `internal/runtime/executor` package tests.
- `git diff --check`.
- Apply the exact patch in a separate copy, run modified owner/cache tests,
  execute `ROLLBACK.sh`, then run the baseline focused suite.
- Falsifying result: same-owner reuse fails, different owners share a
  transport, ownerless T1 behavior changes, or any prohibited signal changes.

## Rollback

```text
bash artifacts/CPA-CLAUDECODE-T2B-OWNER-CACHE-INTEGRATION-20260905/ROLLBACK.sh <test-copy> <artifact-directory>
```

## Status

Implemented and rollback-validated in commit `955a88d5`. The independent-copy
patch, modified tests, rollback script, and post-commit regression checks are
recorded in the T2-B artifact directory. This card does not authorize a
production restart or deployment.
