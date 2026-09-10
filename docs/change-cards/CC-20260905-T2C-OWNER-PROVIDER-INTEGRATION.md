# Change Card: Claude Transport Owner Provider Integration

## Identity

- Signal ID: `pool/ownership` (trusted owner-provider integration subset)
- Scope: install and propagate a trusted process-owner context provider
- Branch: `feat/s3-resolved-profile`
- Baseline commit: `bd4cb753`
- Card owner: CPA Claude compatibility workstream
- Date: 2026-09-05

## Current Evidence

- Current classification: `VISIBLE_CONFOUNDED`
- Observation boundary: CPA source and request-context propagation; no provider-side observation
- Evidence paths:
  - `artifacts/CPA-CLAUDECODE-DOCS-20260829/00-FIRST-READ.md`, sections 56 and 93
  - `artifacts/CPA-CLAUDECODE-S2B-INTEGRATION-BOUNDARY-AUDIT-20260901/MODIFIED_FILE.md`
  - `artifacts/CPA-CLAUDECODE-T2B-OWNER-CACHE-INTEGRATION-20260905/VERIFICATION.txt`
- Version / host / egress / account-cohort / workload: source-only step; no OAuth or provider request
- Known confounders: no trusted downstream process identity exists in the default HTTP server; A/B host and egress differ

## Hypothesis And Scope

- Hypothesis: a trusted embedding host can attach one stable opaque owner to every request from one downstream instance, allowing T2-B owner-scoped transport reuse through retries and all Claude execution entrypoints.
- Allowed files:
  - `sdk/cliproxy/auth/conductor.go`
  - `sdk/cliproxy/auth/conductor_execution.go`
  - `sdk/cliproxy/auth/conductor_selection.go`
  - `sdk/cliproxy/builder.go`
  - focused auth/builder tests
  - this change card and verification artifacts
- Allowed behavior: an explicitly installed provider may bind a context; the bound context is reused by Manager retry and execution paths.
- Prohibited files/behavior: no default provider installation, no owner creation in a singleton executor, no derivation from session IDs, credentials, headers, User-Agent, IP, PID, process name, inbound connection, body, or arbitrary strings; no TLS, HTTP, body, ID, OAuth, proxy, retry-policy, or production configuration changes.
- Smallest owning scope: the Manager is the existing common execution boundary and Builder is the existing trusted embedding boundary; no executor or HTTP handler rewrite is needed.

## Verification Design

- Same-scenario A fixture: Manager with no provider; context and execution behavior remain unchanged.
- Same-scenario B fixture: Manager with a test provider that returns one stable bound context; Execute, ExecuteStream, ExecuteCount, HttpRequest and retry-derived contexts retain the provider result.
- Control: provider returns the input context unchanged; no owner is attached.
- Falsifying observation: a default Manager invokes a provider, a missing provider creates an owner, a retry loses the bound context, or a non-Claude path changes behavior.
- Expected true result: explicit provider is called at the public Manager boundary, its stable context survives retries, and ownerless callers remain unchanged.
- Expected false result: any implicit owner, per-request owner creation, context loss, or behavior change without installation.
- GitHub Actions workflow: `.github/workflows/pr-test-build.yml` (`go test ./...` and build matrix)
- Expected artifact paths: `artifacts/CPA-CLAUDECODE-T2C-OWNER-PROVIDER-INTEGRATION-20260905/`

## Implementation Record

- Commit(s): `5949f9da` (`feat(claude): add trusted transport owner provider boundary`)
- Files changed: `sdk/cliproxy/auth/conductor.go`, `sdk/cliproxy/auth/conductor_execution.go`, `sdk/cliproxy/auth/conductor_selection.go`, `sdk/cliproxy/auth/conductor_owner_provider_test.go`, `sdk/cliproxy/builder.go`
- Default behavior preserved: yes; provider is installed only when explicitly non-nil, so a custom core manager is not cleared by Builder defaults
- Experimental flag/config: none; installation is an embedding API, not a production default
- Secrets scan result: 0 matches

## Rollback

- Rollback command: `bash artifacts/CPA-CLAUDECODE-T2C-OWNER-PROVIDER-INTEGRATION-20260905/ROLLBACK.sh <test-copy> <artifact-directory>`
- Separate-copy rollback test: `cpa-t2c-rollback-test2`, exit 0
- Restored hash/status: baseline `bd4cb753`, clean detached worktree

## Review Decision

- CI run URL / run ID: final branch `ab5188d7`: `pr-test-build` 33965822140 (success), `claude-prompt-state-test` 33965822035 (success), `translator-path-guard` 33965822011 (success); `agents-md-guard` 33965822350 intentionally rejected PR #4 because the feature history includes an `AGENTS.md` difference
- Evidence classification after change: remains `VISIBLE_CONFOUNDED` until a trusted host actually installs the provider
- `FIRST-READ` updated: section 94
- `STRICT-MATRIX` updated: pending
- Release eligible: no
- Reviewer: pending
