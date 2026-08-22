#!/usr/bin/env bash
set -euo pipefail

APPROVAL_REQUIRED='YES_I_EXPLICITLY_APPROVE_DEEPSEEK_API'
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-go}"
CURL_BIN="${CURL_BIN:-curl}"
PY_BIN="${PY_BIN:-python3}"
MANIFEST="$ROOT/configs/balance_mod_v020_real_provider_manifest.json"
TEMPLATE="$ROOT/configs/reasonix.balance.v020.real.template.toml"
FX="${BALANCE_V20_USD_KZT:-}"
BUDGET="${BALANCE_V20_BUDGET_KZT:-10}"
WAIT_SECONDS="${BALANCE_V20_WAIT_SECONDS:-120}"
API_BASE='https://api.deepseek.com'

redact_stream() {
  sed -E \
    -e 's/(Bearer )[A-Za-z0-9._-]+/\1<redacted>/g' \
    -e 's/sk-[A-Za-z0-9_-]+/<redacted>/g' \
    -e 's/("DEEPSEEK_API_KEY"[[:space:]]*:[[:space:]]*")[^"]+/\1<redacted>/g'
}

diagnostics() {
  set +e
  if [[ -n "${TMP:-}" ]]; then
    for f in models.json balance-api.json doctor.json doctor.err status.json before.json start.json live.json poll-budget.json events.err; do
      if [[ -s "$TMP/$f" ]]; then
        echo "----- redacted $f -----" >&2
        tail -n 160 "$TMP/$f" | redact_stream >&2 || true
      fi
    done
  fi
  if [[ -n "${LOG:-}" && -f "$LOG" ]]; then
    echo '----- backend error/warning scan -----' >&2
    grep -Ein \
      'error|fail|failed|blocked|provider|deepseek|401|402|422|429|500|503|unauthor|insufficient|rate.?limit|overload|timeout|deadline|refused|no such host|tls|certificate|budget' \
      "$LOG" | tail -n 120 | redact_stream >&2 || true
    echo '----- redacted backend log tail -----' >&2
    tail -n 160 "$LOG" | redact_stream >&2 || true
  fi
  set -e
}

fail() {
  echo "BALANCE_V20_REAL_FAIL: $*" >&2
  diagnostics
  exit 1
}

http_reason() {
  case "${1:-}" in
    200) echo 'OK' ;;
    400) echo 'DeepSeek HTTP 400: invalid request format' ;;
    401) echo 'DeepSeek HTTP 401: API key rejected' ;;
    402) echo 'DeepSeek HTTP 402: insufficient account balance' ;;
    422) echo 'DeepSeek HTTP 422: invalid request parameter' ;;
    429) echo 'DeepSeek HTTP 429: rate limit reached' ;;
    500) echo 'DeepSeek HTTP 500: provider internal error' ;;
    503) echo 'DeepSeek HTTP 503: provider overloaded' ;;
    000|'') echo 'DeepSeek network request failed before HTTP response' ;;
    *) echo "DeepSeek unexpected HTTP ${1:-unknown}" ;;
  esac
}

if [[ "${BALANCE_V20_REAL_API_APPROVED:-}" != "$APPROVAL_REQUIRED" ]]; then
  echo 'BALANCE_V20_REAL_GATE_LOCKED: explicit user approval missing'
  exit 20
fi

cd "$ROOT"

echo '[REAL 1/12] validate approval + API key + FX + hard limits'
[[ -n "${DEEPSEEK_API_KEY:-}" ]] || fail 'DEEPSEEK_API_KEY is empty in this shell'
[[ "$DEEPSEEK_API_KEY" == sk-* ]] || fail 'DEEPSEEK_API_KEY has unexpected prefix'
for x in "$GO_BIN" "$CURL_BIN" "$PY_BIN"; do
  command -v "$x" >/dev/null || fail "missing executable: $x"
done
command -v timeout >/dev/null || fail 'timeout executable missing (coreutils)'
echo "  API key=present | length=${#DEEPSEEK_API_KEY} | value hidden"

"$PY_BIN" - "$MANIFEST" "$FX" "$BUDGET" "$WAIT_SECONDS" <<'PY'
import json,sys
from datetime import date
m=json.load(open(sys.argv[1], encoding='utf-8'))
try:
    fx=float(sys.argv[2]); budget=float(sys.argv[3]); wait=int(sys.argv[4])
except Exception:
    raise SystemExit('BALANCE_V20_REAL_FAIL: FX/budget/wait must be numeric')
