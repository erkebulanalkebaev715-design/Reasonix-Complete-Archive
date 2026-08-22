#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

./scripts/balance_mod_smoke_quick_v017.sh
./scripts/balance_mod_v018_targeted.sh

echo "BALANCE_MOD_V18_QUICK_PASS"
