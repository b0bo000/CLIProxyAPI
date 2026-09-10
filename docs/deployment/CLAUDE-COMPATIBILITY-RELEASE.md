# Claude Compatibility Release Manifest

## Purpose

This manifest is the release gate for the CPA/Claude Code compatibility work.
It records what the code changes control and what the evidence does not prove.

## Candidate identity

- Audit baseline: Claude Code `2.1.241`, CPA `7.2.156`.
- Development branch: `feat/release-gate-v7.2.156-20260910`.
- Release branch: `release/claude-compatibility-v7.2.156-20260910` (local candidate; remote push and GitHub Actions are pending).
- Candidate commit/tag: the local guard-correction candidate is
  `2ec017db0fd0505a656846c83d369462b6b74fe0`; the preflight still emits
  `COMMIT=<sha>` for the exact release ref and stores it with release evidence.

## Included validated scopes

- S6-A same-process multi-prompt success capture.
- S6-B 404 classification/recovery audit.
- S6-C cancellation/recovery audit.
- S6-D compact/count_tokens success and local 2xx-only controlled-fault recovery.
- S7 opt-in Claude inference TLS session-resumption policy. Omitted/true keeps
  the existing cache; false creates a policy-separated non-resuming transport.

## Configuration

```yaml
claude-code:
  # Omit for the existing enabled behavior.
  tls-session-resumption: true
```

Set `false` only for a controlled comparison. The setting does not rewrite
headers, body fields, IDs, model/tools, OAuth, proxy, or control-plane TLS.

## Required CI gate

Run `.github/workflows/claude-compatibility-regression.yml` and retain its
artifact logs. The workflow runs baseline and modified focused tests, S6
compact/count_tokens tests, S7 policy tests, and an allowed-path source diff
guard. The local preflight emits the exact candidate commit; the evidence
records the latest result. No remote workflow run is claimed until a GitHub
Actions run ID is available.

The S8 source-diff guard permits the historical `AGENTS.md` tree delta only
when the file's blob matches the canonical fork-policy SHA-1
`57027473d7b65be22e86a0ab2172c9a09a4e495d`; arbitrary policy-file edits still
fail the gate. This is repository-guard plumbing and does not change CPA
runtime or wire behavior.

## Open evidence boundaries

The release must continue to state `NOT STRICTLY EQUIVALENT`. Remaining open
boundaries include provider-side raw H1 (client-unobservable), strict same-host /
same-egress causal TLS A/B, two-real-account transport isolation, full
OAuth/device lifecycle, caller matrix, and timing cadence. Controlled local
faults are not Anthropic-native failures.

## Rollback

1. Stop using the release candidate and deploy the previously recorded release
   tag.
2. For the S7 switch, remove `claude-code.tls-session-resumption` or set it to
   `true` to restore the existing enabled policy.
3. Retain the change-card artifact and its independently tested `ROLLBACK.sh`.

Preflight command:

```powershell
./scripts/verify-claude-compatibility-release.ps1
```

## v7.2.156 release-gate update

- Upstream baseline: `d1a024e9400bc65bd78ccd908945cf2eacc2835e` (`v7.2.156`).
- Release candidate: `release/claude-compatibility-v7.2.156-20260910` at `2ec017db0fd0505a656846c83d369462b6b74fe0`.
- The compatibility workflow is pinned to the upstream tag commit and reads its allowlist from `docs/deployment/CLAUDE-COMPATIBILITY-RELEASE-PATHS.txt`.
- Experiment evidence remains outside the source repository under the local artifacts tree.
