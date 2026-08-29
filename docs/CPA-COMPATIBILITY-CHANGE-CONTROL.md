# CPA Compatibility Change Control

This document is the operating index for maintaining the private CLIProxyAPI fork while comparing it with official Claude Code. It prevents a source change from drifting away from the verified experiment scope or from turning a scoped observation into a claim of strict equivalence.

## Source Of Truth

The current factual baseline is:

`C:\Users\Administrator\gpt-5.6-instruct\artifacts\CPA-ClaudeCode-FIRST-READ-20260825\MODIFIED_FILE.md`

Read it before every experiment, analysis, report, or source change. It records the active version pair, A/B definitions, evidence boundaries, confounders, and the current production conclusion. The current conclusion is `NOT STRICTLY EQUIVALENT`; this is a release-state statement, not proof that every request is wrong or that a later account event was caused by CPA.

The reports are deliberately layered. A lower layer can add evidence, but it cannot silently replace a higher layer:

| Order | Artifact | Role | Override rule |
|---|---|---|---|
| 1 | `FIRST-READ/MODIFIED_FILE.md` | Verified factual baseline and boundary decisions | New verified evidence updates this first |
| 2 | `STRICT-ONE-TO-ONE-REANALYSIS/STRICT-MATRIX.json` | 63-signal machine inventory and next-evidence actions | Classifications are changed only with evidence |
| 3 | `FULL-FINGERPRINT-REANALYSIS/MODIFIED_FILE.md` | Attribution and risk interpretation across axes | Interpret; do not invent wire evidence |
| 4 | `DETAILED-EXPLANATION/MODIFIED_FILE.md` | Human-readable explanation | Explain the first three layers |
| 5 | B-gap, startup graph, raw evidence, rerun, and export reports | Scoped experiment records and attachments | Must link back to a signal and observation boundary |
| 6 | PCAPs, debug logs, source snapshots, and hashes | Primary evidence | Never paraphrase as a stronger boundary than captured |

When two reports disagree, use this sequence:

1. Check whether they refer to the same version, host, egress, account, workload, and observation boundary.
2. Prefer the primary artifact and the newest verified rerun for that exact scenario.
3. Preserve the weaker status when the boundary is not closed (`UNTESTED`, `BOUNDARY_UNPROVEN`, or `VISIBLE_CONFOUNDED`).
4. Update `FIRST-READ`, then the matrix classification/action, then derived reports. Record the old wording as superseded; do not rewrite history.

## Branch And Release Model

- `main` is a clean mirror of `upstream/main`; no compatibility work lands directly there.
- `baseline/upstream-YYYYMMDD` is immutable and identifies the source used for an experiment.
- `feat/<signal>-<scope>` contains one behavior change and its focused tests/docs.
- `integration/<date-or-scope>` combines already validated feature branches for a comparison run.
- `release/<version-or-date>` is the only branch eligible for deployment after CI and rollback evidence exist.
- `origin` is the personal fork for branches and pull requests. `upstream` is read-only.

Deployment uses a tagged or explicitly named `release/*` commit, never a moving feature branch. The deployed commit, workflow run, source hash, and rollback artifact must be recorded in the release card.

## Mandatory Change Card

Create `docs/change-cards/CC-YYYYMMDD-<signal>.md` before touching source. Use the template in `docs/change-cards/TEMPLATE.md`. A card is complete only when it contains:

- one exact Signal ID from `STRICT-MATRIX.json` (or a documented, narrowly scoped signal group);
- current classification and the evidence paths that justify it;
- the hypothesis and the smallest owning package/file set;
- fields and behavior explicitly allowed to change;
- fields explicitly prohibited from changing;
- same-scenario A/B fixture, control, falsifying observation, and confounders;
- GitHub Actions workflow/run URL and saved output paths;
- rollback command and an independently tested restored status.

The card is the review unit. A commit, pull request, and release must link to it. If the implementation discovers that a second signal must change, stop and create a second card or an integration card; do not broaden the first card silently.

## Pre-Edit Gate

Run this checklist and record the result in the card:

```text
[ ] FIRST-READ was read at the start of this turn.
[ ] Current branch is feat/*, integration/*, or release/* as appropriate; main/baseline is untouched.
[ ] Signal ID and classification are copied from STRICT-MATRIX.json.
[ ] Version, host, egress, account/cohort, workload, concurrency, and quota state are recorded.
[ ] Observation boundary is named (process, relay, packet, or provider-side; never conflated).
[ ] Allowed files and prohibited fields are listed.
[ ] A same-scenario control and a falsifying result are defined.
[ ] CI workflow and expected artifacts are named; no local Go build/test is planned.
[ ] Rollback is prepared before implementation.
```

## Compatibility Guardrails

The following are caller- and lifecycle-generated. They are not constants for a baseline capture and must not be hard-coded to make a table look similar:

- model, Tools, system prompt, workspace paths, Git state, beta sets, and body size;
- CCH, `cc_version` suffixes, device/account/session/request/prompt IDs;
- `cli`, `sdk-cli`, and `claude-vscode` entrypoints unless the real caller classification proves the value;
- socket totals, connection counts, TLS PSK/resumption counts, retry counts, and timing values.

Transport ownership, TLS session policy, and application continuity are separate cards and separate commits. A change to one axis cannot be justified by a result from another axis. Defaults remain backward-compatible and experimental behavior stays opt-in until same-scenario evidence and CI coverage support promotion.

## Evidence And Reporting

Every result records literal commands, inputs, outputs, and exit statuses. OAuth, proxy credentials, email addresses, and raw tokens stay outside the source repository and reports; use hashes, counts, or redacted identifiers. HTTP 401/429 and account expiry are client/account state unless a paired control proves a CPA behavior difference.

Before a release claim, update the baseline and matrix, link all cards and CI runs, and state what remains `UNTESTED` or `VISIBLE_CONFOUNDED`. The phrase `strictly equivalent` is reserved for a named scenario with exact same-boundary evidence; matching UA, JA3, ALPN, headers, or one successful request is insufficient.

## Current Starting Point

The current development starting point is `feat/transport-scope`. It contains maintenance rules only; no transport behavior has been changed. The next compatibility implementation must begin with a new change card and focused design/tests, then proceed through GitHub Actions. No source edit should begin merely because a report lists a difference.
