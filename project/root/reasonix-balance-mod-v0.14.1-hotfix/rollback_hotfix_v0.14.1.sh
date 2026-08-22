#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
PATCH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/balance_mod_v0.14.1_hotfix.patch"
cd "$TARGET"
if ! git apply --reverse --check "$PATCH"; then
  echo "ERROR: v0.14.1 hotfix is not cleanly reversible from this tree." >&2
  exit 1
fi
git apply --reverse "$PATCH"
echo "Balance Mod v0.14.1 hotfix rolled back."
