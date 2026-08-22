#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-go}"
CURL_BIN="${CURL_BIN:-curl}"
PY_BIN="${PY_BIN:-python3}"

command -v "$GO_BIN" >/dev/null || { echo "BALANCE_V19_FAIL: go missing" >&2; exit 1; }
command -v "$CURL_BIN" >/dev/null || { echo "BALANCE_V19_FAIL: curl missing" >&2; exit 1; }
command -v "$PY_BIN" >/dev/null || { echo "BALANCE_V19_FAIL: python3 missing" >&2; exit 1; }

cd "$ROOT"
MANIFEST="$ROOT/configs/balance_mod_v019_apk_backend_manifest.json"

echo "[v0.19 1/8] version + APK backend manifest"
grep -Eq 'const balanceModVersion = "balance-mod-v0\.(19|20)"' internal/serve/mod_bridge.go
test -f configs/reasonix.balance.v019.toml
test -f "$MANIFEST"
"$PY_BIN" - "$MANIFEST" <<'PY'
import json,sys
m=json.load(open(sys.argv[1],encoding='utf-8'))
assert m['schema']=='balance-apk-backend-integration-v1', m
assert m['modVersion']=='balance-mod-v0.19', m
assert m['baseline']=='balance-mod-v0.18', m
assert m['apkContract']=='balance-apk-v1', m
assert m['transport']['bind']=='127.0.0.1:0', m
assert m['transport']['authMode']=='token', m
assert m['provider']['kind']=='mock' and m['provider']['apiKeyRequired'] is False, m
assert m['budget']['hardStop'] is True and m['budget']['budgetKzt']==1000, m
assert m['contractMinimums']=={'endpoints':67,'eventTypes':68}, m
PY

echo "[v0.19 2/8] native frozen contract + serve auth regression"
# Existing native tests remain the source of truth; do not duplicate the server.
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^(TestModAPKContract|TestHardBudgetSkipsCosmeticTitleProvider|TestHardPreCallBudgetSurvivesNativeModelSwitch|TestAuth)' -count=1

echo "[v0.19 3/8] Android supervisor contract"
test -f scripts/reasonix_android_backend.sh
bash -n scripts/reasonix_android_backend.sh
for cmd in start stop restart status token log; do
  grep -qi "$cmd" scripts/reasonix_android_backend.sh || {
    echo "BALANCE_V19_FAIL: android supervisor missing command marker: $cmd" >&2
    exit 1
  }
done

echo "[v0.19 4/8] CLI build"
mkdir -p bin
GOTOOLCHAIN=local CGO_ENABLED=0 "$GO_BIN" build -o bin/reasonix ./cmd/reasonix

echo "[v0.19 5/8] token-authenticated loopback backend"
TMP="$(mktemp -d)"
PID=""
SERVER_PID=""
cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
  fi
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

cp configs/reasonix.balance.v019.toml "$TMP/reasonix.toml"
printf 'print("TEST OK")\n' > "$TMP/hello.py"
PORT_FILE="$TMP/serve.port"
TOKEN_FILE="$TMP/serve.token"
PID_FILE="$TMP/serve.pid"
LOG_FILE="$TMP/serve.log"
COOKIE_JAR="$TMP/cookies.txt"

# v0.19 hotfix:
# --token-file is INPUT to reasonix serve.
# Supervisor creates a private random token first; Reasonix reads it.
"$PY_BIN" - "$TOKEN_FILE" <<'PYTOKEN'
import os
import secrets
import sys

path = sys.argv[1]
token = secrets.token_hex(32)

fd = os.open(
    path,
    os.O_WRONLY | os.O_CREAT | os.O_TRUNC,
    0o600,
)
with os.fdopen(fd, "w", encoding="utf-8") as f:
    f.write(token)

os.chmod(path, 0o600)
PYTOKEN
(
  cd "$TMP"
  env -u DEEPSEEK_API_KEY -u ANTHROPIC_API_KEY -u OPENAI_API_KEY -u GEMINI_API_KEY \
    "$ROOT/bin/reasonix" serve --model balance-apk-mock --addr 127.0.0.1:0 --auth token \
      --port-file "$PORT_FILE" --token-file "$TOKEN_FILE" --pid-file "$PID_FILE" \
      >"$LOG_FILE" 2>&1
) &
PID=$!

for _ in $(seq 1 160); do
  if [[ -s "$PORT_FILE" && -s "$PID_FILE" ]]; then break; fi
  if ! kill -0 "$PID" 2>/dev/null; then
    cat "$LOG_FILE" >&2 || true
    echo "BALANCE_V19_FAIL: serve exited before supervisor files were ready" >&2
    exit 1
  fi
  sleep 0.1
