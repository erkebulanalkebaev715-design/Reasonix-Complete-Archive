#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
CXX="${CXX:-clang++}"
if ! command -v "$CXX" >/dev/null 2>&1; then echo "clang++ missing" >&2; exit 2; fi
ARCH="-march=armv8-a"
if grep -m1 -E '^Features' /proc/cpuinfo 2>/dev/null | grep -qw asimddp; then ARCH="-march=armv8.2-a+dotprod"; fi
FLAGS="-O3 -std=c++17 -Wall -Wextra $ARCH"
echo "compile_arch=$ARCH"
"$CXX" $FLAGS reasonix_matvec.cpp test_matvec.cpp -o test_matvec_android
"$CXX" $FLAGS reasonix_matvec.cpp reasonix_native_model.cpp reasonix_native_cli.cpp -o reasonix-native-android
"$CXX" $FLAGS reasonix_matvec.cpp reasonix_native_model.cpp bench_native_model.cpp -o bench-native-model-android
./test_matvec_android
