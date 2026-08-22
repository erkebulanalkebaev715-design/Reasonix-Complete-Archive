#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

: "${GOTOOLCHAIN:=local}"
export GOTOOLCHAIN

for f in \
  scripts/balance_mod_offline_prototype.sh \
  scripts/balance_mod_offline_stress.sh \
  scripts/balance_mod_precall_budget.sh \
  scripts/balance_mod_v017_targeted.sh; do
  [[ -x "$f" ]] || { echo "BALANCE_V18_RC_FAIL: required gate missing/not executable: $f" >&2; exit 1; }
done

echo "[v0.18 RC 1/5] canonical single-instance RC flow"
./scripts/balance_mod_v018_targeted.sh

echo "[v0.18 RC 2/5] frozen v0.14 full localhost APK prototype"
./scripts/balance_mod_offline_prototype.sh

echo "[v0.18 RC 3/5] v0.15 native offline stress gate"
./scripts/balance_mod_offline_stress.sh

echo "[v0.18 RC 4/5] v0.16 real pre-call hard-budget process gate"
./scripts/balance_mod_precall_budget.sh

echo "[v0.18 RC 5/5] v0.17 durable crash/replay gate"
./scripts/balance_mod_v017_targeted.sh

echo "BALANCE_MOD_V18_RC_PASS"
