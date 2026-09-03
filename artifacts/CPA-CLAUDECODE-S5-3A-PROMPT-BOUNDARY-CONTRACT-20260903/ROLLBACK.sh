#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <repository-root>" >&2
  exit 64
fi

artifact_dir="$(cd "$(dirname "$0")" && pwd)"
repo_root="$1"
patch_file="$artifact_dir/SOURCE-DIFF.patch"

git -C "$repo_root" apply --check --reverse "$patch_file"
git -C "$repo_root" apply --reverse "$patch_file"
git -C "$repo_root" diff --check

echo "ROLLBACK_OK TARGET=$repo_root"
