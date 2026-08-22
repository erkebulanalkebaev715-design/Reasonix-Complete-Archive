#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.4.patch"

if [[ ! -d "$TARGET" || ! -f "$TARGET/go.mod" ]]; then
  echo "ERROR: Reasonix source tree not found: $TARGET" >&2
  exit 1
fi
if ! grep -q 'balance-mod-v0.3' "$TARGET/internal/serve/mod_bridge.go" 2>/dev/null; then
  if grep -q 'balance-mod-v0.4' "$TARGET/internal/serve/mod_bridge.go" 2>/dev/null; then
    echo "Balance Mod v0.4 already appears to be applied."
    exit 0
  fi
  echo "ERROR: Balance Mod v0.3 baseline not detected. Apply/pass v0.3 first." >&2
  exit 1
fi
if ! grep -q 'BALANCE_MOD_V03_SMOKE_PASS' "$TARGET/scripts/balance_mod_smoke.sh" 2>/dev/null; then
  echo "ERROR: v0.3 smoke-test baseline not detected." >&2
  exit 1
fi

cd "$TARGET"
if ! git apply --check "$PATCH"; then
  echo "ERROR: v0.4 patch does not apply cleanly. Do not force it; send the error output." >&2
  exit 1
fi

git apply "$PATCH"
echo "Balance Mod v0.4 applied."
echo "Next: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
