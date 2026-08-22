#!/data/data/com.termux/files/usr/bin/bash
set -Eeuo pipefail

NIGHT_SESSION="reasonix-night"
WATCH_SESSION="reasonix-budget-watch"
PROMPT_DL="/sdcard/Download/REASONIX_3_NIGHT_QA_FIX_MASTER_PROMPT_FINAL.txt"
WATCH_DL="/sdcard/Download/REASONIX_NIGHT_BUDGET_WATCH.sh"
PROMPT_ROOT="/root/REASONIX_3_NIGHT_QA_FIX_MASTER_PROMPT.txt"
MIN_USD="${MIN_USD:-1.30}"

die() { echo "ERROR: $*" >&2; exit 1; }

command -v proot-distro >/dev/null || die "proot-distro not found"
command -v tmux >/dev/null || die "tmux not found"
command -v termux-wake-lock >/dev/null || die "termux-wake-lock not found"
test -f "$PROMPT_DL" || die "Download missing: $PROMPT_DL"
test -f "$WATCH_DL" || die "Download missing: $WATCH_DL"

echo "=== 1/8 WAKE LOCK ==="
termux-wake-lock || die "termux-wake-lock failed"

echo "=== 2/8 INSTALL NIGHT FILES ==="
cp "$WATCH_DL" "$HOME/REASONIX_NIGHT_BUDGET_WATCH.sh"
chmod 700 "$HOME/REASONIX_NIGHT_BUDGET_WATCH.sh"

proot-distro login debian -- bash -lc "
set -e
cp '$PROMPT_DL' '$PROMPT_ROOT'
test -s '$PROMPT_ROOT'
test -x /root/.opencode/bin/opencode
test -x /usr/local/go/bin/go
test -d /root/DeepSeek-Reasonix
echo OPENCODE=\$(/root/.opencode/bin/opencode --version 2>/dev/null | head -1 || true)
echo GO=\$(/usr/local/go/bin/go version)
"

echo "=== 3/8 CHECK FREE SPACE ==="
df -h /sdcard/Download | tail -1
FREE_KB="$(df -Pk /sdcard/Download | awk 'NR==2{print $4}')"
if [ "${FREE_KB:-0}" -lt 1048576 ]; then
  die "Less than 1 GiB free in shared storage"
fi

echo "=== 4/8 CHECK DEEPSEEK BALANCE (KEY IS NOT PRINTED) ==="
BAL="$(
proot-distro login debian -- bash -lc '
set -e
KEY="$(grep -oE "sk-[A-Za-z0-9_-]+" /root/REASONIX_3_MORNING_STABLE_WITH_API.txt 2>/dev/null | head -1)"
[ -n "$KEY" ] || { echo API_KEY_SOURCE_MISSING >&2; exit 20; }
curl -fsS --max-time 20 https://api.deepseek.com/user/balance \
  -H "Authorization: Bearer $KEY" |
python3 -c '"'"'
import json,sys
d=json.load(sys.stdin)
vals=[]
for x in d.get("balance_infos", []):
    if str(x.get("currency","")).upper()=="USD":
        try: vals.append(float(x.get("total_balance")))
        except Exception: pass
print(min(vals) if vals else "")
'"'"'
' 2>/dev/null
)"
[[ "$BAL" =~ ^[0-9]+([.][0-9]+)?$ ]] || die "Could not read DeepSeek USD balance"
echo "DeepSeek balance: \$$BAL ; safety floor: \$$MIN_USD"
awk -v bal="$BAL" -v floor="$MIN_USD" 'BEGIN { exit !(bal > floor) }' || die "Balance is already at/below safety floor" 

echo "=== 5/8 STOP OLD SUPERVISORS/SESSIONS ==="
tmux kill-session -t "$WATCH_SESSION" 2>/dev/null || true
tmux kill-session -t "$NIGHT_SESSION" 2>/dev/null || true
tmux kill-session -t reasonix-backend 2>/dev/null || true
proot-distro login debian -- bash -lc '
cd /root/reasonix-mobile-v1.5.1-backend 2>/dev/null &&
  ./reasonix_mobile_backend.sh stop >/dev/null 2>&1 || true
'

echo "=== 6/8 CREATE INNER RUNNER ==="
cat <<'INNER' | proot-distro login debian -- bash -lc 'cat > /root/reasonix-night-inner.sh && chmod 700 /root/reasonix-night-inner.sh'
#!/bin/bash
set -uo pipefail

