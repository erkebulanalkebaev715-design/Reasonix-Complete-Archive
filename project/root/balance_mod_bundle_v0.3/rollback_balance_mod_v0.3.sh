#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.3.patch"
cd "$TARGET"
if git apply -R --check "$PATCH"; then
  git apply -R "$PATCH"
  echo "Balance Mod v0.3 rolled back to the v0.2 file state."
else
  echo "ERROR: clean rollback is not possible (files changed after v0.3). Do not force." >&2
  exit 1
fi
