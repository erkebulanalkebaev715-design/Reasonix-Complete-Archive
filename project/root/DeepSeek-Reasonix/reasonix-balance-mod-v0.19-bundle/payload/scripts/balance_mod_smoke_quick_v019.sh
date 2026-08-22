#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

./scripts/balance_mod_smoke_quick_v018.sh
./scripts/balance_mod_v019_targeted.sh

echo "BALANCE_MOD_V19_QUICK_PASS"
