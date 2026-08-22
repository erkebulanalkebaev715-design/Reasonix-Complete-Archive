#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/balance_mod_v0.15.patch"

[[ -d "$TARGET" ]] || { echo "ERROR: Reasonix directory not found: $TARGET" >&2; exit 1; }
[[ -f "$PATCH" ]] || { echo "ERROR: patch missing: $PATCH" >&2; exit 1; }
cd "$TARGET"

if grep -q 'const balanceModVersion = "balance-mod-v0.15"' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "Balance Mod v0.15 already applied."
  exit 0
fi

if ! git apply --check "$PATCH"; then
  echo "ERROR: v0.15 patch does not cleanly apply. No files were changed." >&2
  echo "Expected base: the v0.14.3 tree that passed BALANCE_MOD_V14_SMOKE_PASS." >&2
  exit 1
fi

git apply "$PATCH"
chmod +x scripts/balance_mod_offline_stress.sh scripts/balance_mod_smoke.sh scripts/balance_mod_smoke_quick.sh

echo "Balance Mod v0.15 applied."
echo "Added native deny-bypass stress + crash/restart/queue/corrupt-state offline gate."
echo "No real provider/API key is required or used by this version."
echo "Quick: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_quick.sh"
echo "Full:  PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
