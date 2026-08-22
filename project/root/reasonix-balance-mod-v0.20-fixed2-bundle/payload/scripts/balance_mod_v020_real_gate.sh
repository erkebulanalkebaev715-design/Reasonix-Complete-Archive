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

fail() {
  echo "BALANCE_V20_REAL_FAIL: $*" >&2
  if [[ -n "${TMP:-}" ]]; then
    for f in start.json live.json poll-budget.json events.err; do
      if [[ -s "$TMP/$f" ]]; then
        echo "----- redacted $f -----" >&2
        tail -n 120 "$TMP/$f" \
          | sed -E 's/(Bearer )[A-Za-z0-9._-]+/\1<redacted>/g;s/sk-[A-Za-z0-9_-]+/<redacted>/g' >&2 || true
      fi
    done
  fi
  if [[ -n "${LOG:-}" && -f "$LOG" ]]; then
    echo '----- redacted backend log (tail) -----' >&2
    tail -n 100 "$LOG" \
      | sed -E 's/(Bearer )[A-Za-z0-9._-]+/\1<redacted>/g;s/sk-[A-Za-z0-9_-]+/<redacted>/g' >&2 || true
  fi
  exit 1
}

if [[ "${BALANCE_V20_REAL_API_APPROVED:-}" != "$APPROVAL_REQUIRED" ]]; then
  echo 'BALANCE_V20_REAL_GATE_LOCKED: explicit user approval missing'
  exit 20
fi

cd "$ROOT"

echo '[REAL 1/10] approval + API-key env + hard limits'
[[ -n "${DEEPSEEK_API_KEY:-}" ]] || fail 'DEEPSEEK_API_KEY is empty'
[[ "$DEEPSEEK_API_KEY" == sk-* ]] || fail 'DEEPSEEK_API_KEY has unexpected prefix'
echo "  API key: present (length=${#DEEPSEEK_API_KEY}, value hidden)"

for x in "$GO_BIN" "$CURL_BIN" "$PY_BIN"; do
  command -v "$x" >/dev/null || fail "missing executable: $x"
done

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
print(f"  budget={budget:.2f} KZT | USD/KZT={fx:.4f} | Pro=0% | max_output=64 | timeout={wait}s")
PY

grep -q 'const balanceModVersion = "balance-mod-v0.20"' internal/serve/mod_bridge.go \
  || fail 'v0.20 marker missing'

echo '[REAL 2/10] build ARM64/Linux Reasonix CLI'
mkdir -p bin
PATH="$(dirname "$(command -v "$GO_BIN")"):$PATH" \
  GOTOOLCHAIN=local CGO_ENABLED=0 \
  "$GO_BIN" build -o bin/reasonix ./cmd/reasonix
[[ -x bin/reasonix ]] || fail 'CLI build produced no executable'

echo '[REAL 3/10] create isolated online runtime'
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
cp "$TEMPLATE" "$TMP/reasonix.toml"

# Current Reasonix resolves provider credentials ONLY from <REASONIX_HOME>/.env.
# Inherited shell variables are deliberately not provider-key runtime fallbacks.
# The gate therefore copies the already user-approved key into this isolated,
# owner-only temporary Reasonix home. cleanup removes the whole TMP tree.
printf 'DEEPSEEK_API_KEY=%s\n' "$DEEPSEEK_API_KEY" > "$TMP/rxhome/.env"
chmod 600 "$TMP/rxhome/.env"
[[ -s "$TMP/rxhome/.env" ]] || fail 'isolated Reasonix credential file was not created'
echo '  provider credential staged in isolated REASONIX_HOME/.env (0600; value hidden)'

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

echo '[REAL 4/10] start token-authenticated localhost backend'
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
echo "  backend=$ADDR"

"$PY_BIN" - "$TOKEN" "$TMP/auth.json" <<'PY'
import json,sys
json.dump({'token':sys.argv[1]},open(sys.argv[2],'w',encoding='utf-8'))
PY
chmod 600 "$TMP/auth.json"
H="$($CURL_BIN -sS --connect-timeout 5 --max-time 10 \
  -o "$TMP/auth.out" -w '%{http_code}' -c "$COOKIE" \
  -H 'Content-Type: application/json' --data-binary @"$TMP/auth.json" \
  "$BASE/auth/token")"
