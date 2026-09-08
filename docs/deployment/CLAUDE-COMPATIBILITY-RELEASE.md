# Claude Compatibility Release Manifest

## Purpose

This manifest is the release gate for the CPA/Claude Code compatibility work.
It records what the code changes control and what the evidence does not prove.

## Candidate identity

- Audit baseline: Claude Code `2.1.241`, CPA `7.2.140`.
- Development branch: `feat/s6b-404-recovery`.
- Release branch: create `release/claude-compatibility-<date>` only after the
  focused workflow succeeds.
- Candidate commit/tag: recorded at release cut; do not deploy a moving feature
  branch.

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
guard.

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
