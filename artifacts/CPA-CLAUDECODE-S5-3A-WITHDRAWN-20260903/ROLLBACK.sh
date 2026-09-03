#!/usr/bin/env sh
set -eu

# This archive is documentation-only. Verify that the active source scope
# remains at the S5-2 baseline; do not reapply the withdrawn implementation.
git diff --quiet 704f980a -- internal cmd sdk config go.mod go.sum .github
test ! -e artifacts/CPA-CLAUDECODE-S5-3A-PROMPT-BOUNDARY-CONTRACT-20260903
test ! -e sdk/cliproxy/executor/prompt_boundary.go
printf 'WITHDRAWN_ARCHIVE_OK=1\n'
