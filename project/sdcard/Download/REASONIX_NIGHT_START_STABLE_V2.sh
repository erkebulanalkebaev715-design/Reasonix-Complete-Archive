#!/data/data/com.termux/files/usr/bin/bash
set -Eeuo pipefail

NIGHT_SESSION="reasonix-night"
WATCH_SESSION="reasonix-budget-watch"
PROMPT_DL="/sdcard/Download/REASONIX_3_NIGHT_QA_FIX_MASTER_PROMPT_FINAL_V2.txt"
WATCH_DL="/sdcard/Download/REASONIX_NIGHT_BUDGET_WATCH_V2.sh"
PROMPT_ROOT="/root/REASONIX_3_NIGHT_QA_FIX_MASTER_PROMPT.txt"
MIN_USD="${MIN_USD:-1.50}"
INTERVAL="${INTERVAL:-60}"
FAIL_LIMIT="${FAIL_LIMIT:-2}"
MAX_DROP_USD="${MAX_DROP_USD:-0.30}"
KEY_FILE="${DEEPSEEK_KEY_FILE:-/root/REASONIX_3_MORNING_STABLE_WITH_API.txt}"
OPENCODE_MODEL="${OPENCODE_MODEL:-}"
PGID_FILE="$HOME/.reasonix-night.pgid"
READY_FLAG="$HOME/.reasonix-night.ready"
STOP_FLAG="/sdcard/Download/.reasonix-night-budget-stop"
WATCH_HOME="$HOME/REASONIX_NIGHT_BUDGET_WATCH_V2.sh"
WATCH_LOG="$HOME/reasonix-budget-watch.log"
EXPECTED_PACKAGE="com.reasonix.mobile.installfix"
EXPECTED_CERT_SHA256="fdf18d0b9d5372d142caf4fe76598e761090db75b5c66165a41b0ce67c65e48c"
cleanup_armed=1
night_started=0
wake_locked=0

die() { echo "ERROR: $*" >&2; exit 1; }
is_num() { [[ "${1:-}" =~ ^[0-9]+([.][0-9]+)?$ ]]; }

kill_night_group() {
  local pgid=""
  [ -s "$PGID_FILE" ] && pgid="$(tr -cd '0-9' < "$PGID_FILE" 2>/dev/null || true)"
  if [[ "$pgid" =~ ^[0-9]+$ ]] && [ "$pgid" -gt 1 ]; then
    kill -INT -- "-$pgid" 2>/dev/null || true
    sleep 2
    kill -TERM -- "-$pgid" 2>/dev/null || true
  fi
  tmux kill-session -t "$NIGHT_SESSION" 2>/dev/null || true
}

cleanup() {
  local rc=$?
  trap - EXIT
  if [ "$cleanup_armed" -eq 1 ]; then
    tmux kill-session -t "$WATCH_SESSION" 2>/dev/null || true
    [ "$night_started" -eq 1 ] && kill_night_group || true
    rm -f "$READY_FLAG" "$PGID_FILE" 2>/dev/null || true
    if [ "$wake_locked" -eq 1 ]; then
      termux-wake-unlock >/dev/null 2>&1 || true
    fi
  fi
  exit "$rc"
}
trap cleanup EXIT

is_num "$MIN_USD" || die "MIN_USD must be numeric"
is_num "$MAX_DROP_USD" || die "MAX_DROP_USD must be numeric"
[[ "$INTERVAL" =~ ^[0-9]+$ ]] && [ "$INTERVAL" -ge 10 ] || die "INTERVAL must be integer >= 10"
[[ "$FAIL_LIMIT" =~ ^[0-9]+$ ]] && [ "$FAIL_LIMIT" -ge 1 ] || die "FAIL_LIMIT must be integer >= 1"

for c in proot-distro tmux termux-wake-lock awk df ps; do
  command -v "$c" >/dev/null || die "$c not found"
done
ps -o pgid= -p $$ >/dev/null 2>&1 || die "ps does not support PGID lookup required by V2 fail-safe"

test -f "$PROMPT_DL" || die "Download missing: $PROMPT_DL"
test -f "$WATCH_DL" || die "Download missing: $WATCH_DL"