if fx <= 0:
    raise SystemExit('BALANCE_V20_REAL_FAIL: BALANCE_V20_USD_KZT must be positive')
if not 0 < budget <= float(m['hardGate']['maximumBudgetKzt']):
    raise SystemExit('BALANCE_V20_REAL_FAIL: budget outside hard maximum')
if wait < 30 or wait > 300:
    raise SystemExit('BALANCE_V20_REAL_FAIL: wait must be 30..300 seconds')
age=(date.today()-date.fromisoformat(m['pricingSnapshot']['asOf'])).days
if age < 0 or age > int(m['pricingSnapshot']['maxAgeDays']):
    raise SystemExit(f'BALANCE_V20_REAL_FAIL: pricing snapshot stale ({age} days)')
assert m['provider']['model']=='deepseek-v4-flash'
assert m['provider']['proAllowed'] is False
assert m['hardGate']['proMaxPercent']==0
print(f"  budget={budget:.2f} KZT | USD/KZT={fx:.4f} | Pro=0% | max_output=64 | wait={wait}s")
PY

grep -q 'const balanceModVersion = "balance-mod-v0.20"' internal/serve/mod_bridge.go \
  || fail 'v0.20 marker missing'

echo '[REAL 2/12] build current Reasonix CLI'
mkdir -p bin
PATH="$(dirname "$(command -v "$GO_BIN")"):$PATH" \
  GOTOOLCHAIN=local CGO_ENABLED=0 \
  "$GO_BIN" build -o bin/reasonix ./cmd/reasonix
[[ -x bin/reasonix ]] || fail 'CLI build produced no executable'
echo "  $("$ROOT/bin/reasonix" --version 2>/dev/null | head -n1 || true)"

echo '[REAL 3/12] create isolated Reasonix runtime + credential store'
TMP="$(mktemp -d)"
PID=''
SERVER_PID=''
SSE_PID=''
LOCK_DIR='/tmp/reasonix-v020-real-gate.lock'
LOG="$TMP/serve.log"

