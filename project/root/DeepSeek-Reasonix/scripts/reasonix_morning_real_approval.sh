#!/usr/bin/env bash
set -euo pipefail

# MORNING STAGE M7 — REAL APPROVAL / MUTATION TEST (Morning Stable run).
# Proves the native approval invariant end-to-end with the REAL DeepSeek
# provider in ONE turn:
#   mutation -> ApprovalRequest -> WAIT -> DENY (no mutation, no blind
#   re-mutation, approval_wait is not a generic no-progress/failure) ->
#   agent retries -> ApprovalRequest -> WAIT -> ALLOW -> continuation ->
#   mutation -> read-back -> final answer confirms the marker.
# Disposable workspace + disposable temp file. Fails closed.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CURL_BIN="${CURL_BIN:-curl}"
PY_BIN="${PY_BIN:-python3}"
APPROVAL_REQUIRED='YES_I_EXPLICITLY_APPROVE_DEEPSEEK_API'
FX="${BALANCE_V20_USD_KZT:-}"
BUDGET="${BALANCE_V20_BUDGET_KZT:-25}"
WAIT_SECONDS="${BALANCE_V20_WAIT_SECONDS:-180}"

redact() { sed -E 's/sk-[A-Za-z0-9_-]+/<redacted>/g' -e 's/(Bearer )[A-Za-z0-9._-]+/\1<redacted>/g'; }

if [[ "${BALANCE_V20_REAL_API_APPROVED:-}" != "$APPROVAL_REQUIRED" ]]; then
  echo 'M7_REAL_APPROVAL_LOCKED: explicit approval missing'; exit 20
fi
[[ -n "${DEEPSEEK_API_KEY:-}" && "$DEEPSEEK_API_KEY" == sk-* ]] || { echo 'M7_REAL_APPROVAL_FAIL: DEEPSEEK_API_KEY missing' >&2; exit 1; }