echo "=== 1/9 DEBIAN / TOOLCHAIN PREFLIGHT ==="
PREFLIGHT="$({
  proot-distro login debian -- env REQUESTED_MODEL="$OPENCODE_MODEL" REASONIX_NIGHT_KEY_FILE="$KEY_FILE" bash -lc '
    set -e
    export PATH=/usr/local/go/bin:/root/.opencode/bin:/root/reasonix-android-tools/bin:$PATH
    export GOTOOLCHAIN=local
    cd /root/DeepSeek-Reasonix
    test -x /root/.opencode/bin/opencode
    test -x /usr/local/go/bin/go
    command -v curl >/dev/null
    command -v python3 >/dev/null
    command -v grep >/dev/null
    command -v git >/dev/null
    command -v sha256sum >/dev/null
    command -v stat >/dev/null
    command -v apksigner >/dev/null
    command -v aapt >/dev/null || command -v aapt2 >/dev/null
    test -s "$REASONIX_NIGHT_KEY_FILE"

    opencode run --help 2>&1 | grep -q -- "--continue" || { echo OPENCODE_RUN_CONTINUE_UNAVAILABLE >&2; exit 33; }
    opencode run --help 2>&1 | grep -q -- "--auto" || { echo OPENCODE_RUN_AUTO_UNAVAILABLE >&2; exit 34; }
    models="$(opencode models 2>/dev/null || true)"
    model="${REQUESTED_MODEL:-}"
    if [ -n "$model" ]; then
      printf "%s\n" "$models" | grep -Fx "$model" >/dev/null || { echo "MODEL_NOT_FOUND=$model" >&2; exit 31; }
    else
      for candidate in custom-api-deepseek-com/deepseek-v4-flash deepseek/deepseek-v4-flash; do
        if printf "%s\n" "$models" | grep -Fx "$candidate" >/dev/null; then model="$candidate"; break; fi
      done
      if [ -z "$model" ]; then
        model="$(printf "%s\n" "$models" | grep -E "/deepseek-v4-flash$" | head -1 || true)"
      fi
      [ -n "$model" ] || { echo MODEL_DEEPSEEK_V4_FLASH_NOT_FOUND >&2; exit 32; }
    fi
    provider="${model%%/*}"
    python3 - "$provider" "$REASONIX_NIGHT_KEY_FILE" <<'PYKEY'
import json,re,sys,hmac
from pathlib import Path
provider,key_file=sys.argv[1:3]
m=re.search(r"sk-[A-Za-z0-9_-]+", Path(key_file).read_text(errors="ignore"))
if not m:
    raise SystemExit(40)
watch_key=m.group(0)
auth_path=Path.home()/".local/share/opencode/auth.json"
try:
    auth=json.loads(auth_path.read_text())
except Exception:
    raise SystemExit(41)
entry=auth.get(provider)
if not isinstance(entry, dict) or entry.get("type") != "api" or not isinstance(entry.get("key"), str):
    raise SystemExit(42)
if not hmac.compare_digest(entry["key"], watch_key):
    raise SystemExit(43)
PYKEY
    echo "BUDGET_KEY_MATCH=verified"
    echo "MODEL=$model"
    echo "GO=$(go version)"
    echo "OPENCODE=$(opencode --version 2>/dev/null | head -1 || true)"
  '
} 2>&1)" || { printf '%s\n' "$PREFLIGHT" >&2; die "Debian/toolchain preflight failed"; }
printf '%s\n' "$PREFLIGHT"
OPENCODE_MODEL="$(printf '%s\n' "$PREFLIGHT" | sed -n 's/^MODEL=//p' | tail -1)"
[ -n "$OPENCODE_MODEL" ] || die "Could not resolve OpenCode Flash model"

echo "=== 2/9 FREE SPACE ==="
df -h /sdcard/Download | tail -1
FREE_KB="$(df -Pk /sdcard/Download | awk 'NR==2{print $4}')"
[ "${FREE_KB:-0}" -ge 1048576 ] || die "Less than 1 GiB free in shared storage"
ROOT_FREE_KB="$(proot-distro login debian -- bash -lc 'df -Pk /root/DeepSeek-Reasonix | tail -1 | tr -s " " | cut -d" " -f4')"
[ "${ROOT_FREE_KB:-0}" -ge 1048576 ] || die "Less than 1 GiB free in Debian project filesystem"

echo "=== 3/9 CHECK DEEPSEEK BALANCE ==="
BAL="$(proot-distro login debian -- env REASONIX_NIGHT_KEY_FILE="$KEY_FILE" bash -lc '
  set -o pipefail
  KEY="$(grep -oE "sk-[A-Za-z0-9_-]+" "$REASONIX_NIGHT_KEY_FILE" 2>/dev/null | head -1)"
  [ -n "$KEY" ] || exit 20
  curl -fsS --max-time 20 https://api.deepseek.com/user/balance \
    -H "Authorization: Bearer $KEY" |
  python3 -c '\''