[[ "$H" == 204 ]] || fail "auth HTTP $H"

echo '[REAL 5/10] apply and verify hard pre-call KZT budget'
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
assert g['currency'] == 'USD' and float(g['providerLimit']) > 0, g
print(f"  pre-call gate=PASS | providerLimit={float(g['providerLimit']):.8f} USD")
PY

echo '[REAL 6/10] open authenticated SSE telemetry'
set +e
"$CURL_BIN" -sS -N --connect-timeout 5 --max-time "$((WAIT_SECONDS + 20))" \
  -b "$COOKIE" "$BASE/mod/events" > "$TMP/events.sse" 2> "$TMP/events.err" &
SSE_PID=$!
set -e
sleep 0.2

echo '[REAL 7/10] submit EXACTLY ONE real DeepSeek V4 Flash task'
"$PY_BIN" - "$TMP/task.json" <<'PY'
import json,sys
json.dump({
  'input':'Reply exactly BALANCE_V20_REAL_PROVIDER_OK. Do not use any tool. Do not add any other text.'
},open(sys.argv[1],'w',encoding='utf-8'))
PY
H="$($CURL_BIN -sS --connect-timeout 5 --max-time 10 \
  -o "$TMP/start.json" -w '%{http_code}' -b "$COOKIE" \
  -H 'Content-Type: application/json' --data-binary @"$TMP/task.json" \
  "$BASE/mod/app/task/start")"
[[ "$H" == 202 ]] || {
  echo '----- task-start body -----' >&2
  cat "$TMP/start.json" >&2 || true
  fail "task start HTTP $H"
}
echo '  request accepted (HTTP 202); no second model request will be submitted'

echo '[REAL 8/10] wait for provider completion + positive ledger spend'
PASS=0
LAST_SPENT='0'
for sec in $(seq 1 "$WAIT_SECONDS"); do
  kill -0 "$SERVER_PID" 2>/dev/null || fail 'backend exited while waiting for provider'

  "$CURL_BIN" -fsS --connect-timeout 2 --max-time 3 \
    -b "$COOKIE" "$BASE/mod/live/history?limit=300" > "$TMP/live.json" || true
  "$CURL_BIN" -fsS --connect-timeout 2 --max-time 3 \
    -b "$COOKIE" "$BASE/mod/budget" > "$TMP/poll-budget.json" || true

  LAST_SPENT="$($PY_BIN - "$TMP/poll-budget.json" 2>/dev/null <<'PY' || echo 0
import json,sys
try:
 p=json.load(open(sys.argv[1])); print(float(p['budget']['spentKzt']))
except Exception:
 print(0)
PY
)"

  MARKER=0
  [[ -f "$TMP/live.json" ]] && grep -q 'BALANCE_V20_REAL_PROVIDER_OK' "$TMP/live.json" && MARKER=1
  POSITIVE="$($PY_BIN - "$LAST_SPENT" <<'PY'
import sys
try: print(1 if float(sys.argv[1]) > 0 else 0)
except: print(0)
PY
)"

  if [[ "$MARKER" == 1 && "$POSITIVE" == 1 ]]; then
    PASS=1
    echo "  provider completed in ~${sec}s | spentKzt=${LAST_SPENT}"
    break
  fi

  if (( sec % 5 == 0 )); then
    echo "  waiting... ${sec}s | spentKzt=${LAST_SPENT}"
  fi
  sleep 1
done

[[ "$PASS" == 1 ]] || fail "provider marker + positive spend not observed within ${WAIT_SECONDS}s"

echo '[REAL 9/10] verify Flash-only path + reconcile reported usage/cost'
"$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 \
  -b "$COOKIE" "$BASE/mod/budget" > "$TMP/after.json"
sleep 0.3
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

echo '[REAL 10/10] final hard-budget assertion'
"$PY_BIN" - "$TMP/after.json" "$BUDGET" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
b=p['budget']; spent=float(b['spentKzt']); cap=float(sys.argv[2]); remain=float(b['remainingKzt'])
assert 0 < spent <= cap, (spent,cap)
assert remain >= 0, remain
print(f"  spent={spent:.6f} KZT | remaining={remain:.6f} KZT | cap={cap:.2f} KZT")
PY

echo 'BALANCE_MOD_V20_REAL_GATE_PASS'
