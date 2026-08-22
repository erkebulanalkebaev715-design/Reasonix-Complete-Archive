#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export PYTHONPATH="${PYTHONPATH:-}:$(pwd)"
python -m unittest discover -s tests -v
python -m reasonix_model.native_validate_v6
./native/bench-native-model results/native_fixture/smoke_v06_int8.rxm6 500 > results/host_v06_int8.txt
./native/bench-native-model results/native_fixture/smoke_v06_mixed.rxm6 500 > results/host_v06_mixed.txt
printf 'REASONIX_V06_HOST_PASS\n'
