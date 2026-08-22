#!/data/data/com.termux/files/usr/bin/bash
set -u

SESSION="${SESSION:-reasonix-night}"
MIN_USD="${MIN_USD:-1.30}"
INTERVAL="${INTERVAL:-120}"
FAIL_LIMIT="${FAIL_LIMIT:-3}"
fails=0

stop_night() {
  echo "[$(date '+%F %T')] stopping ${SESSION}: $1"
  tmux send-keys -t "$SESSION" C-c 2>/dev/null || true
  sleep 8
  tmux kill-session -t "$SESSION" 2>/dev/null || true
}

echo "Reasonix night budget watch: session=$SESSION floor=\$$MIN_USD interval=${INTERVAL}s"

while tmux has-session -t "$SESSION" 2>/dev/null; do
  BAL="$(
    proot-distro login debian -- bash -lc '
      KEY="$(grep -oE "sk-[A-Za-z0-9_-]+" /root/REASONIX_3_MORNING_STABLE_WITH_API.txt 2>/dev/null | head -1)"
      [ -n "$KEY" ] || exit 20
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

  if [[ "$BAL" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    fails=0
    echo "[$(date '+%F %T')] DeepSeek USD balance: \$$BAL"
    if python - "$BAL" "$MIN_USD" <<'PY'
import sys
bal=float(sys.argv[1]); floor=float(sys.argv[2])
raise SystemExit(0 if bal <= floor else 1)
PY
    then
      stop_night "balance reached safety floor (\$$BAL <= \$$MIN_USD)"
      exit 0
    fi
  else
    fails=$((fails+1))
    echo "[$(date '+%F %T')] balance read failed ($fails/$FAIL_LIMIT)"
    if [ "$fails" -ge "$FAIL_LIMIT" ]; then
      stop_night "cannot verify balance safely"
      exit 2
    fi
  fi

  sleep "$INTERVAL"
done

echo "[$(date '+%F %T')] ${SESSION} ended normally; watcher exits."
