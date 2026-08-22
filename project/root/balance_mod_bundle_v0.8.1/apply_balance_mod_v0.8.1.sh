#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.8.1.patch"

if [[ ! -f "$TARGET/go.mod" || ! -d "$TARGET/internal/serve" ]]; then
  echo "ERROR: Reasonix source tree not found: $TARGET" >&2
  exit 2
fi
cd "$TARGET"

if grep -q 'balance-mod-v0.8' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "Balance Mod v0.8 already appears to be applied."
  exit 0
fi
if ! grep -q 'balance-mod-v0.7' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "ERROR: expected Balance Mod v0.7 baseline." >&2
  exit 3
fi
if ! grep -q 'BALANCE_MOD_V07_SMOKE_PASS' scripts/balance_mod_smoke.sh 2>/dev/null; then
  echo "ERROR: expected the v0.7 smoke baseline before v0.8." >&2
  exit 3
fi
# v0.7 includes the reasoning-policy field; v0.8.1 is rebased against that exact tested tree.
if ! grep -q 'hidden-chain-never-exported' internal/serve/mod_capabilities.go 2>/dev/null; then
  echo "ERROR: exact tested v0.7 APK protocol baseline not detected." >&2
  exit 3
fi

if ! git apply --check "$PATCH"; then
  echo "ERROR: corrected v0.8.1 patch still does not cleanly apply. No files were changed." >&2
  exit 4
fi

git apply "$PATCH"
echo "Balance Mod v0.8.1 (corrected v0.8 patch) applied."
echo "No provider/API key was read, written, or used."
echo "Next: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
