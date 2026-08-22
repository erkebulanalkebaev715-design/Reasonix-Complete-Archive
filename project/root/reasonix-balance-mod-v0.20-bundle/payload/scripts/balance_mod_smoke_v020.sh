#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.."&&pwd)";cd "$ROOT"
./scripts/balance_mod_smoke_v019.sh
./scripts/balance_mod_v020_targeted.sh
./scripts/balance_mod_v020_real_gate.sh
echo BALANCE_MOD_V20_SMOKE_PASS