cleanup() {
  set +e
  [[ -n "$SSE_PID" ]] && kill "$SSE_PID" 2>/dev/null
  [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null
  [[ -n "$PID" ]] && kill "$PID" 2>/dev/null
  rm -rf "$TMP"
  if [[ -d "$LOCK_DIR" && -f "$LOCK_DIR/pid" && "$(cat "$LOCK_DIR/pid" 2>/dev/null)" == "$$" ]]; then
    rm -rf "$LOCK_DIR"
  fi
}
trap cleanup EXIT INT TERM

if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  OLD_PID="$(cat "$LOCK_DIR/pid" 2>/dev/null || true)"
  if [[ -n "$OLD_PID" ]] && kill -0 "$OLD_PID" 2>/dev/null; then
    fail "another v0.20 real gate is already running (pid=$OLD_PID)"
  fi
  rm -rf "$LOCK_DIR"
  mkdir "$LOCK_DIR" || fail 'cannot create real-gate lock'
fi
echo "$$" > "$LOCK_DIR/pid"

mkdir -p "$TMP/home" "$TMP/rxhome"
chmod 700 "$TMP/home" "$TMP/rxhome"
cp "$TEMPLATE" "$TMP/reasonix.toml"
printf 'DEEPSEEK_API_KEY=%s\n' "$DEEPSEEK_API_KEY" > "$TMP/rxhome/.env"
chmod 600 "$TMP/rxhome/.env"
[[ -s "$TMP/rxhome/.env" ]] || fail 'isolated Reasonix .env was not created'
echo '  REASONIX_HOME/.env=present (0600; secret hidden)'

# Private curl config keeps the Bearer token out of curl argv and diagnostics.
cat > "$TMP/deepseek.curl" <<EOF
silent
show-error
connect-timeout = 8
max-time = 20
header = "Accept: application/json"
header = "Authorization: Bearer $DEEPSEEK_API_KEY"
EOF
chmod 600 "$TMP/deepseek.curl"

echo '[REAL 4/12] zero-generation DeepSeek API preflight (/models + /user/balance)'
set +e
MODELS_HTTP="$("$CURL_BIN" --config "$TMP/deepseek.curl" \
  -o "$TMP/models.json" -w '%{http_code}' "$API_BASE/models")"
MODELS_RC=$?
set -e
[[ "$MODELS_RC" == 0 ]] || fail "DeepSeek /models transport failed (curl rc=$MODELS_RC)"
[[ "$MODELS_HTTP" == 200 ]] || fail "$(http_reason "$MODELS_HTTP")"
"$PY_BIN" - "$TMP/models.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
ids={x.get('id') for x in p.get('data',[]) if isinstance(x,dict)}
if 'deepseek-v4-flash' not in ids:
    raise SystemExit(f"BALANCE_V20_REAL_FAIL: deepseek-v4-flash not listed; models={sorted(x for x in ids if x)}")
print('  /models auth=PASS | deepseek-v4-flash=AVAILABLE')
PY

set +e
BAL_HTTP="$("$CURL_BIN" --config "$TMP/deepseek.curl" \
  -o "$TMP/balance-api.json" -w '%{http_code}' "$API_BASE/user/balance")"
BAL_RC=$?
set -e
[[ "$BAL_RC" == 0 ]] || fail "DeepSeek /user/balance transport failed (curl rc=$BAL_RC)"
[[ "$BAL_HTTP" == 200 ]] || fail "$(http_reason "$BAL_HTTP")"
"$PY_BIN" - "$TMP/balance-api.json" <<'PY'
import json,sys
from decimal import Decimal, InvalidOperation
p=json.load(open(sys.argv[1],encoding='utf-8'))
if p.get('is_available') is not True:
    raise SystemExit('BALANCE_V20_REAL_FAIL: DeepSeek reports API balance unavailable')
positive=[]
for x in p.get('balance_infos',[]):
    try:
        amount=Decimal(str(x.get('total_balance','0')))
    except InvalidOperation:
        continue
    if amount > 0:
        positive.append((x.get('currency','?'), amount))
if not positive:
    raise SystemExit('BALANCE_V20_REAL_FAIL: DeepSeek reports no positive balance')
print('  /user/balance=PASS | positive balance available')
PY

echo '[REAL 5/12] verify Reasonix sees the same provider credential'
set +e
(
  cd "$TMP"
  HOME="$TMP/home" REASONIX_HOME="$TMP/rxhome" \
    timeout 25 "$ROOT/bin/reasonix" doctor --json \
      > "$TMP/doctor.json" 2> "$TMP/doctor.err"
)
DOCTOR_RC=$?
set -e
[[ "$DOCTOR_RC" == 0 ]] || fail "reasonix doctor failed (rc=$DOCTOR_RC)"
"$PY_BIN" - "$TMP/doctor.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
providers=p.get('providers',[])
matches=[x for x in providers if isinstance(x,dict) and x.get('name')=='deepseek-v20']
if not matches:
    raise SystemExit('BALANCE_V20_REAL_FAIL: doctor did not load provider deepseek-v20 from reasonix.toml')
x=matches[0]
if x.get('key_present') is not True:
    raise SystemExit('BALANCE_V20_REAL_FAIL: Reasonix doctor says deepseek-v20 key_present=false')
models=set(x.get('models') or [])
model=x.get('model')
if model: models.add(model)
if 'deepseek-v4-flash' not in models:
    raise SystemExit(f'BALANCE_V20_REAL_FAIL: doctor provider model mismatch: {sorted(models)}')
print('  doctor provider=deepseek-v20 | key_present=true | model=deepseek-v4-flash')
PY

PORT="$TMP/port"
TOKENF="$TMP/token"
PIDF="$TMP/pid"
COOKIE="$TMP/cookie"

"$PY_BIN" - "$TOKENF" <<'PY'
import os,secrets,sys
fd=os.open(sys.argv[1], os.O_WRONLY|os.O_CREAT|os.O_TRUNC, 0o600)
with os.fdopen(fd,'w',encoding='utf-8') as f:
    f.write(secrets.token_hex(32))
os.chmod(sys.argv[1],0o600)
PY

echo '[REAL 6/12] start token-authenticated localhost backend'
(
  cd "$TMP"
  HOME="$TMP/home" REASONIX_HOME="$TMP/rxhome" \
    "$ROOT/bin/reasonix" serve \
      --model deepseek-v20/deepseek-v4-flash \
      --addr 127.0.0.1:0 \
      --auth token \
      --port-file "$PORT" \
      --token-file "$TOKENF" \
      --pid-file "$PIDF" \
      >"$LOG" 2>&1
) &
PID=$!

for i in $(seq 1 200); do
  [[ -s "$PORT" && -s "$PIDF" ]] && break
  kill -0 "$PID" 2>/dev/null || fail 'serve exited during startup'
  if (( i % 50 == 0 )); then echo "  backend starting... $((i/10))s"; fi
  sleep 0.1
done
[[ -s "$PORT" && -s "$PIDF" ]] || fail 'supervisor files were not created'

ADDR="$(tr -d '\r\n' < "$PORT")"
SERVER_PID="$(tr -d '\r\n' < "$PIDF")"
BASE="http://$ADDR"
TOKEN="$(tr -d '\r\n' < "$TOKENF")"
[[ "$ADDR" == 127.0.0.1:* ]] || fail "backend escaped loopback: $ADDR"

"$PY_BIN" - "$TOKEN" "$TMP/auth.json" <<'PY'
import json,sys
json.dump({'token':sys.argv[1]},open(sys.argv[2],'w',encoding='utf-8'))
PY
chmod 600 "$TMP/auth.json"
H="$("$CURL_BIN" -sS --connect-timeout 5 --max-time 10 \
  -o "$TMP/auth.out" -w '%{http_code}' -c "$COOKIE" \
  -H 'Content-Type: application/json' --data-binary @"$TMP/auth.json" \
  "$BASE/auth/token")"
[[ "$H" == 204 ]] || fail "localhost auth HTTP $H"
"$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 \
  -b "$COOKIE" "$BASE/mod/status" > "$TMP/status.json"
"$PY_BIN" - "$TMP/status.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
assert p.get('modVersion')=='balance-mod-v0.20', p
print('  localhost backend/auth/status=PASS')
PY

echo '[REAL 7/12] apply and verify hard pre-call KZT budget'
"$PY_BIN" - "$TMP/breq.json" "$BUDGET" "$FX" <<'PY'
import json,sys
json.dump({
  'budgetKzt':float(sys.argv[2]),
  'reservePercent':20,
  'proMaxPercent':0,
  'hardStop':True,
  'fxKztPerUnit':{'USD':float(sys.argv[3])}
},open(sys.argv[1],'w',encoding='utf-8'))
PY
"$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 \
  -b "$COOKIE" -H 'Content-Type: application/json' \
  --data-binary @"$TMP/breq.json" "$BASE/mod/budget" > "$TMP/before.json"
"$PY_BIN" - "$TMP/before.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
b=p['budget']; g=p['taskCostGate']
assert float(b['spentKzt']) == 0, b
assert g['applied'] and g['preCall'] and g['singleAgent'], g
assert g['currency']=='USD' and float(g['providerLimit']) > 0, g
print(f"  gate=PASS | providerLimit={float(g['providerLimit']):.8f} USD")
PY

echo '[REAL 8/12] open authenticated SSE telemetry'
set +e
"$CURL_BIN" -sS -N --connect-timeout 5 --max-time "$((WAIT_SECONDS + 25))" \
  -b "$COOKIE" "$BASE/mod/events" > "$TMP/events.sse" 2> "$TMP/events.err" &
SSE_PID=$!
set -e
sleep 0.25

echo '[REAL 9/12] submit EXACTLY ONE DeepSeek V4 Flash model task'
"$PY_BIN" - "$TMP/task.json" <<'PY'
import json,sys
# The expected contiguous marker is deliberately NOT present in the prompt,
# preventing a prompt-echo false positive.
json.dump({
  'input':'Reply with the exact concatenation of these two pieces, with no separator and no other text: BALANCE_V20_REAL_ and PROVIDER_OK. Do not use any tool.'
},open(sys.argv[1],'w',encoding='utf-8'))
PY
H="$("$CURL_BIN" -sS --connect-timeout 5 --max-time 10 \
  -o "$TMP/start.json" -w '%{http_code}' -b "$COOKIE" \
  -H 'Content-Type: application/json' --data-binary @"$TMP/task.json" \
  "$BASE/mod/app/task/start")"
[[ "$H" == 202 ]] || fail "task start HTTP $H"
echo '  request accepted HTTP 202 | model submissions=1'

echo '[REAL 10/12] wait for provider result; fail immediately on typed/runtime error'
PASS=0
LAST_SPENT='0'
LOG_SCAN_FROM="$(wc -l < "$LOG" 2>/dev/null || echo 0)"

for sec in $(seq 1 "$WAIT_SECONDS"); do
  kill -0 "$SERVER_PID" 2>/dev/null || fail 'backend exited while waiting for provider'

  "$CURL_BIN" -fsS --connect-timeout 2 --max-time 3 \
    -b "$COOKIE" "$BASE/mod/live/history?limit=400" > "$TMP/live.json" || true
  "$CURL_BIN" -fsS --connect-timeout 2 --max-time 3 \
    -b "$COOKIE" "$BASE/mod/budget" > "$TMP/poll-budget.json" || true

  TERMINAL_ERROR="$("$PY_BIN" - "$TMP/live.json" 2>/dev/null <<'PY' || true
import json,sys
try:
    p=json.load(open(sys.argv[1],encoding='utf-8'))
except Exception:
    raise SystemExit(0)
bad=('error','failed','failure','blocked','denied','timeout','cancelled','canceled')
msgs=[]
def walk(x):
    if isinstance(x,dict):
        typ=' '.join(str(x.get(k,'')) for k in ('type','event','kind','phase','status')).lower()
        if any(b in typ for b in bad):
            msg=' | '.join(str(x.get(k,'')) for k in ('type','event','kind','phase','status','message','reason','error') if x.get(k))
            if msg: msgs.append(msg[:700])
        for v in x.values(): walk(v)
    elif isinstance(x,list):
        for v in x: walk(v)
walk(p)
if msgs:
    print(msgs[-1])
PY
)"
  [[ -z "$TERMINAL_ERROR" ]] || fail "live terminal event: $TERMINAL_ERROR"

  NEW_LOG="$(tail -n "+$((LOG_SCAN_FROM + 1))" "$LOG" 2>/dev/null || true)"
  if printf '%s\n' "$NEW_LOG" | grep -Eqi \
    '(^|[ =])(ERROR|FATAL)([ =:]|$)|provider.*(error|fail)|deepseek.*(401|402|422|429|500|503)|unauthor|insufficient balance|deadline exceeded|no such host|connection refused|tls.*error'; then
    fail 'backend/provider error detected in log'
  fi

  LAST_SPENT="$("$PY_BIN" - "$TMP/poll-budget.json" 2>/dev/null <<'PY' || echo 0
