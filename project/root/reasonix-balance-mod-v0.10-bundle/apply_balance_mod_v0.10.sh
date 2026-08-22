#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.10.patch"

if [[ ! -f "$TARGET/go.mod" || ! -d "$TARGET/internal/serve" ]]; then
  echo "ERROR: Reasonix source tree not found: $TARGET" >&2
  exit 2
fi
cd "$TARGET"

if grep -q 'balance-mod-v0.10' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "Balance Mod v0.10 already appears to be applied."
  exit 0
fi
if ! grep -q 'balance-mod-v0.9' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "ERROR: expected the tested Balance Mod v0.9 baseline." >&2
  exit 3
fi
if ! grep -q 'BALANCE_MOD_V09_SMOKE_PASS' scripts/balance_mod_smoke.sh 2>/dev/null; then
  echo "ERROR: v0.9 smoke baseline not detected. Run/fix v0.9 first." >&2
  exit 3
fi

if ! git apply --check "$PATCH"; then
  echo "ERROR: v0.10 patch does not cleanly apply. No files were changed." >&2
  exit 4
fi

git apply "$PATCH"
echo "Balance Mod v0.10 applied."
echo "Native durable task queue + reviewed recovery + per-turn budgets added; no provider/API key was read or used."
echo "Next: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
