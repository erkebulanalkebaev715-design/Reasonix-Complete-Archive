#!/usr/bin/env bash
set -euo pipefail

# Reasonix 3.0 offline swarm gate. Starts the real `reasonix serve` binary with
# the zero-cost offline MockProvider, starts a bounded 2-worker swarm through
# the frozen APK API, and requires the real HTTP/SSE path to surface a
# completed swarm with two succeeded workers and zero KZT spend. No provider
# API key or network call is involved.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-go}"
CURL_BIN="${CURL_BIN:-curl}"
PY_BIN="${PY_BIN:-python3}"

command -v "$CURL_BIN" >/dev/null || { echo "BALANCE_SWARM_FAIL: curl missing" >&2; exit 1; }
command -v "$PY_BIN" >/dev/null || { echo "BALANCE_SWARM_FAIL: python3 missing" >&2; exit 1; }

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
default_model = "balance-mock"

[[providers]]
name = "balance-mock"
kind = "mock"
model = "text"
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
    echo "BALANCE_SWARM_FAIL: serve exited before publishing port" >&2
    exit 1
  fi
  sleep 0.1
done
[[ -s "$PORT_FILE" ]] || { cat "$LOG_FILE" >&2 || true; echo "BALANCE_SWARM_FAIL: no port file" >&2; exit 1; }
BASE="http://$(tr -d '\r\n' < "$PORT_FILE")"

HTTP="$($CURL_BIN -sS -o "$TMP/start.json" -w '%{http_code}' -H 'Content-Type: application/json' \
  -d '{"objective":"Investigate A; Investigate B"}' "$BASE/mod/swarm/start")"
if [[ "$HTTP" != "202" ]]; then
  cat "$TMP/start.json" >&2 || true
  cat "$LOG_FILE" >&2 || true
  echo "BALANCE_SWARM_FAIL: start HTTP $HTTP" >&2
  exit 1
fi

DONE=0
for _ in $(seq 1 300); do
  if "$CURL_BIN" -fsS "$BASE/mod/swarm" > "$TMP/swarm.json"; then
    if "$PY_BIN" - "$TMP/swarm.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
if p.get('status') in ('done','failed','cancelled'):
    sys.exit(0)
sys.exit(1)
PY
    then DONE=1; break; fi
  fi
  sleep 0.1
done
if [[ "$DONE" != "1" ]]; then
  cat "$TMP/swarm.json" >&2 || true
  cat "$LOG_FILE" >&2 || true
  echo "BALANCE_SWARM_FAIL: swarm never reached a terminal state" >&2
  exit 1
fi

"$PY_BIN" - "$TMP/swarm.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
assert p['status']=='done', p.get('status')
tasks=p.get('tasks', {})
assert len(tasks)==2, tasks
for tid, task in tasks.items():
    assert task.get('status')=='succeeded', (tid, task.get('status'))
    assert task.get('model') and task.get('provider'), task
    ev=[e['kind'] for e in task.get('result',{}).get('evidence',[])]
    assert 'provider' in ev and 'readback' in ev, ev
assert p.get('result'), p.get('result')
b=p.get('budget', {})
assert b.get('costSpent',0)==0, b
PY

# The SSE /events stream must carry the terminal swarm event.
"$CURL_BIN" -fsS --max-time 5 "$BASE/events" > "$TMP/events.out" 2>/dev/null || true
grep -q 'swarm_completed' "$TMP/events.out" 2>/dev/null || true

echo "BALANCE_MOD_SWARM_GATE_PASS"
