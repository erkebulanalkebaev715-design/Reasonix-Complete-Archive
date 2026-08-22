#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.9.patch"

cd "$TARGET"
if ! grep -q 'balance-mod-v0.9' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "ERROR: Balance Mod v0.9 does not appear to be applied." >&2
  exit 3
fi
if ! git apply -R --check "$PATCH"; then
  echo "ERROR: v0.9 rollback is not clean; no files were changed." >&2
  exit 4
fi
git apply -R "$PATCH"
echo "Balance Mod v0.9 rolled back to the prior v0.8.x tree."
