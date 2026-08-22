#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# Full historical regression first. If any older behavior regresses, the RC is dead.
./scripts/balance_mod_smoke_v017.sh

# Then the integrated release-candidate boundary.
./scripts/balance_mod_v018_rc.sh

echo "BALANCE_MOD_V18_SMOKE_PASS"
