# Change Card: S8 Compatibility Regression and Release Manifest

## Identity

- Signal group: S6/S7 integration gate and release traceability
- Branch: `feat/s6b-404-recovery`
- Baseline commit: `586e4360` (parent of the S7 behavior commit)
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

## 2026-09-08 baseline and commit-capture correction

The workflow baseline now points to 586e4360bf655d7d6e22287aeda3cc4750f2fecf, the parent of S7, so CI exercises the S7 behavior delta instead of comparing two trees that already contain it. The preflight now emits COMMIT=<sha>; the manifest describes this external capture rather than embedding a self-referential commit hash. Runtime and wire behavior are unchanged.

## 2026-09-08 AGENTS.md guard correction

The fork's pull-request guard rejects arbitrary `AGENTS.md` changes. The
release candidate restores that file to the canonical fork-policy blob, but the
S8 diff baseline predates that restoration and therefore reports the file as a
tree difference. The source-diff guard now permits `AGENTS.md` only when its
blob exactly matches the canonical fork-policy SHA-1
`57027473d7b65be22e86a0ab2172c9a09a4e495d`; any other edit still fails the gate.
This preserves the repository policy and changes no CPA runtime or wire
behavior.
