#!/usr/bin/env bash
set -euo pipefail

repo_root="${1:-$(git rev-parse --show-toplevel)}"
test_file="$repo_root/internal/runtime/executor/claude_executor_prompt_state_integration_test.go"

rm -f -- "$test_file"
test ! -e "$test_file"
printf 'ROLLBACK_OK test_file_absent=%s\n' "$test_file"
