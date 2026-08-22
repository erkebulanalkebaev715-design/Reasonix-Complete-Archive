#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.8.1.patch"
cd "$TARGET"
if ! git apply --reverse --check "$PATCH"; then
  echo "ERROR: v0.8.1 rollback check failed; no files were changed." >&2
  exit 4
fi
git apply --reverse "$PATCH"
echo "Balance Mod v0.8.1 rolled back to v0.7."