cd "$ROOT"
TMP="$(mktemp -d)"
PID=''; SERVER_PID=''
cleanup() { set +e; [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null; [[ -n "$PID" ]] && kill "$PID" 2>/dev/null; rm -rf "$TMP"; }
trap cleanup EXIT

mkdir -p "$TMP/home" "$TMP/rxhome"
chmod 700 "$TMP/home" "$TMP/rxhome"

cat > "$TMP/reasonix.toml" <<TOML
default_model = "deepseek-v20/deepseek-v4-flash"

[agent]
system_prompt = "You are a careful assistant. To create a file you must use the write_file tool and nothing else. Do not use the ask tool. If write_file is blocked, retry write_file once."

[[providers]]
name = "deepseek-v20"
kind = "openai"
base_url = "https://api.deepseek.com"
models = ["deepseek-v4-flash"]
default = "deepseek-v4-flash"
api_key_env = "DEEPSEEK_API_KEY"
context_window = 1000000
max_output_tokens = 256
billing_currency = "USD"
price = { cache_hit = 0.014, input = 0.44, output = 1.32, currency = "USD" }
effort = "disabled"

[environment]
enabled = false
offline = false

[tools]
enabled = []

[sandbox]
bash = "off"

[permissions]
mode = "ask"

[serve]
auth_mode = "token"
TOML

printf 'DEEPSEEK_API_KEY=%s\n' "$DEEPSEEK_API_KEY" > "$TMP/rxhome/.env"
chmod 600 "$TMP/rxhome/.env"

MUT="$TMP/MORNING_APPROVAL_TARGET.txt"

PORT_FILE="$TMP/port"; PID_FILE="$TMP/pid"; TOKEN_FILE="$TMP/token"
LOG="$TMP/serve.log"

"$PY_BIN" - "$TOKEN_FILE" <<'PY'
import os,secrets,sys
fd=os.open(sys.argv[1], os.O_WRONLY|os.O_CREAT|os.O_TRUNC, 0o600)
with os.fdopen(fd,'w') as f: f.write(secrets.token_hex(32))
PY

( cd "$TMP"
  HOME="$TMP/home" REASONIX_HOME="$TMP/rxhome" \
  BALANCE_V20_USAGE_RECEIPT_PATH="$TMP/provider-usage.json" \
  "$ROOT/bin/reasonix" serve --model deepseek-v20/deepseek-v4-flash \
    --addr 127.0.0.1:0 --auth token --port-file "$PORT_FILE" \
    --token-file "$TOKEN_FILE" --pid-file "$PID_FILE" >"$LOG" 2>&1 ) &
PID=$!

for _ in $(seq 1 200); do [[ -s "$PORT_FILE" && -s "$PID_FILE" ]] && break; kill -0 "$PID" 2>/dev/null || { echo 'M7_REAL_APPROVAL_FAIL: serve exited' >&2; cat "$LOG" | redact >&2; exit 1; }; sleep 0.1; done
[[ -s "$PORT_FILE" ]] || { echo 'M7_REAL_APPROVAL_FAIL: no port' >&2; exit 1; }
ADDR="$(tr -d '\r\n' < "$PORT_FILE")"; SERVER_PID="$(tr -d '\r\n' < "$PID_FILE")"
BASE="http://$ADDR"; TOKEN="$(tr -d '\r\n' < "$TOKEN_FILE")"
[[ "$ADDR" == 127.0.0.1:* ]] || { echo 'M7_REAL_APPROVAL_FAIL: escaped loopback' >&2; exit 1; }

COOKIE="$TMP/cookie"
"$PY_BIN" - "$TOKEN" "$TMP/auth.json" <<'PY'
import json,sys
json.dump({'token':sys.argv[1]},open(sys.argv[2],'w',encoding='utf-8'))
PY
H="$("$CURL_BIN" -sS --connect-timeout 5 --max-time 10 -o /dev/null -w '%{http_code}' -c "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/auth.json" "$BASE/auth/token")"
[[ "$H" == 204 ]] || { echo "M7_REAL_APPROVAL_FAIL: auth HTTP $H" >&2; exit 1; }

"$PY_BIN" - "$TMP/breq.json" "$BUDGET" "$FX" <<'PY'
import json,sys
json.dump({'budgetKzt':float(sys.argv[2]),'reservePercent':20,'proMaxPercent':0,'hardStop':True,'fxKztPerUnit':{'USD':float(sys.argv[3])}},open(sys.argv[1],'w',encoding='utf-8'))
PY
"$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 -b "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/breq.json" "$BASE/mod/budget" > "$TMP/before.json"

"$PY_BIN" - "$TMP/task.json" <<'PY'
import json,sys
json.dump({'input':'Create the file MORNING_APPROVAL_TARGET.txt in the workspace containing exactly this line: MORNING_APPROVAL_MARKER_OK. Then read the file back and confirm its exact contents in your final answer.'},open(sys.argv[1],'w',encoding='utf-8'))
PY
H="$("$CURL_BIN" -sS --connect-timeout 5 --max-time 10 -o "$TMP/start.json" -w '%{http_code}' -b "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/task.json" "$BASE/mod/app/task/start")"
[[ "$H" == 202 ]] || { echo "M7_REAL_APPROVAL_FAIL: task start HTTP $H" >&2; cat "$TMP/start.json" >&2; exit 1; }

DENIED=0
ALLOWED=0
PASS=0
for sec in $(seq 1 "$WAIT_SECONDS"); do
  kill -0 "$SERVER_PID" 2>/dev/null || { echo 'M7_REAL_APPROVAL_FAIL: backend exited' >&2; exit 1; }
  "$CURL_BIN" -fsS --connect-timeout 2 --max-time 3 -b "$COOKIE" "$BASE/mod/live/history?limit=512" > "$TMP/live.json" || true
  "$CURL_BIN" -fsS --connect-timeout 2 --max-time 3 -b "$COOKIE" "$BASE/mod/budget" > "$TMP/after.json" || true

  ERR="$("$PY_BIN" - "$TMP/live.json" <<'PY'
import json,sys
live=json.load(open(sys.argv[1],encoding='utf-8'))
for e in live.get('events',[]):
    d=e.get('data') if isinstance(e.get('data'),dict) else {}
    if d.get('cancelled') is True or d.get('canceled') is True:
        print('turn cancelled'); raise SystemExit(0)
    err=str(d.get('error','')).strip()
    if err and not err.startswith('blocked'):
        print(err[:300]); raise SystemExit(0)
print('')
PY
)"
  if [[ -n "$ERR" ]]; then echo "M7_REAL_APPROVAL_FAIL: $ERR" >&2; exit 1; fi

  # Hand approvals in order: first DENY, subsequent ALLOW.
  TARGET=""
  if [[ "$DENIED" == 0 ]]; then TARGET=false; else TARGET=true; fi
  "$PY_BIN" - "$TMP/live.json" "$DENIED" "$ALLOWED" "$TARGET" <<'PY'
