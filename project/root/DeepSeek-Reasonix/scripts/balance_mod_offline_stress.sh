#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-go}"
CURL_BIN="${CURL_BIN:-curl}"
PY_BIN="${PY_BIN:-python3}"

command -v "$CURL_BIN" >/dev/null || { echo "BALANCE_OFFLINE_STRESS_FAIL: curl missing" >&2; exit 1; }
command -v "$PY_BIN" >/dev/null || { echo "BALANCE_OFFLINE_STRESS_FAIL: python3 missing" >&2; exit 1; }

cd "$ROOT"
if [[ ! -x bin/reasonix ]]; then
  mkdir -p bin
  GOTOOLCHAIN=local CGO_ENABLED=0 "$GO_BIN" build -o bin/reasonix ./cmd/reasonix
fi

TMP="$(mktemp -d)"
WORK="$TMP/workspace"
RXHOME="$TMP/reasonix-home"
SHELLHOME="$TMP/shell-home"
mkdir -p "$WORK" "$RXHOME" "$SHELLHOME"
printf 'print("STRESS OK")\n' > "$WORK/hello.py"
cat > "$WORK/reasonix.toml" <<'TOML'
default_model = "balance-mock-deny"

[[providers]]
name = "balance-mock-deny"
kind = "mock"
model = "deny-bypass"
base_url = "http://127.0.0.1"
context_window = 1000000
price = { cache_hit = 0, input = 0, output = 0, currency = "KZT" }

[environment]
offline = true

[sandbox]
bash = "off"
TOML

WRAPPER_PID=""
PORT_FILE="$TMP/serve.port"
PID_FILE="$TMP/serve.pid"
LOG_FILE="$TMP/serve.log"
BASE=""

stop_server() {
  local signal="${1:-TERM}"
  local server_pid=""
  if [[ -s "$PID_FILE" ]]; then
    server_pid="$(tr -d '\r\n' < "$PID_FILE" 2>/dev/null || true)"
  fi
  if [[ "$server_pid" =~ ^[0-9]+$ ]] && kill -0 "$server_pid" 2>/dev/null; then
    if [[ "$signal" == "KILL" ]]; then
      kill -9 "$server_pid" 2>/dev/null || true
    else
      kill "$server_pid" 2>/dev/null || true
    fi
  fi
  if [[ -n "$WRAPPER_PID" ]]; then
    wait "$WRAPPER_PID" 2>/dev/null || true
  fi
  WRAPPER_PID=""
}

cleanup() {
  stop_server TERM || true
  rm -rf "$TMP"
}
trap cleanup EXIT

start_server() {
  rm -f "$PORT_FILE" "$PID_FILE"
  : > "$LOG_FILE"
  (
    cd "$WORK"
    env -u DEEPSEEK_API_KEY -u ANTHROPIC_API_KEY -u OPENAI_API_KEY \
      HOME="$SHELLHOME" REASONIX_HOME="$RXHOME" \
      "$ROOT/bin/reasonix" serve --model balance-mock-deny --addr 127.0.0.1:0 --auth none \
        --port-file "$PORT_FILE" --pid-file "$PID_FILE" >>"$LOG_FILE" 2>&1
  ) &
  WRAPPER_PID=$!

  for _ in $(seq 1 120); do
    if [[ -s "$PORT_FILE" ]]; then
      BASE="http://$(tr -d '\r\n' < "$PORT_FILE")"
      if "$CURL_BIN" -fsS "$BASE/mod/app/contract" >/dev/null 2>&1; then
        return 0
      fi
    fi
    if ! kill -0 "$WRAPPER_PID" 2>/dev/null; then
      cat "$LOG_FILE" >&2 || true
      echo "BALANCE_OFFLINE_STRESS_FAIL: backend exited before readiness" >&2
      exit 1
    fi
    sleep 0.1
  done
  cat "$LOG_FILE" >&2 || true
  echo "BALANCE_OFFLINE_STRESS_FAIL: backend readiness timeout" >&2
  exit 1
}

post_json() {
  local path="$1"
  local body="$2"
  "$CURL_BIN" -fsS -H 'Content-Type: application/json' -d "$body" "$BASE$path"
}

wait_idle() {
  for _ in $(seq 1 160); do
    if "$CURL_BIN" -fsS "$BASE/mod/app/bootstrap" > "$TMP/bootstrap-poll.json"; then
      if "$PY_BIN" - "$TMP/bootstrap-poll.json" <<'PY' >/dev/null 2>&1
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
raise SystemExit(0 if not p.get('running', False) else 1)
PY
      then
        return 0
      fi
    fi
    sleep 0.1
  done
  cat "$LOG_FILE" >&2 || true
  echo "BALANCE_OFFLINE_STRESS_FAIL: backend did not become idle" >&2
  exit 1
}

start_server

echo "[stress 1/8] Contract and isolated zero-key backend"
"$CURL_BIN" -fsS "$BASE/mod/app/contract" > "$TMP/contract.json"
"$PY_BIN" - "$TMP/contract.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
assert p['protocolVersion']=='balance-apk-v1'
assert p['guarantees']['hiddenReasoningExported'] is False
assert p['guarantees']['budgetEnforcedInBackend'] is True
assert p['guarantees']['apkNeverAuthoritativeForSafety'] is True
PY