done
for f in "$PORT_FILE" "$TOKEN_FILE" "$PID_FILE"; do
  [[ -s "$f" ]] || { cat "$LOG_FILE" >&2 || true; echo "BALANCE_V19_FAIL: missing $(basename "$f")" >&2; exit 1; }
done

ADDR="$(tr -d '\r\n' < "$PORT_FILE")"
[[ "$ADDR" == 127.0.0.1:* ]] || { echo "BALANCE_V19_FAIL: non-loopback serve address: $ADDR" >&2; exit 1; }
BASE="http://$ADDR"
TOKEN="$(tr -d '\r\n' < "$TOKEN_FILE")"
SERVER_PID="$(tr -d '\r\n' < "$PID_FILE")"
[[ "$SERVER_PID" =~ ^[0-9]+$ ]] || { echo "BALANCE_V19_FAIL: bad pid file" >&2; exit 1; }

"$PY_BIN" - "$TOKEN_FILE" "$TOKEN" "$SERVER_PID" <<'PY'
import os,stat,sys
p,token,pid=sys.argv[1:]
st=os.stat(p)
if stat.S_IMODE(st.st_mode) & 0o077:
    raise SystemExit(f"BALANCE_V19_FAIL: token file permissions too broad: {oct(stat.S_IMODE(st.st_mode))}")
if len(token) < 32:
    raise SystemExit("BALANCE_V19_FAIL: generated serve token unexpectedly short")
cmdline=open(f"/proc/{pid}/cmdline","rb").read()
if token.encode() in cmdline:
    raise SystemExit("BALANCE_V19_FAIL: serve token leaked into Reasonix process argv")
environ=open(f"/proc/{pid}/environ","rb").read()
for name in (b'DEEPSEEK_API_KEY=',b'ANTHROPIC_API_KEY=',b'OPENAI_API_KEY=',b'GEMINI_API_KEY='):
    if name in environ:
        raise SystemExit(f"BALANCE_V19_FAIL: provider key env present in offline backend: {name!r}")
PY

UNAUTH="$($CURL_BIN -sS -o "$TMP/unauth.txt" -w '%{http_code}' "$BASE/mod/status")"
[[ "$UNAUTH" == "401" ]] || { echo "BALANCE_V19_FAIL: unauthenticated /mod/status returned HTTP $UNAUTH" >&2; exit 1; }

"$PY_BIN" - "$TOKEN" "$TMP/auth.json" <<'PY'
import json,sys
json.dump({'token':sys.argv[1]},open(sys.argv[2],'w',encoding='utf-8'))
PY
chmod 600 "$TMP/auth.json"
AUTH_HTTP="$($CURL_BIN -sS -o "$TMP/auth.out" -w '%{http_code}' -c "$COOKIE_JAR" \
  -H 'Content-Type: application/json' --data-binary @"$TMP/auth.json" "$BASE/auth/token")"
[[ "$AUTH_HTTP" == "204" ]] || { cat "$TMP/auth.out" >&2 || true; echo "BALANCE_V19_FAIL: token bootstrap HTTP $AUTH_HTTP" >&2; exit 1; }

"$CURL_BIN" -fsS -b "$COOKIE_JAR" "$BASE/mod/status" > "$TMP/status.json"
"$PY_BIN" - "$TMP/status.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
assert p['modVersion'] in ('balance-mod-v0.19','balance-mod-v0.20'), p
for k in ('budget','taskCostGate','features'):
    assert k in p,(k,p)
PY

echo "[v0.19 6/8] frozen contract inventory + authenticated SSE"
"$CURL_BIN" -fsS -b "$COOKIE_JAR" "$BASE/mod/app/contract" > "$TMP/contract.json"
"$PY_BIN" scripts/balance_mod_v019_contract_probe.py "$TMP/contract.json" "$MANIFEST"

# Open the authenticated SSE stream first. A quiet stream may not flush
# response headers until the first event, so actively generate a harmless
# budget.updated event instead of treating "no bytes in 1 second" as failure.
rm -f "$TMP/events.headers" "$TMP/events.body" "$TMP/events.stderr"

set +e
"$CURL_BIN" -sS -N --max-time 3 \
  -D "$TMP/events.headers" \
  -o "$TMP/events.body" \
  -b "$COOKIE_JAR" \
  "$BASE/mod/events" \
  2>"$TMP/events.stderr" &
