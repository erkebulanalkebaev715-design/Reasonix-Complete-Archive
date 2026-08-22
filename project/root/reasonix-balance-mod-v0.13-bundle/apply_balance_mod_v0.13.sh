#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/balance_mod_v0.13.patch"

if [[ ! -d "$ROOT/.git" ]]; then
  echo "ERROR: Reasonix git tree not found: $ROOT" >&2
  exit 1
fi
cd "$ROOT"

if grep -q 'const balanceModVersion = "balance-mod-v0.13"' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "Balance Mod v0.13 is already applied."
  exit 0
fi
if ! grep -q 'const balanceModVersion = "balance-mod-v0.12"' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "ERROR: v0.13 expects the passed v0.12 tree. No files were changed." >&2
  exit 1
fi
if ! grep -q 'BALANCE_MOD_V12_SMOKE_PASS' scripts/balance_mod_smoke.sh 2>/dev/null; then
  echo "ERROR: passed v0.12 smoke baseline not detected. No files were changed." >&2
  exit 1
fi
if [[ ! -f internal/serve/mod_orchestrator.go || ! -f internal/serve/mod_power_turn.go ]]; then
  echo "ERROR: v0.12 orchestrator/power-turn foundation is missing." >&2
  exit 1
fi

if ! git apply --check "$PATCH"; then
  echo "ERROR: v0.13 patch does not cleanly apply. No files were changed." >&2
  exit 1
fi
git apply "$PATCH"

echo "Balance Mod v0.13 applied."
echo "Added durable sanitized pending-route recovery + stable inbox idempotency + native read-only Pro diagnosis guard."
echo "No provider/API key was read, written, or used."
echo "Quick: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_quick.sh"
echo "Full:  PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
