#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-go}"
CURL_BIN="${CURL_BIN:-curl}"
PY_BIN="${PY_BIN:-python3}"

command -v "$CURL_BIN" >/dev/null || { echo "BALANCE_PRECALL_FAIL: curl missing" >&2; exit 1; }
command -v "$PY_BIN" >/dev/null || { echo "BALANCE_PRECALL_FAIL: python3 missing" >&2; exit 1; }

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

cat > "$TMP/reasonix.toml" <<'TOML'
default_model = "balance-budget-mock"

[[providers]]
name = "balance-budget-mock"
kind = "mock"
model = "budget-cap"
base_url = "http://127.0.0.1"
context_window = 1000000
billing_currency = "CNY"
price = { cache_hit = 0.02, input = 1, output = 2, currency = "CNY" }

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
    "$ROOT/bin/reasonix" serve --model balance-budget-mock --addr 127.0.0.1:0 --auth none \
      --port-file "$PORT_FILE" --pid-file "$PID_FILE" >"$LOG_FILE" 2>&1
) &
PID=$!

for _ in $(seq 1 100); do
  [[ -s "$PORT_FILE" ]] && break
  if ! kill -0 "$PID" 2>/dev/null; then
    cat "$LOG_FILE" >&2 || true
    echo "BALANCE_PRECALL_FAIL: serve exited before publishing port" >&2
    exit 1
  fi
  sleep 0.1
done
[[ -s "$PORT_FILE" ]] || { cat "$LOG_FILE" >&2 || true; echo "BALANCE_PRECALL_FAIL: no port file" >&2; exit 1; }
BASE="http://$(tr -d '\r\n' < "$PORT_FILE")"

# A hard budget without the active provider's FX rate must fail closed before
# any task reaches Provider.Stream. This attacks the configuration-error path,
# not only the happy path.
"$CURL_BIN" -fsS -H 'Content-Type: application/json' -d '{
  "budgetKzt":1000,
  "reservePercent":15,
  "proMaxPercent":25,
  "hardStop":true,
  "fxKztPerUnit":{"USD":500}
}' "$BASE/mod/budget" > "$TMP/budget-no-fx.json"
"$PY_BIN" - "$TMP/budget-no-fx.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
g=p['taskCostGate']
assert g['applied'] is False, g
assert g['preCall'] is True, g
assert 'FX' in g.get('reason','') or 'fx' in g.get('reason',''), g
PY
HTTP="$($CURL_BIN -sS -o "$TMP/no-fx-start.out" -w '%{http_code}' -H 'Content-Type: application/json' \
  -d '{"input":"This must be blocked before provider I/O."}' "$BASE/mod/app/task/start")"
if [[ "$HTTP" != "409" ]]; then
  cat "$TMP/no-fx-start.out" >&2 || true
  echo "BALANCE_PRECALL_FAIL: missing-FX hard budget admitted task with HTTP $HTTP" >&2
  exit 1
fi

# v0.20-fixed9 replaced the obsolete 66-way retry-reserve split with one paid
# attempt per hard-budget admission. The budget must therefore be small enough
# that the affordable output cap alone lands below the mock provider's 128K
# baseline: 15 KZT at FX 70 -> ~0.214 CNY provider limit -> ~107K output tokens.
"$CURL_BIN" -fsS -H 'Content-Type: application/json' -d '{
  "budgetKzt":15,
  "reservePercent":15,
  "proMaxPercent":25,
  "hardStop":true,
  "fxKztPerUnit":{"CNY":70}
}' "$BASE/mod/budget" > "$TMP/budget-set.json"

"$PY_BIN" - "$TMP/budget-set.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
g=p['taskCostGate']
assert g['applied'] is True, g
assert g['preCall'] is True, g
assert g['singleAgent'] is True, g
assert g['currency']=='CNY', g
assert g['providerLimit'] > 0, g
PY

HTTP="$($CURL_BIN -sS -o "$TMP/start.out" -w '%{http_code}' -H 'Content-Type: application/json' \
  -d '{"input":"Run the offline hard pre-call budget probe."}' "$BASE/mod/app/task/start")"
if [[ "$HTTP" != "202" ]]; then
  cat "$TMP/start.out" >&2 || true
  cat "$LOG_FILE" >&2 || true
  echo "BALANCE_PRECALL_FAIL: task start HTTP $HTTP" >&2
  exit 1
fi

PASS=0
for _ in $(seq 1 160); do
  if "$CURL_BIN" -fsS "$BASE/mod/live/history?limit=100" > "$TMP/live.json"; then
    if grep -q 'OFFLINE_PRECALL_BUDGET_PASS' "$TMP/live.json"; then PASS=1; break; fi
    if grep -q 'MOCK_PRECALL_BUDGET_FAIL' "$TMP/live.json"; then
      cat "$TMP/live.json" >&2 || true
      echo "BALANCE_PRECALL_FAIL: provider request was not capped" >&2
      exit 1
    fi
  fi
  sleep 0.1
done
if [[ "$PASS" != "1" ]]; then
  cat "$TMP/live.json" >&2 || true
  cat "$LOG_FILE" >&2 || true
  echo "BALANCE_PRECALL_FAIL: capped mock completion not observed" >&2
  exit 1
fi

"$CURL_BIN" -fsS "$BASE/mod/budget" > "$TMP/budget-after.json"
"$PY_BIN" - "$TMP/budget-after.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
b=p['budget']; g=p['taskCostGate']
assert g['preCall'] is True, g
assert 0 <= b['spentKzt'] < b['budgetKzt'], b
PY

echo "BALANCE_MOD_PRECALL_BUDGET_PASS"
