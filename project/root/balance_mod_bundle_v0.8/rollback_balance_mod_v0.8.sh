#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.8.patch"
cd "$TARGET"
if grep -q 'balance-mod-v0.7' internal/serve/mod_bridge.go 2>/dev/null && ! grep -q 'balance-mod-v0.8' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "Balance Mod v0.8 is not applied."
  exit 0
fi
if ! git apply -R --check "$PATCH"; then
  echo "ERROR: v0.8 rollback does not cleanly apply; refusing partial rollback." >&2
  exit 4
fi
git apply -R "$PATCH"
echo "Balance Mod v0.8 rolled back to v0.7 files."
