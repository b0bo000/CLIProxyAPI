# Change Card: S8 Compatibility Regression and Release Manifest

## Identity

- Signal group: S6/S7 integration gate and release traceability
- Branch: `feat/s6b-404-recovery`
- Baseline commit: `4504e4ee`
- Date: 2026-09-08
- Observation boundary: repository/CI configuration and deterministic focused tests

## Scope

S8 packages the already-scoped S6 lifecycle audits and S7 TLS policy into a
repeatable CI gate and a release manifest. It does not add a new wire mutation,
caller disguise, OAuth behavior, proxy behavior, or production deployment.

## Allowed changes

- Add a GitHub Actions workflow that runs baseline and modified focused tests,
  the S6 lifecycle/compact tests, the S7 policy tests, and a source-diff guard.
- Add a release-manifest document naming the branch, commit, version baseline,
  open evidence boundaries, configuration defaults, and rollback procedure.
- Add a local PowerShell preflight that refuses non-`release/*` deployment refs,
  dirty trees, or an omitted verification artifact.
- Keep the canonical `docs/CPA-COMPATIBILITY-CHANGE-CONTROL.md` pointer in
  the release diff allowlist and pull-request trigger paths.

## Prohibited changes

- No automatic merge of upstream version drift (`2.1.258` is not the current
  `2.1.241` audit baseline).
- No production service restart or configuration edit.
- No hard-coded model, tools, prompt, body, ID, timing, socket, or endpoint data.
- No claim that CI closes provider-side raw H1 or strict A/B equivalence.

## Acceptance

1. Workflow YAML parses as a valid GitHub Actions definition and covers the
   focused config, transport, executor, and manager tests.
2. The release manifest identifies the exact source commit and preserves the
   `NOT STRICTLY EQUIVALENT` boundary statement.
3. The preflight reports a clean release ref and required evidence paths, while
   failing safely for a feature branch or dirty worktree.
4. Independent rollback of the new files returns the repository copy exactly.

## Decision

S8 is a packaging and verification gate only. A deployment candidate still
requires a separately created `release/*` branch, CI success, and an operator
review of all remaining `UNTESTED`/`VISIBLE_CONFOUNDED` signals.

## 2026-09-08 guard correction

The first static allowlist check against baseline `4504e4ee` found the
already-committed canonical change-control index
(`docs/CPA-COMPATIBILITY-CHANGE-CONTROL.md`) outside the S8 regex. The
workflow now includes that path in both pull-request triggers and the
source-diff allowlist. This changes only release-gate coverage; no runtime or
wire behavior changes.