SSE_PID=$!
set -e

sleep 0.2

# Generate a real typed backend event while the SSE subscriber is connected.
"$CURL_BIN" -fsS \
  -b "$COOKIE_JAR" \
  -H 'Content-Type: application/json' \
  -d '{
    "budgetKzt":1000,
    "reservePercent":15,
    "proMaxPercent":25,
    "hardStop":true,
    "fxKztPerUnit":{"CNY":70}
  }' \
  "$BASE/mod/budget" > "$TMP/sse-trigger-budget.json"

set +e
wait "$SSE_PID"
SSE_RC=$?
set -e

if [[ "$SSE_RC" != "0" && "$SSE_RC" != "28" ]]; then
  cat "$TMP/events.stderr" >&2 || true
  echo "BALANCE_V19_FAIL: SSE request failed rc=$SSE_RC" >&2
  exit 1
fi

grep -Eq '^HTTP/.* 200' "$TMP/events.headers" || {
  cat "$TMP/events.headers" >&2 || true
  echo "BALANCE_V19_FAIL: authenticated /mod/events did not return HTTP 200" >&2
  exit 1
}

grep -qi '^content-type:.*text/event-stream' "$TMP/events.headers" || {
  cat "$TMP/events.headers" >&2 || true
  cat "$TMP/events.stderr" >&2 || true
  echo "BALANCE_V19_FAIL: /mod/events did not negotiate text/event-stream" >&2
  exit 1
}

[[ -s "$TMP/events.body" ]] || {
  cat "$TMP/events.headers" >&2 || true
  cat "$TMP/events.stderr" >&2 || true
  echo "BALANCE_V19_FAIL: SSE connected but budget update produced no event" >&2
  exit 1
}
"$CURL_BIN" -fsS -b "$COOKIE_JAR" "$BASE/mod/resources" > "$TMP/resources.json"

echo "[v0.19 7/8] APK-style hard-budget task + live result"
"$CURL_BIN" -fsS -b "$COOKIE_JAR" -H 'Content-Type: application/json' -d '{
  "budgetKzt":1000,
  "reservePercent":15,
  "proMaxPercent":25,
  "hardStop":true,
  "fxKztPerUnit":{"CNY":70}
}' "$BASE/mod/budget" > "$TMP/budget.json"
"$PY_BIN" - "$TMP/budget.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
g=p['taskCostGate']
assert g['applied'] is True and g['preCall'] is True and g['singleAgent'] is True,g
assert g['currency']=='CNY' and g['providerLimit']>0,g
PY

START_HTTP="$($CURL_BIN -sS -o "$TMP/start.json" -w '%{http_code}' -b "$COOKIE_JAR" \
  -H 'Content-Type: application/json' \
  -d '{"input":"Run the v0.19 authenticated APK backend offline tool-loop and verify hello.py."}' \
  "$BASE/mod/app/task/start")"
[[ "$START_HTTP" == "202" ]] || {
  cat "$TMP/start.json" >&2 || true
  cat "$LOG_FILE" >&2 || true
  echo "BALANCE_V19_FAIL: task start HTTP $START_HTTP" >&2
  exit 1
}

PASS=0
for _ in $(seq 1 240); do
  if "$CURL_BIN" -fsS -b "$COOKIE_JAR" "$BASE/mod/live/history?limit=250" > "$TMP/live.json"; then
    if grep -q 'OFFLINE_MOCK_PASS' "$TMP/live.json"; then PASS=1; break; fi
  fi
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    cat "$LOG_FILE" >&2 || true
    echo "BALANCE_V19_FAIL: serve exited during APK-style task" >&2
    exit 1
  fi
  sleep 0.1
done
[[ "$PASS" == "1" ]] || {
  cat "$TMP/live.json" >&2 || true
  cat "$LOG_FILE" >&2 || true
  echo "BALANCE_V19_FAIL: OFFLINE_MOCK_PASS not observed through authenticated live history" >&2
  exit 1
}

"$CURL_BIN" -fsS -b "$COOKIE_JAR" "$BASE/mod/budget" > "$TMP/budget-after.json"
"$PY_BIN" - "$TMP/budget-after.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
b=p['budget']; g=p['taskCostGate']
assert b['budgetKzt']==1000,b
assert 0 <= b['spentKzt'] < b['budgetKzt'],b
assert g['preCall'] is True,g
PY

grep -q 'TEST OK' "$TMP/hello.py"

echo "[v0.19 8/8] source diff sanity"
git diff --check

echo "BALANCE_MOD_V19_TARGETED_PASS"
