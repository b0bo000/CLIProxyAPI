#!/usr/bin/env bash
set -eu

TARGET="${1:?independent copy path required}"
BASELINE="${2:-$(dirname "$0")/ORIGINAL_FILE.md}"
MODIFIED="${3:-$(dirname "$0")/MODIFIED_FILE.md}"

test -f "$BASELINE"
test -f "$MODIFIED"
mkdir -p "$(dirname "$TARGET")"
cp "$MODIFIED" "$TARGET"
before="$(sha256sum "$TARGET" | awk '{print $1}')"
cp "$BASELINE" "$TARGET"
after="$(sha256sum "$TARGET" | awk '{print $1}')"
expected="$(sha256sum "$BASELINE" | awk '{print $1}')"
test "$after" = "$expected"
test "$before" != "$after"
printf 'ROLLBACK_COPY_CHANGED=1\n'
printf 'RESTORED_HASH_MATCH=1\n'
printf 'RESTORED_SHA256=%s\n' "$after"