import json,sys
try:
    p=json.load(open(sys.argv[1],encoding='utf-8'))
    print(float(p['budget']['spentKzt']))
except Exception:
    print(0)
PY
)"

  MARKER=0
  [[ -f "$TMP/live.json" ]] && grep -q 'BALANCE_V20_REAL_PROVIDER_OK' "$TMP/live.json" && MARKER=1
  POSITIVE="$("$PY_BIN" - "$LAST_SPENT" <<'PY'
import sys
try: print(1 if float(sys.argv[1]) > 0 else 0)
except Exception: print(0)
PY
)"
  if [[ "$MARKER" == 1 && "$POSITIVE" == 1 ]]; then
    PASS=1
    echo "  provider result observed in ~${sec}s | spentKzt=${LAST_SPENT}"
    break
  fi

  if (( sec % 5 == 0 )); then
    echo "  waiting... ${sec}s | spentKzt=${LAST_SPENT}"
  fi
  sleep 1
done
[[ "$PASS" == 1 ]] || fail "provider result + positive spend not observed within ${WAIT_SECONDS}s"

echo '[REAL 11/12] verify Flash-only path + reconcile provider-reported usage'
"$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 \
  -b "$COOKIE" "$BASE/mod/budget" > "$TMP/after.json"
sleep 0.25
if [[ -n "$SSE_PID" ]]; then
  kill "$SSE_PID" 2>/dev/null || true
  wait "$SSE_PID" 2>/dev/null || true
  SSE_PID=''
fi
! grep -Eqi 'deepseek-v4-pro|deepseek-pro' "$TMP/live.json" "$TMP/events.sse" \
  || fail 'Pro appeared in Flash-only gate'

"$PY_BIN" scripts/balance_mod_v020_reconcile.py \
  --manifest "$MANIFEST" \
  --before "$TMP/before.json" \
  --after "$TMP/after.json" \
  --fx "$FX" \
  "$TMP/live.json" "$TMP/events.sse"

echo '[REAL 12/12] final hard-budget assertion'
"$PY_BIN" - "$TMP/after.json" "$BUDGET" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
b=p['budget']; spent=float(b['spentKzt']); cap=float(sys.argv[2]); remain=float(b['remainingKzt'])
assert 0 < spent <= cap, (spent,cap)
assert remain >= 0, remain
print(f"  spent={spent:.6f} KZT | remaining={remain:.6f} KZT | cap={cap:.2f} KZT")
PY

echo 'BALANCE_MOD_V20_REAL_GATE_PASS'
