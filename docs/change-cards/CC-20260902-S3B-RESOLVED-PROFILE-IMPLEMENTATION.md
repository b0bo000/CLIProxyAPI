# S3-B Resolved Claude Software Profile Implementation

Date: 2026-09-02
Branch: `feat/s3-resolved-profile`
Baseline: `a105f5620f6882e3f0170ad30bfd483776a396bb`
Behavior HEAD: `95553cbf8549f4cfd923b76fdc70adb8b0227aef`

## Signal Group

Software identity authority only: User-Agent/device tuple, entrypoint,
subclient, package/runtime tuple, `X-App`, and billing identity coherence.

## Owning Package And Files

The resolver lives in `internal/runtime/executor/helps/claude_software_profile.go`
with focused tests. Execute, Stream, CountTokens and cloaking call sites are
the only consumers; the exact source list is in the implementation artifact.

## Allowed Change

Resolve one request-scoped profile, preserve `detected`, `configured-cli` and
`unknown` provenance, use one authority for headers and billing, and fail
closed on a final identity conflict. Recompute CCH after body mutation.

## Prohibited Change

Do not hard-code or rewrite model, Tools, system prompts, workspace paths,
beta sets, CCH values/algorithm, session/request/prompt IDs, TLS, transport
pools, credentials or production configuration. Do not infer a physical PID
or device from HTTP-visible values.

## Controls And Falsifiers

Controls cover confirmed CLI, SDK CLI, VS Code, unknown, forged UA, configured
CLI, Execute/Stream parity and upstream CountTokens. Any header/body identity
conflict, unknown promotion, dynamic-ID contamination or non-target diff
invalidates the change. The review fix additionally covers string-form and
whitespace-padded billing text.

## Verification

Focused and full `helps`/`executor` tests, diff checks, source snapshots,
sensitive scan, reverse patch check and independent-copy rollback are recorded
under `artifacts/CPA-CLAUDECODE-S3B-RESOLVED-PROFILE-IMPLEMENTATION-20260902`.
Release remains `NOT STRICTLY EQUIVALENT`.