export PATH=/usr/local/go/bin:/root/.opencode/bin:/root/reasonix-android-tools/bin:$PATH
export GOTOOLCHAIN=local
hash -r

REPO=/root/DeepSeek-Reasonix
PROMPT=/root/REASONIX_3_NIGHT_QA_FIX_MASTER_PROMPT.txt
LOG=/root/reasonix-night.log
REPORT=/root/DeepSeek-Reasonix/docs/REASONIX_3_NIGHT_QA_FIX_FINAL_REPORT.md
APK=/sdcard/Download/Reasonix-Mobile-v3.0.1-NIGHT.apk

cd "$REPO" || exit 10
: > "$LOG"

echo "[$(date -Is)] NIGHT START" | tee -a "$LOG"
echo "GO=$(go version)" | tee -a "$LOG"
echo "OPENCODE=$(opencode --version 2>/dev/null | head -1 || true)" | tee -a "$LOG"
echo "PROMPT_SHA256=$(sha256sum "$PROMPT" | awk '{print $1}')" | tee -a "$LOG"
echo "BASELINE=$(git rev-parse HEAD 2>/dev/null || true)" | tee -a "$LOG"

attempt=1
final_rc=1

while [ "$attempt" -le 2 ]; do
  echo "[$(date -Is)] OPENCODE ATTEMPT=$attempt" | tee -a "$LOG"

  nice -n 5 opencode run -m deepseek/deepseek-v4-flash "$(cat "$PROMPT")" \
    2>&1 | tee -a "$LOG"
  rc=${PIPESTATUS[0]}

  echo "[$(date -Is)] OPENCODE EXIT rc=$rc attempt=$attempt" | tee -a "$LOG"

  if [ -s "$REPORT" ] && [ -s "$APK" ]; then
    echo "[$(date -Is)] FINAL ARTIFACTS FOUND" | tee -a "$LOG"
    final_rc=0
    break
  fi

  if [ "$rc" -eq 0 ]; then
    echo "[$(date -Is)] Agent ended normally; no blind automatic rerun." | tee -a "$LOG"
    final_rc=0
    break
  fi

  if [ "$attempt" -ge 2 ]; then
    final_rc="$rc"
    break
  fi

  echo "[$(date -Is)] Abnormal CLI exit; one recovery retry in 60s." | tee -a "$LOG"
  sleep 60
  attempt=$((attempt+1))
done

echo "[$(date -Is)] NIGHT FINISH rc=$final_rc" | tee -a "$LOG"
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

cp -f "$LOG" /sdcard/Download/reasonix-night.log 2>/dev/null || true
[ -s "$REPORT" ] && cp -f "$REPORT" /sdcard/Download/REASONIX_3_NIGHT_QA_FIX_FINAL_REPORT.md 2>/dev/null || true

exit "$final_rc"
INNER

echo "=== 7/8 START OPENCODE + BUDGET WATCH ==="
tmux new-session -d -s "$NIGHT_SESSION" \
  "proot-distro login debian -- /root/reasonix-night-inner.sh"

sleep 2
tmux has-session -t "$NIGHT_SESSION" 2>/dev/null || die "night session failed to start"

tmux new-session -d -s "$WATCH_SESSION" \
  "SESSION='$NIGHT_SESSION' MIN_USD='$MIN_USD' INTERVAL=120 FAIL_LIMIT=3 '$HOME/REASONIX_NIGHT_BUDGET_WATCH.sh' 2>&1 | tee -a '$HOME/reasonix-budget-watch.log'"

sleep 2
tmux has-session -t "$WATCH_SESSION" 2>/dev/null || die "budget watcher failed to start"

echo "=== 8/8 STATUS ==="
tmux ls
echo
echo "--- OpenCode tail ---"
tmux capture-pane -pt "$NIGHT_SESSION" -S -20 2>/dev/null || true
echo
echo "--- Budget watcher tail ---"
tmux capture-pane -pt "$WATCH_SESSION" -S -10 2>/dev/null || true
echo
echo "READY."
echo "Keep Termux protected from battery optimization and do NOT swipe it away."
echo "Morning log: /sdcard/Download/reasonix-night.log"
echo "Target APK: /sdcard/Download/Reasonix-Mobile-v3.0.1-NIGHT.apk"
echo "Target report: /sdcard/Download/REASONIX_3_NIGHT_QA_FIX_FINAL_REPORT.md"