import json,sys
live=json.load(open(sys.argv[1],encoding='utf-8')); denied=int(sys.argv[2]); allowed=int(sys.argv[3]); target=sys.argv[4]
approvals=[e for e in live.get('events',[]) if isinstance(e,dict) and str(e.get('type','')).lower()=='live.approval.requested']
handled=denied+allowed
if len(approvals)>handled:
    e=approvals[handled]
    print(f"HELD\t{e['data'].get('id','')}\t{target}")
else:
    print('NONE')
PY
  HELD="$("$PY_BIN" - "$TMP/live.json" "$DENIED" "$ALLOWED" <<'PY'
import json,sys
live=json.load(open(sys.argv[1],encoding='utf-8')); denied=int(sys.argv[2]); allowed=int(sys.argv[3])
approvals=[e for e in live.get('events',[]) if isinstance(e,dict) and str(e.get('type','')).lower()=='live.approval.requested']
handled=denied+allowed
if len(approvals)>handled:
    e=approvals[handled]; print(e['data'].get('id','')); raise SystemExit(0)
print('')
PY
)"
  if [[ -n "$HELD" ]]; then
    if [[ "$DENIED" == 0 ]]; then
      echo "  [M7] DENY approval id=$HELD"
      "$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 -b "$COOKIE" -H 'Content-Type: application/json' \
        -d "{\"id\":\"$HELD\",\"allow\":false}" "$BASE/approve" -o /dev/null || true
      DENIED=1
      if [[ -f "$MUT" ]]; then
        echo 'M7_REAL_APPROVAL_FAIL: file existed after DENY (blind mutation)' >&2
        exit 1
      fi
      echo '  [M7] denied; file absent; agent continues'
    else
      echo "  [M7] ALLOW approval id=$HELD"
      "$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 -b "$COOKIE" -H 'Content-Type: application/json' \
        -d "{\"id\":\"$HELD\",\"allow\":true}" "$BASE/approve" -o /dev/null || true
      ALLOWED=1
    fi
  fi

  DONE="$("$PY_BIN" - "$TMP/live.json" "$DENIED" <<'PY'
import json,sys
live=json.load(open(sys.argv[1],encoding='utf-8')); denied=int(sys.argv[2])
for e in live.get('events',[]):
    if str(e.get('type','')).lower()=='live.turn.done':
        d=e.get('data') if isinstance(e.get('data'),dict) else {}
        err=str(d.get('error','')).strip()
        if err and not err.startswith('blocked'):
            print('ERR\t'+err[:300]); raise SystemExit(0)
        print('DONE'); raise SystemExit(0)
print('WAIT')
PY
)"
  case "$DONE" in
    DONE)
      if [[ "$DENIED" == 1 && "$ALLOWED" == 1 && -f "$MUT" ]]; then
        PASS=1
      fi
      break ;;
    ERR$'\t'*) echo "M7_REAL_APPROVAL_FAIL: ${DONE#*$'\t'}" >&2; exit 1 ;;
  esac
  sleep 1
done
[[ "$PASS" == 1 ]] || { echo "M7_REAL_APPROVAL_FAIL: no clean deny->allow->mutation in ${WAIT_SECONDS}s (denied=$DENIED allowed=$ALLOWED file=$([[ -f $MUT ]] && echo yes || echo no))" >&2; exit 1; }

grep -q 'MORNING_APPROVAL_MARKER_OK' "$MUT" || { echo 'M7_REAL_APPROVAL_FAIL: wrong file content' >&2; cat "$MUT" >&2; exit 1; }
echo "  [M7] read-back OK: $(cat "$MUT")"

"$PY_BIN" - "$TMP/live.json" <<'PY'
import json,sys
live=json.load(open(sys.argv[1],encoding='utf-8'))
texts=[e['data'].get('text','') for e in live.get('events',[]) if isinstance(e.get('data'),dict) and str(e.get('type','')).lower()=='live.chat.message']
nonempty=[t for t in texts if t and t.strip()]
assert nonempty, 'no final assistant message'
assert 'MORNING_APPROVAL_MARKER_OK' in ' '.join(nonempty), f'final answer missing marker: {nonempty!r}'
print(f"M7_FINAL={nonempty[-1]!r}")
PY

"$PY_BIN" - "$TMP/after.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8')); b=p['budget']
assert float(b['spentKzt'])>0, b
print(f"M7_BUDGET spentKzt={b['spentKzt']}")
PY
[[ -s "$TMP/provider-usage.json" ]] || { echo 'M7_REAL_APPROVAL_FAIL: no usage receipt' >&2; exit 1; }

echo 'M7_REAL_APPROVAL_PASS'
