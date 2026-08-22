#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# Android shared storage is commonly noexec. If the archive was extracted in Downloads,
# copy it into Termux private HOME and continue there automatically.
case "$ROOT" in
  /storage/*|"$HOME"/storage/*)
    DEST="$HOME/reasonix_v06_phone_only"
    rm -rf "$DEST"
    mkdir -p "$DEST"
    cp -a "$ROOT"/. "$DEST"/
    exec bash "$DEST/scripts/phone_one_tap.sh"
    ;;
esac
cd "$ROOT"

say(){ printf '%s\n' "$*"; }
kv(){ printf '%s=%s\n' "$1" "$2"; }

say 'REASONIX_V06_PHONE_ONLY_BEGIN'
kv model "$(getprop ro.product.model 2>/dev/null || true)"
kv hardware "$(getprop ro.hardware 2>/dev/null || true)"
kv abi "$(getprop ro.product.cpu.abi 2>/dev/null || true)"
kv arch "$(uname -m)"

if [ "$(uname -m)" != "aarch64" ]; then
  say 'ERROR: this build targets 64-bit ARM Android (aarch64).'; exit 2
fi
if ! command -v awk >/dev/null 2>&1 && command -v pkg >/dev/null 2>&1; then pkg install -y gawk; fi
if ! command -v clang++ >/dev/null 2>&1; then
  if command -v pkg >/dev/null 2>&1; then
    say 'Installing clang in Termux...'; pkg install -y clang
  else
    say 'ERROR: clang++ missing and pkg is unavailable.'; exit 3
  fi
fi

say '--- MEMORY BEFORE ---'
grep -E 'MemTotal|MemAvailable|SwapTotal|SwapFree' /proc/meminfo || true
say '--- CPU FEATURES ---'
grep -m1 -E '^Features' /proc/cpuinfo || true
say '--- THERMAL BEFORE ---'
for f in /sys/class/thermal/thermal_zone*/temp; do [ -r "$f" ] && printf '%s=' "$f" && cat "$f"; done 2>/dev/null || true

say '--- BUILD NATIVE ENGINE ---'
./native/build_termux.sh
CLI=./native/reasonix-native-android
BENCH=./native/bench-native-model-android
PROMPT='82,101,97,115,111,110,105,120'
EXPECTED='120,120,120,120,120,120,120,120'

for policy in int8 mixed; do
  MODEL="results/native_fixture/smoke_v06_${policy}.rxm6"
  [ -f "$MODEL" ] || { say "ERROR: missing $MODEL"; exit 4; }
  say "--- CORRECTNESS $policy ---"
  for mode in fast standard deep; do
    line="$($CLI "$MODEL" "$PROMPT" 8 "$mode" | head -1)"
    kv "${policy}_${mode}" "$line"
    [ "$line" = "GREEDY=$EXPECTED" ] || { say "ERROR: correctness gate failed for $policy/$mode"; exit 5; }
  done
  say "--- BENCH $policy ---"
  "$BENCH" "$MODEL" 300 | tee "results/phone_smoke_${policy}.txt"
done

I8_TPS="$(grep '^deep_tokens_per_s=' results/phone_smoke_int8.txt | cut -d= -f2)"
MX_TPS="$(grep '^deep_tokens_per_s=' results/phone_smoke_mixed.txt | cut -d= -f2)"
I8_SZ="$(grep '^model_bytes=' results/phone_smoke_int8.txt | cut -d= -f2)"
MX_SZ="$(grep '^model_bytes=' results/phone_smoke_mixed.txt | cut -d= -f2)"
CHOSEN=int8
# Mixed wins only if it is smaller and retains at least 85% of INT8 deep throughput.
if awk -v mt="$MX_TPS" -v it="$I8_TPS" -v ms="$MX_SZ" -v is="$I8_SZ" 'BEGIN{exit !((ms<is)&&(mt>=0.85*it))}'; then CHOSEN=mixed; fi
kv chosen_weight_policy "$CHOSEN"
kv smoke_int8_deep_tps "$I8_TPS"
kv smoke_mixed_deep_tps "$MX_TPS"
kv smoke_int8_bytes "$I8_SZ"
kv smoke_mixed_bytes "$MX_SZ"

MOBILE=results/native_fixture/mobile_s_v06_mixed_BENCH_ONLY.rxm6
if [ -f "$MOBILE" ]; then
  say '--- MOBILE-S FULL GRAPH BENCH (RANDOM WEIGHTS; PERFORMANCE ONLY) ---'
  "$BENCH" "$MOBILE" 6 | tee results/phone_mobile_s_bench.txt
fi

say '--- MEMORY AFTER ---'
grep -E 'MemTotal|MemAvailable|SwapTotal|SwapFree' /proc/meminfo || true
say '--- THERMAL AFTER ---'
for f in /sys/class/thermal/thermal_zone*/temp; do [ -r "$f" ] && printf '%s=' "$f" && cat "$f"; done 2>/dev/null || true
say 'NOTE: mobile_s_v06_mixed_BENCH_ONLY.rxm6 has random weights and measures only runtime performance/RSS, never intelligence.'
say 'REASONIX_V06_PHONE_ONLY_PASS'
