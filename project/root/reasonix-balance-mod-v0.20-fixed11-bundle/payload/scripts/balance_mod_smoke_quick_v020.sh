#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.."&&pwd)";cd "$ROOT"
./scripts/balance_mod_smoke_quick_v019.sh
./scripts/balance_mod_v020_targeted.sh
echo BALANCE_MOD_V20_QUICK_PASS