echo "[stress 2/8] Native deny survives capability-proxy bypass attempt"
post_json /mod/app/apply '{
  "profile":{"name":"Offline Stress","mode":"agent","toolPacks":["developer"],"liveDetail":"project"},
  "budget":{"budgetKzt":50,"reservePercent":15,"proMaxPercent":25,"hardStop":true,"fxKztPerUnit":{"KZT":1}},
  "toolDecisions":{"write_file":"deny"},
  "approvalMode":"ask"
}' > "$TMP/applied.json"
"$CURL_BIN" -fsS "$BASE/mod/capabilities" > "$TMP/capabilities.json"
"$PY_BIN" - "$TMP/capabilities.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
by={x['name']:x for x in p['capabilities']}
w=by['write_file']
assert w['manualDecision']=='deny', w
assert w['decision']=='deny', w
assert w['providerVisible'] is False, w
PY
HTTP="$($CURL_BIN -sS -o "$TMP/deny-start.out" -w '%{http_code}' -H 'Content-Type: application/json' \
  -d '{"input":"Attempt the offline denied-write bypass stress scenario."}' "$BASE/mod/app/task/start")"
[[ "$HTTP" == "202" ]] || { cat "$TMP/deny-start.out" >&2; cat "$LOG_FILE" >&2; echo "BALANCE_OFFLINE_STRESS_FAIL: denied-write task start HTTP $HTTP" >&2; exit 1; }
PASS=0
for _ in $(seq 1 180); do
  if "$CURL_BIN" -fsS "$BASE/mod/live/history?limit=160" > "$TMP/live-deny.json" && grep -q 'OFFLINE_DENY_NATIVE_PASS' "$TMP/live-deny.json"; then
    PASS=1
    break
  fi
  sleep 0.1
done
[[ "$PASS" == "1" ]] || { cat "$TMP/live-deny.json" >&2 || true; cat "$LOG_FILE" >&2; echo "BALANCE_OFFLINE_STRESS_FAIL: native deny/proxy bypass gate was not observed" >&2; exit 1; }
[[ ! -e "$WORK/should_not_exist.txt" ]] || { echo "BALANCE_OFFLINE_STRESS_FAIL: denied writer created a file" >&2; exit 1; }
wait_idle

echo "[stress 3/8] Queue budget is clipped by workspace KZT ceiling"
post_json /mod/queue/pause '{}' >/dev/null
post_json /mod/queue/items '{"input":"Queued offline stress item","idempotencyKey":"stress-queue-1","taskBudget":{"budgetKzt":500,"tokenLimit":1000,"wallSeconds":60}}' > "$TMP/enqueue.json"
"$PY_BIN" - "$TMP/enqueue.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
b=p['taskBudget']
assert b['requestedKzt']==500, b
assert b['effectiveKzt']==50, b
assert b['providerCurrency']=='KZT', b
assert b['providerCostLimit']==50, b
PY
"$CURL_BIN" -fsS "$BASE/mod/queue" > "$TMP/queue-before.json"
QUEUE_ID="$($PY_BIN - "$TMP/queue-before.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
assert p['paused'] is True, p
assert p['count'] >= 1, p
for x in p['items']:
    if x.get('preview') == 'Queued offline stress item':
        print(x['id']); break
else:
    raise SystemExit('queued item missing')
PY
)"
[[ -n "$QUEUE_ID" ]] || { echo "BALANCE_OFFLINE_STRESS_FAIL: queue id missing" >&2; exit 1; }

echo "[stress 4/8] Rollback without checkpoint fails closed"
HTTP="$($CURL_BIN -sS -o "$TMP/rollback.out" -w '%{http_code}' -H 'Content-Type: application/json' -d '{}' "$BASE/mod/recovery/rollback-last")"
[[ "$HTTP" == "409" ]] || { cat "$TMP/rollback.out" >&2 || true; echo "BALANCE_OFFLINE_STRESS_FAIL: rollback status $HTTP, want 409" >&2; exit 1; }

echo "[stress 5/8] Crash/restart preserves APK policy and paused durable queue"
post_json /mod/app/persistence/save '{}' >/dev/null
"$CURL_BIN" -fsS "$BASE/mod/tasks" > "$TMP/tasks-before.json"
SESSION_PATH="$($PY_BIN - "$TMP/tasks-before.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
cur=[x for x in p.get('tasks',[]) if x.get('current')]
if not cur:
    raise SystemExit('current session missing')