import json,sys
d=json.load(sys.stdin)
vals=[]
for x in d.get("balance_infos", []):
    if str(x.get("currency", "")).upper()=="USD":
        try: vals.append(float(x.get("total_balance")))
        except Exception: pass
print(min(vals) if vals else "")
'\''
' 2>/dev/null || true)"
is_num "$BAL" || die "Could not read DeepSeek USD balance"
echo "DeepSeek USD balance: \$$BAL ; safety floor: \$$MIN_USD"
awk -v bal="$BAL" -v floor="$MIN_USD" 'BEGIN { exit !(bal > floor) }' || die "Balance is already at/below safety floor"

echo "=== 4/9 WAKE LOCK ==="
termux-wake-lock || die "termux-wake-lock failed"
wake_locked=1

echo "=== 5/9 INSTALL NIGHT FILES ==="
cp "$WATCH_DL" "$WATCH_HOME"
chmod 700 "$WATCH_HOME"
: > "$WATCH_LOG"
rm -f "$PGID_FILE" "$READY_FLAG" "$STOP_FLAG" 2>/dev/null || true
proot-distro login debian -- bash -lc "cp '$PROMPT_DL' '$PROMPT_ROOT' && test -s '$PROMPT_ROOT'"

echo "=== 6/9 STOP OLD REASONIX SUPERVISORS ==="
tmux kill-session -t "$WATCH_SESSION" 2>/dev/null || true
tmux kill-session -t "$NIGHT_SESSION" 2>/dev/null || true
tmux kill-session -t reasonix-backend 2>/dev/null || true
proot-distro login debian -- bash -lc '
  cd /root/reasonix-mobile-v1.5.1-backend 2>/dev/null &&
    ./reasonix_mobile_backend.sh stop >/dev/null 2>&1 || true
'

echo "=== 7/9 CREATE INNER RUNNER ==="
cat <<'INNER' | proot-distro login debian -- bash -lc 'cat > /root/reasonix-night-inner-v2.sh && chmod 700 /root/reasonix-night-inner-v2.sh'
#!/bin/bash
set -uo pipefail

export PATH=/usr/local/go/bin:/root/.opencode/bin:/root/reasonix-android-tools/bin:$PATH
export GOTOOLCHAIN=local
export OPENCODE_DISABLE_AUTOUPDATE=1
export OPENCODE_DISABLE_LSP_DOWNLOAD=1
hash -r

REPO=/root/DeepSeek-Reasonix
PROMPT=/root/REASONIX_3_NIGHT_QA_FIX_MASTER_PROMPT.txt
LOG=/root/reasonix-night.log
REPORT=/root/DeepSeek-Reasonix/docs/REASONIX_3_NIGHT_QA_FIX_FINAL_REPORT.md
APK=/sdcard/Download/Reasonix-Mobile-v3.0.1-NIGHT.apk
STOP_FLAG=/sdcard/Download/.reasonix-night-budget-stop
MODEL="${OPENCODE_MODEL:?OPENCODE_MODEL missing}"
RUN_START_EPOCH="${RUN_START_EPOCH:?RUN_START_EPOCH missing}"
EXPECTED_PACKAGE="${EXPECTED_PACKAGE:?EXPECTED_PACKAGE missing}"
EXPECTED_CERT_SHA256="${EXPECTED_CERT_SHA256:?EXPECTED_CERT_SHA256 missing}"
final_rc=1

copy_outputs() {
  cp -f "$LOG" /sdcard/Download/reasonix-night.log 2>/dev/null || true
  [ -s "$REPORT" ] && cp -f "$REPORT" /sdcard/Download/REASONIX_3_NIGHT_QA_FIX_FINAL_REPORT.md 2>/dev/null || true
}
trap copy_outputs EXIT
trap 'echo "[$(date -Is)] INNER INTERRUPTED" >> "$LOG" 2>/dev/null || true; exit 130' INT TERM

artifact_fresh() {
  local f="$1" m=""
  [ -s "$f" ] || return 1
  m="$(stat -c %Y "$f" 2>/dev/null || echo 0)"
  [ "$m" -ge "$RUN_START_EPOCH" ]
}

