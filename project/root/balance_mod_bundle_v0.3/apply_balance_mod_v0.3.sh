#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.3.patch"

if [[ ! -d "$TARGET" ]]; then
  echo "ERROR: Reasonix directory not found: $TARGET" >&2
  exit 1
fi
if [[ ! -f "$TARGET/go.mod" ]]; then
  echo "ERROR: $TARGET does not look like the Reasonix source tree" >&2
  exit 1
fi
if [[ ! -f "$TARGET/scripts/balance_mod_smoke.sh" ]] || ! grep -q 'BALANCE_MOD_V02_SMOKE_PASS' "$TARGET/scripts/balance_mod_smoke.sh"; then
  echo "ERROR: Balance Mod v0.2 baseline not detected. Apply/pass v0.2 first." >&2
  exit 1
fi
if grep -q 'balance-mod-v0.3' "$TARGET/internal/serve/mod_bridge.go" 2>/dev/null; then
  echo "Balance Mod v0.3 already appears to be applied."
  exit 0
fi

cd "$TARGET"
if ! git apply --check "$PATCH"; then
  echo "ERROR: v0.3 patch does not apply cleanly. Do not force it; send the error output." >&2
  exit 1
fi

git apply "$PATCH"
echo "Balance Mod v0.3 applied."
echo "Next: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
