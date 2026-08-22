#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.6.patch"
cd "$TARGET"
if ! git apply --reverse --check "$PATCH"; then echo "ERROR: v0.6 rollback does not apply cleanly; refusing to force." >&2; exit 1; fi
git apply --reverse "$PATCH"
echo "Balance Mod v0.6 rolled back to the v0.5 code state."
