#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo '[v0.20 FINAL 1/2] online-only real-provider gate'
echo '  prerequisite: BALANCE_MOD_V20_PREFLIGHT_PASS was already obtained; offline suites are NOT repeated here.'
./scripts/balance_mod_v020_real_gate.sh

echo '[v0.20 FINAL 2/2] release gate complete'
echo 'BALANCE_MOD_V20_SMOKE_PASS'
