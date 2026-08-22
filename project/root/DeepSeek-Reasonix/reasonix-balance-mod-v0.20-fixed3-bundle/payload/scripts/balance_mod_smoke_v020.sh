#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo '[v0.20 FINAL] ONLINE-only real-provider gate'
echo '  Offline targeted/quick/preflight were already passed before this final gate.'
echo '  This wrapper does NOT rerun v0.18/v0.19 offline suites.'
./scripts/balance_mod_v020_real_gate.sh
echo 'BALANCE_MOD_V20_SMOKE_PASS'
