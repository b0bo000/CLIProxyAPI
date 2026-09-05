# Change Card: S4-B.4 Diagnostics Beta Coupling

- Date: 2026-09-04
- Branch: `feat/s3-resolved-profile`
- Baseline commit: `b7769ceb964a1b8e7dc0f8c301e5f3ba08c97dc3`
- Signal group: `B-03/B-10` diagnostics request acceptance
- Changed field: final Anthropic Messages `diagnostics` Body and
  `Anthropic-Beta` coupling

## Verified cause

The earlier provider probe sent a final Body containing `diagnostics` without
`cache-diagnosis-2026-04-07`. That request returned HTTP 400, but it did not
isolate the Body field from its required beta.

The paired live matrix on 2026-09-04 held the request Body constant and changed
only that beta. Stream and Execute both returned 400 without the beta and 200
with it. Requests without diagnostics returned 200 both with and without the
beta. Official A captures place the beta after
`extended-cache-ttl-2025-04-11`.

Root cause in CPA: `claudeCodeCLIBetas` derives the beta from the Body, but the
confirmed-client branch subsequently restores the incoming beta list. A native
caller using a custom base URL has no diagnostics yet, so CPA's later managed
injection leaves the final Body/header pair inconsistent.

## Atomic implementation

Allowed source changes:

- `internal/runtime/executor/claude_executor_request.go`
- `internal/runtime/executor/claude_executor_beta_policy_test.go`
- this change card

At the final header assembly boundary, append and deduplicate
`cache-diagnosis-2026-04-07` only when all of these conditions hold:

1. the final Body contains a diagnostics object;
2. the destination is the real Anthropic upstream;
3. the endpoint is not `count_tokens`.

This applies equally to confirmed-client restoration and API-key
caller-owned passthrough. It does not rebuild the full beta list, overwrite
caller ordering, or add the beta when the Body lacks diagnostics.

## Required tests

- confirmed caller missing the beta receives it exactly once;
- existing beta remains exactly once;
- Body without diagnostics receives no beta;
- caller-owned diagnostics on Anthropic receives the beta;
- custom upstream and count_tokens remain unchanged;
- Stream and Execute produce the same invariant;
- existing diagnostics state and success-commit tests pass;
- full executor package and `git diff --check` pass;
- independent copy applies the patch, passes modified tests, rolls back, and
  passes baseline tests with a clean worktree.

## Live acceptance

Disposable capture binary SHA-256:
`35F15A195B411DA0BFBB695E6C005717AC7663B907411EBEE2A1F8847FB15276`.

The implementation-level run captured four final requests: managed Stream,
caller-owned Stream, managed Execute and caller-owned Execute. All four returned
HTTP 200, all four final Bodies contained diagnostics, all four final beta lists
contained the cache-diagnosis beta exactly once, and all four placed it directly
after extended-cache-ttl. Managed requests changed from incoming diagnostics
absent to final diagnostics present. Production CPA remained running.

Evidence:
`artifacts/CPA-CLAUDECODE-S4B4-DIAGNOSTICS-BETA-COUPLING-20260904/`.

## Explicit exclusions

This step does not change diagnostics state identity, success-commit timing,
prompt IDs, prior-request IDs, context management, model, tools, system prompt,
CCH, TLS, transport pools, OAuth refresh, custom-upstream policy or production
configuration.