validate_artifacts() {
  artifact_fresh "$REPORT" || { echo "VALIDATION: fresh report missing" | tee -a "$LOG"; return 1; }
  artifact_fresh "$APK" || { echo "VALIDATION: fresh APK missing" | tee -a "$LOG"; return 1; }

  local pkg="" cert="" aapt_cmd=""
  if command -v aapt >/dev/null; then aapt_cmd=aapt; else aapt_cmd=aapt2; fi
  pkg="$($aapt_cmd dump badging "$APK" 2>/dev/null | sed -n "s/^package: name='\([^']*\)'.*/\1/p" | head -1)"
  [ "$pkg" = "$EXPECTED_PACKAGE" ] || { echo "VALIDATION: package mismatch: ${pkg:-<none>}" | tee -a "$LOG"; return 1; }

  apksigner verify "$APK" >/dev/null 2>&1 || { echo "VALIDATION: apksigner verify failed" | tee -a "$LOG"; return 1; }
  cert="$(apksigner verify --print-certs "$APK" 2>/dev/null | awk -F': ' '/certificate SHA-256 digest/ {print $2; exit}' | tr -d ':' | tr 'A-F' 'a-f')"
  [ "$cert" = "$EXPECTED_CERT_SHA256" ] || { echo "VALIDATION: signing cert mismatch" | tee -a "$LOG"; return 1; }

  grep -qE 'PASS|READY_FOR_DEVICE_TEST|BLOCKED' "$REPORT" || { echo "VALIDATION: report lacks status evidence" | tee -a "$LOG"; return 1; }
  echo "VALIDATION: mandatory APK/report checks passed" | tee -a "$LOG"
  return 0
}

run_agent() {
  local mode="$1" text="$2" rc=0
  echo "[$(date -Is)] OPENCODE $mode model=$MODEL" | tee -a "$LOG"
  if [ "$mode" = "INITIAL" ]; then
    nice -n 5 opencode run --auto -m "$MODEL" "$text" 2>&1 | tee -a "$LOG"
  else
    nice -n 5 opencode run --auto --continue -m "$MODEL" "$text" 2>&1 | tee -a "$LOG"
  fi
  rc=${PIPESTATUS[0]}
  echo "[$(date -Is)] OPENCODE $mode EXIT rc=$rc" | tee -a "$LOG"
  return "$rc"
}

cd "$REPO" || exit 10
: > "$LOG"
echo "[$(date -Is)] NIGHT V2 START" | tee -a "$LOG"
echo "GO=$(go version)" | tee -a "$LOG"
echo "OPENCODE=$(opencode --version 2>/dev/null | head -1 || true)" | tee -a "$LOG"
echo "MODEL=$MODEL" | tee -a "$LOG"
echo "PROMPT_SHA256=$(sha256sum "$PROMPT" | awk '{print $1}')" | tee -a "$LOG"
echo "BASELINE=$(git rev-parse HEAD 2>/dev/null || true)" | tee -a "$LOG"
echo "RUN_START_EPOCH=$RUN_START_EPOCH" | tee -a "$LOG"

initial_rc=0
run_agent INITIAL "$(cat "$PROMPT")" || initial_rc=$?

if validate_artifacts; then
  final_rc=0
else
  if [ -s "$STOP_FLAG" ]; then
    echo "[$(date -Is)] Budget watchdog stop flag present; no continuation." | tee -a "$LOG"
    final_rc=90
  else
    echo "[$(date -Is)] Mandatory artifacts are not yet valid. One continuation pass only." | tee -a "$LOG"
    cont='CONTINUE THE SAME EXISTING Reasonix night repair from the current repo and current OpenCode session. DO NOT restart the project, redo completed work, or repeat unchanged paid tests. Inspect what is already changed and the existing night log/report state. Finish only the remaining mandatory gates. A successful finish requires a fresh /sdcard/Download/Reasonix-Mobile-v3.0.1-NIGHT.apk with package com.reasonix.mobile.installfix and preserved signing lineage, plus /root/DeepSeek-Reasonix/docs/REASONIX_3_NIGHT_QA_FIX_FINAL_REPORT.md with evidence and PASS/READY_FOR_DEVICE_TEST/BLOCKED matrix. If blocked, write the truthful report and stop. No subagents, Flash only, local-first.'
    continuation_rc=0
    run_agent CONTINUATION "$cont" || continuation_rc=$?
    if validate_artifacts; then
      final_rc=0
    else
      echo "[$(date -Is)] Mandatory artifact validation still failed after the single continuation." | tee -a "$LOG"
      final_rc=42
      [ "$continuation_rc" -ne 0 ] && final_rc="$continuation_rc"
    fi
  fi
