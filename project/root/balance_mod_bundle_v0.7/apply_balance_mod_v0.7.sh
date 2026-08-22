#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.7.patch"

if [[ ! -f "$TARGET/go.mod" || ! -d "$TARGET/internal/serve" ]]; then
  echo "ERROR: Reasonix source tree not found: $TARGET" >&2
  exit 2
fi
if [[ ! -f "$PATCH" ]]; then
  echo "ERROR: patch not found: $PATCH" >&2
  exit 2
fi

cd "$TARGET"
if grep -q 'balance-mod-v0.7' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "Balance Mod v0.7 is already applied."
  exit 0
fi

# v0.7 is intentionally incremental. Refuse a guessed/older tree instead of
# partially applying APK/control changes to the wrong Reasonix state.
if ! grep -q 'balance-mod-v0.6' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "ERROR: expected Balance Mod v0.6 baseline (v0.6.1 hotfix level)." >&2
  exit 3
fi
if ! grep -q 'return r.isProviderVisibleLocked' internal/tool/tool.go 2>/dev/null; then
  echo "ERROR: v0.6.1 provider-visibility hotfix is missing." >&2
  exit 3
fi
if ! grep -q 'BALANCE_MOD_V06_SMOKE_PASS' scripts/balance_mod_smoke.sh 2>/dev/null; then
  echo "ERROR: expected the v0.6 smoke baseline before v0.7." >&2
  exit 3
fi

if ! git apply --check "$PATCH"; then
  echo "ERROR: v0.7 patch does not cleanly apply. No files were changed." >&2
  exit 4
fi

git apply "$PATCH"
echo "Balance Mod v0.7 applied."
echo "No provider/API key was read, written, or used."
echo "Next: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
