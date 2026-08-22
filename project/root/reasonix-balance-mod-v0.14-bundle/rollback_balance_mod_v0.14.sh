#!/usr/bin/env bash
set -euo pipefail
ROOT="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/balance_mod_v0.14.patch"
cd "$ROOT"
if ! git apply --reverse --check "$PATCH"; then
  echo "ERROR: v0.14 rollback does not cleanly apply. No files were changed." >&2
  exit 1
fi
git apply --reverse "$PATCH"
echo "Balance Mod v0.14 rolled back to v0.13."
