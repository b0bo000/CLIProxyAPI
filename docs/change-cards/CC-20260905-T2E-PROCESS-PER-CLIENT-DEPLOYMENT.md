# Change Card: Process-Per-Client Deployment Contract

## Identity

- Signal: `pool/ownership` (operational process boundary)
- Scope: document and template process-per-client CPA isolation
- Branch: `feat/s3-resolved-profile`
- Baseline: `571fba46`
- Date: 2026-09-05
- Status: `IMPLEMENTED_AND_LOCALLY_VERIFIED_NO_PRODUCTION_CHANGE`

## Change

Add `docker-compose.process-per-client.yml` with two independently named CPA
instances, distinct host ports, and distinct config/auth/log/plugin mounts.
Add `docs/deployment/PROCESS-PER-CLIENT.md` describing lifecycle, routing, and
state-isolation rules. The existing default compose file is untouched.

## Invariants

- Every client instance maps to one CPA process/container.
- No host state directory is shared between instances.
- Each downstream session remains pinned to one instance.
- No source-level owner inference is introduced.
- No credentials, management secrets, production endpoints, or OAuth material
  are written into the template.

## Verification

- Static template checks confirm two services, unique ports, unique container
  names, and distinct state roots.
- `git diff --check` passes.
- Existing focused Go tests remain green; this step does not change Go code.
- A separate artifact copy is restored with `ROLLBACK.sh`; the original
  modified artifact remains changed.

## Evidence

`artifacts/CPA-CLAUDECODE-T2E-PROCESS-PER-CLIENT-20260905/`

## 2026-09-08 verification update

Static compose/contract assertions passed with two services, two distinct host ports, separate client-a/client-b state roots, and the documented session pinning/restart rules. Independent rollback-copy equality passed. The verified artifact is rtifacts/CPA-CLAUDECODE-T2E-PROCESS-PER-CLIENT-20260908/. No production deployment occurred.

