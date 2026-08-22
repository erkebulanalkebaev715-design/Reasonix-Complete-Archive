#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

./scripts/balance_mod_smoke.sh
./scripts/balance_mod_v017_targeted.sh

echo "BALANCE_MOD_V17_SMOKE_PASS"