print(cur[0]['path'])
PY
)"
[[ -n "$SESSION_PATH" ]] || { echo "BALANCE_OFFLINE_STRESS_FAIL: current session path missing" >&2; exit 1; }
stop_server KILL
start_server
RESUME_BODY="$("$PY_BIN" - "$SESSION_PATH" <<'PY'
import json, sys
print(json.dumps({'path':sys.argv[1]}))
PY
)"
RESUMED=0
for _ in $(seq 1 60); do
  HTTP="$($CURL_BIN -sS -o "$TMP/resume.out" -w '%{http_code}' -H 'Content-Type: application/json' -d "$RESUME_BODY" "$BASE/resume")"
  if [[ "$HTTP" == "204" ]]; then
    RESUMED=1
    break
  fi
  # A killed previous process can leave a lease that is still being observed;
  # only transient conflict is retried. Other responses are real failures.
  if [[ "$HTTP" != "409" ]]; then
    cat "$TMP/resume.out" >&2 || true
    echo "BALANCE_OFFLINE_STRESS_FAIL: resume HTTP $HTTP" >&2
    exit 1
  fi
  sleep 0.1
done
[[ "$RESUMED" == "1" ]] || { cat "$TMP/resume.out" >&2 || true; echo "BALANCE_OFFLINE_STRESS_FAIL: stale lease did not clear after killed backend" >&2; exit 1; }
"$CURL_BIN" -fsS "$BASE/mod/app/bootstrap" > "$TMP/bootstrap-after.json"
"$CURL_BIN" -fsS "$BASE/mod/capabilities" > "$TMP/cap-after.json"
"$CURL_BIN" -fsS "$BASE/mod/queue" > "$TMP/queue-after.json"
"$PY_BIN" - "$TMP/bootstrap-after.json" "$TMP/cap-after.json" "$TMP/queue-after.json" "$QUEUE_ID" <<'PY'
import json, sys
b=json.load(open(sys.argv[1], encoding='utf-8'))
c=json.load(open(sys.argv[2], encoding='utf-8'))
q=json.load(open(sys.argv[3], encoding='utf-8'))
qid=sys.argv[4]
assert b['profile']['mode']=='agent', b['profile']
assert b['budget']['budgetKzt']==50, b['budget']
assert b['budget']['spentKzt']==0, b['budget']
by={x['name']:x for x in c['capabilities']}
assert by['write_file']['manualDecision']=='deny', by['write_file']
assert by['write_file']['providerVisible'] is False, by['write_file']
assert q['paused'] is True, q
assert any(x['id']==qid for x in q['items']), q
PY

echo "[stress 6/8] Corrupt APK state is reported, not fatal"
stop_server TERM
STATE_FILE="$(find "$RXHOME/balance/apk-state" -maxdepth 1 -type f -name '*.json' -print -quit 2>/dev/null || true)"
[[ -n "$STATE_FILE" ]] || { echo "BALANCE_OFFLINE_STRESS_FAIL: persisted APK state file missing" >&2; exit 1; }
cp "$STATE_FILE" "$STATE_FILE.good"
printf '{broken-json' > "$STATE_FILE"
start_server
"$CURL_BIN" -fsS "$BASE/mod/app/persistence" > "$TMP/persistence-corrupt.json"
"$PY_BIN" - "$TMP/persistence-corrupt.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
assert p['enabled'] is True, p
assert isinstance(p.get('lastError'), str) and p['lastError'], p
PY
stop_server TERM
mv "$STATE_FILE.good" "$STATE_FILE"
start_server
"$CURL_BIN" -fsS "$BASE/mod/app/bootstrap" > "$TMP/bootstrap-restored.json"
"$PY_BIN" - "$TMP/bootstrap-restored.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))
assert p['profile']['mode']=='agent', p['profile']
assert p['budget']['budgetKzt']==50, p['budget']
assert p['budget']['spentKzt']==0, p['budget']
assert not p['persistence'].get('lastError',''), p['persistence']
PY

echo "[stress 7/8] Invalid budget request is rejected without mutating state"
HTTP="$($CURL_BIN -sS -o "$TMP/bad-budget.out" -w '%{http_code}' -H 'Content-Type: application/json' \
  -d '{"budgetKzt":-1,"reservePercent":15,"proMaxPercent":25,"hardStop":true}' "$BASE/mod/budget")"
[[ "$HTTP" == "400" ]] || { cat "$TMP/bad-budget.out" >&2 || true; echo "BALANCE_OFFLINE_STRESS_FAIL: invalid budget status $HTTP, want 400" >&2; exit 1; }
"$CURL_BIN" -fsS "$BASE/mod/budget" > "$TMP/budget-final.json"
"$PY_BIN" - "$TMP/budget-final.json" <<'PY'
import json, sys
p=json.load(open(sys.argv[1], encoding='utf-8'))['budget']
assert p['budgetKzt']==50, p
assert p['spentKzt']==0, p
PY

echo "[stress 8/8] PASS — crash, corrupt-state and deny-path remained offline"
if grep -R -E 'sk-[A-Za-z0-9_-]{8,}|DEEPSEEK_API_KEY|ANTHROPIC_API_KEY|OPENAI_API_KEY' "$RXHOME/balance" >/dev/null 2>&1; then
  echo "BALANCE_OFFLINE_STRESS_FAIL: provider-key-shaped data leaked into Balance Mod state" >&2
  exit 1
fi

echo "BALANCE_MOD_OFFLINE_STRESS_PASS"
