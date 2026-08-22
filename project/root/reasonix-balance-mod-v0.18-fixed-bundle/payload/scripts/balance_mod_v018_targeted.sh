#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-go}"
CURL_BIN="${CURL_BIN:-curl}"
PY_BIN="${PY_BIN:-python3}"

command -v "$GO_BIN" >/dev/null || { echo "BALANCE_V18_FAIL: go missing" >&2; exit 1; }
command -v "$CURL_BIN" >/dev/null || { echo "BALANCE_V18_FAIL: curl missing" >&2; exit 1; }
command -v "$PY_BIN" >/dev/null || { echo "BALANCE_V18_FAIL: python3 missing" >&2; exit 1; }

cd "$ROOT"

echo "[v0.18 1/6] release markers + canonical config"
grep -q 'const balanceModVersion = "balance-mod-v0.18"' internal/serve/mod_bridge.go
grep -q 'const SchemaVersion = 3' internal/sessioninbox/types.go
grep -q 's.runID = foreignRunID' internal/sessioninbox/completion_receipt_test.go
"$PY_BIN" - <<'PY'
import json
from pathlib import Path
m=json.loads(Path('configs/balance_mod_v018_rc_manifest.json').read_text(encoding='utf-8'))
assert m['modVersion']=='balance-mod-v0.18', m
assert m['baseline']=='balance-mod-v0.17', m
assert m['apkContract']=='balance-apk-v1', m
assert m['provider']['kind']=='mock' and m['provider']['apiKeyRequired'] is False, m
assert m['budget']=={
  'budgetKzt':1000,
  'reservePercent':15,
  'proMaxPercent':25,
  'hardStop':True,
  'fxKztPerUnit':{'CNY':70},
}, m['budget']
PY

echo "[v0.18 2/6] frozen APK + project/live/queue contract regression"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^(TestModAPKContract|TestModProjectChatModeHidesMutatingToolsAndRestoresAgentMode|TestModQueue|TestModLive)' -count=1

echo "[v0.18 3/6] hard pre-call + crash/replay critical regression"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^TestRemainingProviderBudgetDoesNotRegrantSpentMoney$' -count=1
GOTOOLCHAIN=local "$GO_BIN" test ./internal/agent -run '^(TestStrictPreCallBudget|TestProviderRequestTokenUpperBound|TestStrictBudgetBlocks)' -count=1
./scripts/balance_mod_v017_targeted.sh

echo "[v0.18 4/6] CLI build"
mkdir -p bin
GOTOOLCHAIN=local CGO_ENABLED=0 "$GO_BIN" build -o bin/reasonix ./cmd/reasonix

echo "[v0.18 5/6] single-instance localhost APK-style offline RC flow"
TMP="$(mktemp -d)"
PID=""
PID_FILE=""
cleanup() {
  if [[ -n "$PID_FILE" && -s "$PID_FILE" ]]; then
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

cp configs/reasonix.balance.v018.toml "$TMP/reasonix.toml"
cat > "$TMP/hello.py" <<'PYFILE'
print("TEST OK")
PYFILE

PORT_FILE="$TMP/serve.port"
PID_FILE="$TMP/serve.pid"
LOG_FILE="$TMP/serve.log"
(
  cd "$TMP"
  env -u DEEPSEEK_API_KEY -u ANTHROPIC_API_KEY -u OPENAI_API_KEY -u GEMINI_API_KEY \
    "$ROOT/bin/reasonix" serve --model balance-rc-mock --addr 127.0.0.1:0 --auth none \
      --port-file "$PORT_FILE" --pid-file "$PID_FILE" >"$LOG_FILE" 2>&1
) &
PID=$!

for _ in $(seq 1 120); do
  [[ -s "$PORT_FILE" ]] && break
  if ! kill -0 "$PID" 2>/dev/null; then
    cat "$LOG_FILE" >&2 || true
    echo "BALANCE_V18_FAIL: serve exited before publishing port" >&2
    exit 1
  fi
  sleep 0.1
done
[[ -s "$PORT_FILE" ]] || { cat "$LOG_FILE" >&2 || true; echo "BALANCE_V18_FAIL: no port file" >&2; exit 1; }
BASE="http://$(tr -d '\r\n' < "$PORT_FILE")"

"$CURL_BIN" -fsS "$BASE/mod/status" > "$TMP/status.json"
"$PY_BIN" - "$TMP/status.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
assert p['modVersion']=='balance-mod-v0.18', p
for k in ('budget','taskCostGate','features'):
    assert k in p, (k,p)
PY

"$CURL_BIN" -fsS -H 'Content-Type: application/json' -d '{
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
assert g['applied'] is True, g
assert g['preCall'] is True, g
assert g['singleAgent'] is True, g
assert g['currency']=='CNY', g
assert g['providerLimit'] > 0, g
PY

HTTP="$($CURL_BIN -sS -o "$TMP/start.json" -w '%{http_code}' -H 'Content-Type: application/json' \
  -d '{"input":"Run the v0.18 offline RC tool-loop and verify hello.py."}' \
  "$BASE/mod/app/task/start")"
if [[ "$HTTP" != "202" ]]; then
  cat "$TMP/start.json" >&2 || true
  cat "$LOG_FILE" >&2 || true
  echo "BALANCE_V18_FAIL: task start HTTP $HTTP" >&2
  exit 1
fi

PASS=0
for _ in $(seq 1 200); do
  if "$CURL_BIN" -fsS "$BASE/mod/live/history?limit=200" > "$TMP/live.json"; then
    if grep -q 'OFFLINE_MOCK_PASS' "$TMP/live.json"; then
      PASS=1
      break
    fi
  fi
  if ! kill -0 "$PID" 2>/dev/null; then
    cat "$LOG_FILE" >&2 || true
    echo "BALANCE_V18_FAIL: serve exited during RC task" >&2
    exit 1
  fi
  sleep 0.1
done
if [[ "$PASS" != "1" ]]; then
  cat "$TMP/live.json" >&2 || true
  cat "$LOG_FILE" >&2 || true
  echo "BALANCE_V18_FAIL: OFFLINE_MOCK_PASS not observed" >&2
  exit 1
fi

"$CURL_BIN" -fsS "$BASE/mod/budget" > "$TMP/budget-after.json"
"$PY_BIN" - "$TMP/budget-after.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
b=p['budget']; g=p['taskCostGate']
assert g['preCall'] is True, g
assert b['budgetKzt']==1000, b
assert 0 <= b['spentKzt'] < b['budgetKzt'], b
PY

grep -q 'TEST OK' "$TMP/hello.py"

echo "[v0.18 6/6] source diff sanity"
git diff --check

echo "BALANCE_MOD_V18_TARGETED_PASS"
