# S5-2.3 Caller-Provided Prompt ID Preservation

## Result

Branch `feat/s3-resolved-profile` preserves a caller-provided UUID-shaped
`cc_prompt_id` through the full non-stream and stream Claude executor request
pipelines at the synthetic RoundTripper observation boundary.

The new integration tests prove all of the following:

1. The prompt ID remains in `system.0.text`, with the exact caller value and
   exactly one key occurrence.
2. The first and second non-stream requests preserve it.
3. The first and second stream requests preserve it.
4. The second request adds the managed `cc_prev_req` after `cch` and before
   `cc_prompt_id` without changing the prompt value.
5. Diagnostics injection runs and advances its previous-message link without
   changing the prompt value.
6. A matching payload override runs and adds its marker without changing the
   prompt value.
7. CCH is finalized to one five-character lowercase value, and re-signing the
   final body is byte-idempotent.
8. The unrelated system block and user message remain unchanged.
9. A later request that omits `cc_prompt_id` does not generate or inherit one.
10. Malformed and duplicate caller fields fail before the synthetic upstream
    RoundTripper once the prior-request insertion path is active.

## Boundary

This result validates CPA behavior only when the caller supplies the field. It
does not make stock Claude Code send `cc_prompt_id` to a custom base URL, does
not generate or persist prompt state, and does not solve the real B incoming
absence. `B-12` therefore remains `CONFIRMED_DIFFERENCE`, and the release state
remains `NOT STRICTLY EQUIVALENT`.

No production `.go` file changed. The six preserved production/lower-level
source hashes in `ORIGINAL_FILE.md` still match exactly.

## Commits And CI

- Gate/card commit: `dceea60a8d686f7e73464432fa42bb8801e9e734`
- Integration-test commit: `fdc51bc1113fc7f1ad0eaa53669c2586b503a712`
- Baseline focused CI: `33839700702` (`success`)
- Baseline build CI: `33839700678` (`success`)
- Modified focused CI: `33840480312` (`success`)
- Modified build CI: `33840480322` (`success`)
- Pull request: `https://github.com/b0bo000/CLIProxyAPI/pull/1`

## Evidence Files

- `MODIFIED_FILE.md`: this result and boundary statement
- `DIFF_FILE.patch`: exact test-only diff from gate commit to test commit
- `VERIFICATION.txt`: literal commands, inputs, outputs, statuses, and hashes
- `ROLLBACK.sh`: removes only the new integration test from a selected copy
- `BASELINE-CI.log`: raw GitHub Actions baseline focused-test log
- `MODIFIED-CI.log`: raw GitHub Actions modified focused-test log
