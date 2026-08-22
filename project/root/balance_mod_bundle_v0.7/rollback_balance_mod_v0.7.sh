#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.7.patch"

if [[ ! -f "$TARGET/go.mod" ]]; then
  echo "ERROR: Reasonix source tree not found: $TARGET" >&2
  exit 2
fi
cd "$TARGET"
if ! grep -q 'balance-mod-v0.7' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "Balance Mod v0.7 does not appear to be applied."
  exit 0
fi
if ! git apply --reverse --check "$PATCH"; then
  echo "ERROR: cannot cleanly roll back v0.7; tree has diverged." >&2
  exit 4
fi
git apply --reverse "$PATCH"
echo "Balance Mod v0.7 rolled back to the v0.6.1 baseline."
