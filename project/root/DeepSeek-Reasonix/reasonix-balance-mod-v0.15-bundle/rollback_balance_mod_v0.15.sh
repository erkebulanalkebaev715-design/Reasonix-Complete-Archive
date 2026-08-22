#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/balance_mod_v0.15.patch"
cd "$TARGET"
if ! git apply -R --check "$PATCH"; then
  echo "ERROR: v0.15 rollback does not cleanly apply. No files were changed." >&2
  exit 1
fi
git apply -R "$PATCH"
echo "Balance Mod v0.15 rolled back to v0.14.3-compatible tree."
