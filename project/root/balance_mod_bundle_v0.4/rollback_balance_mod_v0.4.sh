#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.4.patch"
cd "$TARGET"
git apply -R --check "$PATCH"
git apply -R "$PATCH"
echo "Balance Mod v0.4 rolled back to v0.3 source state."
