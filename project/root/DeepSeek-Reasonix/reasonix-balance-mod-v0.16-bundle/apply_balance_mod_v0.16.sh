#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/balance_mod_v0.16.patch"

git -C "$TARGET" rev-parse --is-inside-work-tree >/dev/null 2>&1 || { echo "ERROR: Reasonix git tree not found: $TARGET" >&2; exit 1; }
[[ -f "$PATCH" ]] || { echo "ERROR: patch missing: $PATCH" >&2; exit 1; }

if grep -q 'balance-mod-v0.16' "$TARGET/internal/serve/mod_bridge.go" 2>/dev/null; then
  echo "Balance Mod v0.16 already appears to be installed."
  exit 0
fi

if git -C "$TARGET" apply --check "$PATCH"; then
  git -C "$TARGET" apply "$PATCH"
else
  echo "ERROR: v0.16 patch does not cleanly apply. No files were changed." >&2
  echo "Expected base: the v0.15 tree that passed BALANCE_MOD_V15_SMOKE_PASS." >&2
  exit 1
fi

echo "Balance Mod v0.16 applied."
echo "Added hard pre-call provider budget admission, retry reserve, remaining-budget carryover across model rebuilds, and hard-budget side-call fail-closed policy."
echo "No provider/API key was read, written, or used by the patch."
echo "Quick: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_quick.sh"
echo "Full:  PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
