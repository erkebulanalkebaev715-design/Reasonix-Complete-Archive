#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.."&&pwd)";cd "$ROOT"
./scripts/balance_mod_smoke_v019.sh
./scripts/balance_mod_v020_targeted.sh
echo BALANCE_MOD_V20_PREFLIGHT_PASS
echo "NOTE: not FULL PASS until explicitly-approved real API gate succeeds."