fi

echo "[$(date -Is)] NIGHT V2 FINISH rc=$final_rc initial_rc=$initial_rc" | tee -a "$LOG"
if [ -s "$APK" ]; then
  echo "APK_SHA256=$(sha256sum "$APK" | awk '{print $1}')" | tee -a "$LOG"
  ls -lh "$APK" | tee -a "$LOG"
else
  echo "APK_NOT_FOUND" | tee -a "$LOG"
fi
if [ -s "$REPORT" ]; then
  echo "REPORT_FOUND=$REPORT" | tee -a "$LOG"
else
  echo "REPORT_NOT_FOUND" | tee -a "$LOG"
fi

exit "$final_rc"
INNER

RUN_START_EPOCH="$(date +%s)"

echo "=== 8/9 ARM NIGHT SESSION + WATCHDOG ==="
tmux new-session -d -s "$NIGHT_SESSION" \
  "while [ ! -f '$READY_FLAG' ]; do sleep 0.1; done; exec proot-distro login debian -- env OPENCODE_MODEL='$OPENCODE_MODEL' RUN_START_EPOCH='$RUN_START_EPOCH' EXPECTED_PACKAGE='$EXPECTED_PACKAGE' EXPECTED_CERT_SHA256='$EXPECTED_CERT_SHA256' /root/reasonix-night-inner-v2.sh"
night_started=1
sleep 1
tmux has-session -t "$NIGHT_SESSION" 2>/dev/null || die "night tmux session failed to arm"
PANE_PID="$(tmux display-message -p -t "$NIGHT_SESSION":0.0 '#{pane_pid}')"
PGID="$(ps -o pgid= -p "$PANE_PID" 2>/dev/null | tr -d ' ' || true)"
[[ "$PGID" =~ ^[0-9]+$ ]] && [ "$PGID" -gt 1 ] || die "could not resolve night process-group"
printf '%s\n' "$PGID" > "$PGID_FILE"

tmux new-session -d -s "$WATCH_SESSION" \
  "SESSION='$NIGHT_SESSION' MIN_USD='$MIN_USD' INTERVAL='$INTERVAL' FAIL_LIMIT='$FAIL_LIMIT' MAX_DROP_USD='$MAX_DROP_USD' START_BAL='$BAL' PGID_FILE='$PGID_FILE' STOP_FLAG='$STOP_FLAG' DEEPSEEK_KEY_FILE='$KEY_FILE' '$WATCH_HOME' 2>&1 | tee -a '$WATCH_LOG'"
sleep 2
tmux has-session -t "$WATCH_SESSION" 2>/dev/null || die "budget watcher failed to start"
tmux has-session -t "$NIGHT_SESSION" 2>/dev/null || die "night session was stopped during watchdog pre-arm"

touch "$READY_FLAG"

sleep 2
tmux has-session -t "$WATCH_SESSION" 2>/dev/null || die "budget watcher died after OpenCode release"
tmux has-session -t "$NIGHT_SESSION" 2>/dev/null || die "night session died immediately after OpenCode release"

cleanup_armed=0

echo "=== 9/9 STATUS ==="
tmux ls
echo
echo "Resolved OpenCode model: $OPENCODE_MODEL"
echo "Night process-group: $PGID"
echo "DeepSeek start balance: \$$BAL ; floor: \$$MIN_USD"
echo "Watch interval: ${INTERVAL}s ; fail limit: $FAIL_LIMIT ; max one-interval drop: \$$MAX_DROP_USD"
echo
echo "--- OpenCode tail ---"
tmux capture-pane -pt "$NIGHT_SESSION" -S -25 2>/dev/null || true
echo
echo "--- Budget watcher tail ---"
tmux capture-pane -pt "$WATCH_SESSION" -S -15 2>/dev/null || true
echo
echo "READY_V2. Watchdog is armed before the OpenCode workload proceeds."
echo "Morning log: /sdcard/Download/reasonix-night.log"
echo "Target APK: /sdcard/Download/Reasonix-Mobile-v3.0.1-NIGHT.apk"
echo "Target report: /sdcard/Download/REASONIX_3_NIGHT_QA_FIX_FINAL_REPORT.md"
