#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# v0.18 is the verified full offline RC baseline. Re-run it first so v0.19
# cannot earn PASS by breaking any older Balance/Reasonix behavior.
./scripts/balance_mod_smoke_v018.sh

# Then prove the authenticated Android/APK backend boundary.
./scripts/balance_mod_v019_targeted.sh

echo "BALANCE_MOD_V19_SMOKE_PASS"
