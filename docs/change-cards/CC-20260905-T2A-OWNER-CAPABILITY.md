# Change Card: Claude Transport Owner Capability

- Signal ID: `pool/ownership` (trusted process-owner substep)
- Card owner: CPA Claude compatibility workstream
- Branch: `feat/s3-resolved-profile`
- Baseline commit: `30611cd62f3977e77a44646ad685c312a2fa1807`
- Evidence baseline: `artifacts/CPA-CLAUDECODE-DOCS-20260829/00-FIRST-READ.md`, sections 5.3, 8 and 91; `artifacts/CPA-ClaudeCode-STRICT-ONE-TO-ONE-REANALYSIS-20260827/STRICT-MATRIX.json`

## Current Classification

Official CLI transport ownership is inferred at process scope, while CPA's
current credential-scoped transport key has no process-owner dimension. The
signal remains `VISIBLE_CONFOUNDED`. This card only defines a trusted owner
capability for a later cache integration; it does not claim that the caller
has already supplied a verified process identity.

## Hypothesis

An opaque capability created at a real downstream owner boundary and carried
through context can identify that owner without trusting mutable headers,
session IDs, body fields, or arbitrary caller strings.

## Allowed Changes

- Add a private owner token and context key under
  `internal/runtime/executor/helps/`.
- Export a constructor and context attachment function so a verified owner
  boundary can carry the capability to executor code.
- Add focused unit tests for creation, distinctness, propagation, zero-value
  rejection, nil-context handling, and context inheritance.
- Add this card's verification artifacts under
  `artifacts/CPA-CLAUDECODE-T2A-OWNER-CAPABILITY-20260905/`.

## Prohibited Changes

- Do not change transport cache keys or `utls_client.go` in T2-A.
- Do not auto-create one global owner in `NewClaudeExecutor` or any shared
  executor; that would collapse independent downstream processes.
- Do not accept owner values from headers, body, session/request IDs, OAuth,
  device metadata, PIDs, process names, or serialized user input.
- Do not change TLS, HTTP headers/body, model, Tools, system prompt, beta
  values, CCH, OAuth, proxy selection, retry policy, or production config.

## Expected Behavior

| Case | Expected result |
|---|---|
| `NewClaudeCodeTransportOwner()` | Non-zero opaque capability |
| Two constructor calls | Distinct capabilities |
| Same owner attached to two contexts | Same extracted capability |
| Different owners attached to contexts | Distinct extracted capabilities |
| Zero owner | Not attached / not extracted |
| Missing owner | Extraction reports absent |
| Nil context | Attachment returns a usable background-derived context |
| Child context | Inherits the owner unless explicitly replaced |

The owner token is process-local, non-serialisable, and never logged or
persisted. This step exposes no token value and does not yet alter reuse.

## Validation

- Baseline and modified focused `helps` tests.
- Baseline and modified full `internal/runtime/executor` package tests.
- `git diff --check`.
- Apply the exact patch in a separate copy, run modified owner tests, execute
  `ROLLBACK.sh`, then run the baseline owner-related test suite.
- Falsifying result: a zero owner extracts as valid, two constructors collide,
  context propagation changes across copies, or any prohibited file changes.

## Rollback

```text
bash artifacts/CPA-CLAUDECODE-T2A-OWNER-CAPABILITY-20260905/ROLLBACK.sh <test-copy> <artifact-directory>
```

## Baseline Evidence

- `go test ./internal/runtime/executor/helps -count=1`: exit `0`
- `go test ./internal/runtime/executor -count=1`: exit `0`
- `git diff --check`: exit `0`
