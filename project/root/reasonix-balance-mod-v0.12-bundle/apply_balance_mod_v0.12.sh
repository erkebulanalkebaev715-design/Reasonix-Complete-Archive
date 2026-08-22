#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/balance_mod_v0.12.patch"

if [[ ! -d "$ROOT/.git" ]]; then
  echo "ERROR: Reasonix git tree not found: $ROOT" >&2
  exit 1
fi
cd "$ROOT"

if grep -q 'const balanceModVersion = "balance-mod-v0.12"' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "Balance Mod v0.12 is already applied."
  exit 0
fi
if ! grep -q 'const balanceModVersion = "balance-mod-v0.11"' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "ERROR: v0.12 expects the passed v0.11 tree. No files were changed." >&2
  exit 1
fi
if [[ ! -f internal/serve/mod_power_turn.go || ! -f internal/serve/mod_queue.go ]]; then
  echo "ERROR: v0.11 power/queue foundation is missing. No files were changed." >&2
  exit 1
fi
if ! grep -q 'BALANCE_MOD_V11_SMOKE_PASS' scripts/balance_mod_smoke.sh 2>/dev/null; then
  echo "ERROR: expected v0.11 smoke baseline is missing. No files were changed." >&2
  exit 1
fi

if ! git apply --check "$PATCH"; then
  echo "ERROR: v0.12 patch does not cleanly apply. No files were changed." >&2
  exit 1
fi
git apply "$PATCH"

echo "Balance Mod v0.12 applied."
echo "Idle-boundary automatic continuation + durable inbox handoff installed."
echo "Direct APK submit is gated only during the short transition lease; queued work stays durable."
echo "No provider/API key was read, written, or used."
echo "Quick test: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_quick.sh"
echo "Full regression: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
