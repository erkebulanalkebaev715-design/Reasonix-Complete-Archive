#!/data/data/com.termux/files/usr/bin/bash
set -Eeuo pipefail

SESSION="${SESSION:-reasonix-night}"
MIN_USD="${MIN_USD:-1.50}"
INTERVAL="${INTERVAL:-60}"
FAIL_LIMIT="${FAIL_LIMIT:-2}"
MAX_DROP_USD="${MAX_DROP_USD:-0.30}"
START_BAL="${START_BAL:-}"
PGID_FILE="${PGID_FILE:-$HOME/.reasonix-night.pgid}"
STOP_FLAG="${STOP_FLAG:-/sdcard/Download/.reasonix-night-budget-stop}"
KEY_FILE="${DEEPSEEK_KEY_FILE:-/root/REASONIX_3_MORNING_STABLE_WITH_API.txt}"
fails=0
prev_bal="$START_BAL"
graceful=0
stopping=0

is_num() { [[ "${1:-}" =~ ^[0-9]+([.][0-9]+)?$ ]]; }

is_num "$MIN_USD" || { echo "invalid MIN_USD: $MIN_USD" >&2; exit 64; }
is_num "$MAX_DROP_USD" || { echo "invalid MAX_DROP_USD: $MAX_DROP_USD" >&2; exit 64; }
[[ "$INTERVAL" =~ ^[0-9]+$ ]] && [ "$INTERVAL" -ge 10 ] || { echo "invalid INTERVAL: $INTERVAL" >&2; exit 64; }
[[ "$FAIL_LIMIT" =~ ^[0-9]+$ ]] && [ "$FAIL_LIMIT" -ge 1 ] || { echo "invalid FAIL_LIMIT: $FAIL_LIMIT" >&2; exit 64; }
if [ -n "$prev_bal" ] && ! is_num "$prev_bal"; then
  echo "invalid START_BAL: $prev_bal" >&2
  exit 64
fi

read_balance() {
  proot-distro login debian -- env REASONIX_NIGHT_KEY_FILE="$KEY_FILE" bash -lc '
    set -o pipefail
    KEY_FILE="${REASONIX_NIGHT_KEY_FILE:?}"
    KEY="$(grep -oE "sk-[A-Za-z0-9_-]+" "$KEY_FILE" 2>/dev/null | head -1)"
    [ -n "$KEY" ] || exit 20
    curl -fsS --max-time 20 https://api.deepseek.com/user/balance \
      -H "Authorization: Bearer $KEY" |
    python3 -c '\''
import json,sys
d=json.load(sys.stdin)
vals=[]
for x in d.get("balance_infos", []):
    if str(x.get("currency", "")).upper() == "USD":
        try: vals.append(float(x.get("total_balance")))
        except Exception: pass
print(min(vals) if vals else "")
'\''
  '
}

get_pgid() {
  local pgid=""
  [ -s "$PGID_FILE" ] && pgid="$(tr -cd '0-9' < "$PGID_FILE" 2>/dev/null || true)"
  [[ "$pgid" =~ ^[0-9]+$ ]] && [ "$pgid" -gt 1 ] && printf '%s\n' "$pgid"
}

group_alive() {
  local pgid
  pgid="$(get_pgid || true)"
  [ -n "$pgid" ] && kill -0 -- "-$pgid" 2>/dev/null
}

stop_night() {
  local reason="$1" pgid=""
  [ "$stopping" -eq 0 ] || return 0
  stopping=1
  echo "[$(date '+%F %T')] STOP: $reason"
  printf '%s\n' "$reason" > "$STOP_FLAG" 2>/dev/null || true

  pgid="$(get_pgid || true)"
  if [ -n "$pgid" ]; then
    echo "[$(date '+%F %T')] interrupting process-group $pgid"
    kill -INT -- "-$pgid" 2>/dev/null || true
    sleep 8
    if kill -0 -- "-$pgid" 2>/dev/null; then
      echo "[$(date '+%F %T')] escalating process-group $pgid to TERM"
      kill -TERM -- "-$pgid" 2>/dev/null || true
      sleep 3
    fi
    if kill -0 -- "-$pgid" 2>/dev/null; then
      echo "[$(date '+%F %T')] escalating process-group $pgid to KILL"
      kill -KILL -- "-$pgid" 2>/dev/null || true
    fi
  else
    echo "[$(date '+%F %T')] PGID unavailable; falling back to tmux interrupt"
    tmux send-keys -t "$SESSION" C-c 2>/dev/null || true
    sleep 3
  fi

  tmux kill-session -t "$SESSION" 2>/dev/null || true
}

on_exit() {
  local rc=$?
  trap - EXIT
  if [ "$graceful" -eq 0 ] && [ "$stopping" -eq 0 ] && tmux has-session -t "$SESSION" 2>/dev/null; then
    stop_night "budget watcher exited unexpectedly rc=$rc"
  fi
  rm -f "$PGID_FILE" 2>/dev/null || true
  termux-wake-unlock >/dev/null 2>&1 || true
  exit "$rc"
}
trap on_exit EXIT

rm -f "$STOP_FLAG" 2>/dev/null || true

echo "Reasonix V2 budget watch: session=$SESSION floor=\$$MIN_USD interval=${INTERVAL}s fail_limit=$FAIL_LIMIT max_drop=\$$MAX_DROP_USD"
[ -n "$prev_bal" ] && echo "[$(date '+%F %T')] start balance: \$$prev_bal"

while tmux has-session -t "$SESSION" 2>/dev/null; do
  BAL="$(read_balance 2>/dev/null || true)"

  if is_num "$BAL"; then
    fails=0
    echo "[$(date '+%F %T')] DeepSeek USD balance: \$$BAL"

    if [ -n "$prev_bal" ] && is_num "$prev_bal"; then
      drop="$(awk -v prev="$prev_bal" -v cur="$BAL" 'BEGIN { printf "%.6f", prev-cur }')"
      if awk -v drop="$drop" -v max="$MAX_DROP_USD" 'BEGIN { exit !(drop > max) }'; then
        stop_night "abnormal single-interval spend drop \$$drop > \$$MAX_DROP_USD"
        graceful=1
        exit 3
      fi
    fi

    if awk -v bal="$BAL" -v floor="$MIN_USD" 'BEGIN { exit !(bal <= floor) }'; then
      stop_night "balance reached safety floor (\$$BAL <= \$$MIN_USD)"
      graceful=1
      exit 0
    fi

    prev_bal="$BAL"
  else
    fails=$((fails+1))
    echo "[$(date '+%F %T')] balance read failed ($fails/$FAIL_LIMIT)"
    if [ "$fails" -ge "$FAIL_LIMIT" ]; then
      stop_night "cannot verify DeepSeek balance safely"
      graceful=1
      exit 2
    fi
  fi

  sleep "$INTERVAL"
done

if group_alive; then
  stop_night "tmux session disappeared while process-group is still alive"
  graceful=1
  exit 4
fi

graceful=1
echo "[$(date '+%F %T')] ${SESSION} ended; watcher exits and releases wake lock."
