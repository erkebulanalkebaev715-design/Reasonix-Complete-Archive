#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
CXX="${CXX:-g++}"; FLAGS="-O3 -std=c++17 -Wall -Wextra"
"$CXX" $FLAGS reasonix_matvec.cpp test_matvec.cpp -o test_matvec
"$CXX" $FLAGS reasonix_matvec.cpp reasonix_native_model.cpp reasonix_native_cli.cpp -o reasonix-native
"$CXX" $FLAGS reasonix_matvec.cpp reasonix_native_model.cpp bench_native_model.cpp -o bench-native-model
./test_matvec
