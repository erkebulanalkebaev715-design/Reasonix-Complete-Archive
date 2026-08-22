#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-go}"
CURL_BIN="${CURL_BIN:-curl}"
PY_BIN="${PY_BIN:-python3}"

command -v "$CURL_BIN" >/dev/null || { echo "BALANCE_OFFLINE_PROTOTYPE_FAIL: curl missing" >&2; exit 1; }
command -v "$PY_BIN" >/dev/null || { echo "BALANCE_OFFLINE_PROTOTYPE_FAIL: python3 missing" >&2; exit 1; }

cd "$ROOT"
if [[ ! -x bin/reasonix ]]; then
  mkdir -p bin
  GOTOOLCHAIN=local CGO_ENABLED=0 "$GO_BIN" build -o bin/reasonix ./cmd/reasonix
fi

TMP="$(mktemp -d)"
PID=""
cleanup() {
  if [[ -s "${PID_FILE:-}" ]]; then
    SERVER_PID="$(tr -d '\r\n' < "$PID_FILE" 2>/dev/null || true)"
    if [[ "$SERVER_PID" =~ ^[0-9]+$ ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
      kill "$SERVER_PID" 2>/dev/null || true
    fi
  fi
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

printf 'print("TEST OK")\n' > "$TMP/hello.py"
cat > "$TMP/reasonix.toml" <<'TOML'
default_model = "balance-mock"

[[providers]]
name = "balance-mock"
kind = "mock"
model = "smoke"
base_url = "http://127.0.0.1"
context_window = 1000000
price = { cache_hit = 0, input = 0, output = 0, currency = "KZT" }

[environment]
offline = true

[sandbox]
bash = "off"
TOML

PORT_FILE="$TMP/serve.port"
PID_FILE="$TMP/serve.pid"
LOG_FILE="$TMP/serve.log"
(
  cd "$TMP"
  env -u DEEPSEEK_API_KEY -u ANTHROPIC_API_KEY -u OPENAI_API_KEY \
    "$ROOT/bin/reasonix" serve --model balance-mock --addr 127.0.0.1:0 --auth none \
      --port-file "$PORT_FILE" --pid-file "$PID_FILE" >"$LOG_FILE" 2>&1
) &
PID=$!

for _ in $(seq 1 100); do
  [[ -s "$PORT_FILE" ]] && break
  if ! kill -0 "$PID" 2>/dev/null; then
    cat "$LOG_FILE" >&2 || true
    echo "BALANCE_OFFLINE_PROTOTYPE_FAIL: serve exited before publishing port" >&2
    exit 1
  fi
  sleep 0.1
done
[[ -s "$PORT_FILE" ]] || { cat "$LOG_FILE" >&2 || true; echo "BALANCE_OFFLINE_PROTOTYPE_FAIL: no port file" >&2; exit 1; }
BASE="http://$(tr -d '\r\n' < "$PORT_FILE")"

"$CURL_BIN" -fsS "$BASE/mod/app/contract" > "$TMP/contract.json"
"$PY_BIN" - "$TMP/contract.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
assert p['protocolVersion']=='balance-apk-v1'
assert p['contractRevision']==1
assert isinstance(p['digest'], str) and len(p['digest'])==64
assert p['guarantees']['hiddenReasoningExported'] is False
assert p['guarantees']['budgetEnforcedInBackend'] is True
PY

"$CURL_BIN" -fsS -H 'Content-Type: application/json' -d '{
  "profile":{"name":"Offline Prototype","mode":"agent","toolPacks":["developer"],"liveDetail":"project"},
  "budget":{"budgetKzt":100,"reservePercent":15,"proMaxPercent":25,"hardStop":true,"fxKztPerUnit":{"KZT":1}},
  "approvalMode":"ask"
}' "$BASE/mod/app/apply" > "$TMP/bootstrap.json"

"$PY_BIN" - "$TMP/bootstrap.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
assert p['profile']['mode']=='agent'
assert p['budget']['budgetKzt']==100
assert p['contract']['protocolVersion']=='balance-apk-v1'
PY

HTTP="$($CURL_BIN -sS -o "$TMP/start.out" -w '%{http_code}' -H 'Content-Type: application/json' \
  -d '{"input":"Run the offline Balance Mod smoke scenario."}' "$BASE/mod/app/task/start")"
if [[ "$HTTP" != "202" ]]; then
  cat "$TMP/start.out" >&2 || true
  cat "$LOG_FILE" >&2 || true
  echo "BALANCE_OFFLINE_PROTOTYPE_FAIL: task start HTTP $HTTP" >&2
  exit 1
fi

PASS=0
for _ in $(seq 1 160); do
  if "$CURL_BIN" -fsS "$BASE/mod/live/history?limit=100" > "$TMP/live.json"; then
    if grep -q 'OFFLINE_MOCK_PASS' "$TMP/live.json"; then PASS=1; break; fi
  fi
  sleep 0.1
done
if [[ "$PASS" != "1" ]]; then
  cat "$TMP/live.json" >&2 || true
  cat "$LOG_FILE" >&2 || true
  echo "BALANCE_OFFLINE_PROTOTYPE_FAIL: mock task did not complete through HTTP/live protocol" >&2
  exit 1
fi

"$CURL_BIN" -fsS "$BASE/mod/budget" > "$TMP/budget.json"
"$PY_BIN" - "$TMP/budget.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
# Mock provider is priced at zero KZT: the end-to-end control path must not
# fabricate spend or need a real provider key.
b=p['budget']
assert b['spentKzt']==0
assert b['budgetKzt']==100
PY

# Chat mode must keep the same backend/session while shrinking mutating tool
# surface. The detailed permission semantics remain covered by Go regression.
"$CURL_BIN" -fsS -H 'Content-Type: application/json' -d '{"name":"Offline Prototype","mode":"chat","toolPacks":["developer"],"liveDetail":"metadata"}' \
  "$BASE/mod/project" > "$TMP/chat.json"
"$CURL_BIN" -fsS "$BASE/mod/project" > "$TMP/project.json"
"$PY_BIN" - "$TMP/project.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
assert p['profile']['mode']=='chat'
assert p['reasoningPolicy']=='hidden-chain-never-exported; use live plans/actions/results'
PY

echo "BALANCE_MOD_OFFLINE_PROTOTYPE_PASS"
