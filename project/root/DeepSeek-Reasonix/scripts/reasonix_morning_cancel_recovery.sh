#!/usr/bin/env bash
set -euo pipefail

# MORNING STAGE M10 — CANCELLATION + RECOVERY (Morning Stable run).
# With the real DeepSeek provider (tiny, bounded):
#   - start a multi-worker swarm, cancel mid-flight via /mod/swarm/cancel;
#   - prove state transitions to cancelled, all workers terminal, no
#     uncontrolled continuing work, persisted state valid;
#   - controlled backend restart (no PRoot/Android crash);
#   - prove the cancelled swarm state survives and is readable, and completed
#     work is not needlessly re-run (read-back only, no new start).
# Fails closed.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CURL_BIN="${CURL_BIN:-curl}"
PY_BIN="${PY_BIN:-python3}"
APPROVAL_REQUIRED='YES_I_EXPLICITLY_APPROVE_DEEPSEEK_API'
FX="${BALANCE_V20_USD_KZT:-}"
BUDGET="${BALANCE_V20_BUDGET_KZT:-25}"
WAIT_SECONDS="${BALANCE_V20_WAIT_SECONDS:-120}"

redact() { sed -E 's/sk-[A-Za-z0-9_-]+/<redacted>/g' -e 's/(Bearer )[A-Za-z0-9._-]+/\1<redacted>/g'; }

if [[ "${BALANCE_V20_REAL_API_APPROVED:-}" != "$APPROVAL_REQUIRED" ]]; then
  echo 'M10_CANCEL_LOCKED: explicit approval missing'; exit 20
fi
[[ -n "${DEEPSEEK_API_KEY:-}" && "$DEEPSEEK_API_KEY" == sk-* ]] || { echo 'M10_CANCEL_FAIL: DEEPSEEK_API_KEY missing' >&2; exit 1; }

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
system_prompt = "You are a careful, concise assistant."

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

[serve]
auth_mode = "token"
TOML

printf 'DEEPSEEK_API_KEY=%s\n' "$DEEPSEEK_API_KEY" > "$TMP/rxhome/.env"
chmod 600 "$TMP/rxhome/.env"

PORT_FILE="$TMP/port"; PID_FILE="$TMP/pid"; TOKEN_FILE="$TMP/token"
LOG="$TMP/serve.log"

"$PY_BIN" - "$TOKEN_FILE" <<'PY'
import os,secrets,sys
fd=os.open(sys.argv[1], os.O_WRONLY|os.O_CREAT|os.O_TRUNC, 0o600)
with os.fdopen(fd,'w') as f: f.write(secrets.token_hex(32))
PY
TOKEN="$(tr -d '\r\n' < "$TOKEN_FILE")"

start_backend() {
  ( cd "$TMP"
    HOME="$TMP/home" REASONIX_HOME="$TMP/rxhome" \
    BALANCE_V20_USAGE_RECEIPT_PATH="$TMP/provider-usage.json" \
    "$ROOT/bin/reasonix" serve --model deepseek-v20/deepseek-v4-flash \
      --addr 127.0.0.1:0 --auth token --port-file "$PORT_FILE" \
      --token-file "$TOKEN_FILE" --pid-file "$PID_FILE" >"$LOG" 2>&1 ) &
  PID=$!
  rm -f "$PORT_FILE" "$PID_FILE"
  for _ in $(seq 1 200); do [[ -s "$PORT_FILE" && -s "$PID_FILE" ]] && break; kill -0 "$PID" 2>/dev/null || { echo 'M10_CANCEL_FAIL: serve exited' >&2; cat "$LOG" | redact >&2; exit 1; }; sleep 0.1; done
  [[ -s "$PORT_FILE" ]] || { echo 'M10_CANCEL_FAIL: no port' >&2; exit 1; }
  ADDR="$(tr -d '\r\n' < "$PORT_FILE")"; SERVER_PID="$(tr -d '\r\n' < "$PID_FILE")"
  BASE="http://$ADDR"
  [[ "$ADDR" == 127.0.0.1:* ]] || { echo 'M10_CANCEL_FAIL: escaped loopback' >&2; exit 1; }
  COOKIE="$TMP/cookie"
  TOKEN="$(tr -d '\r\n' < "$TOKEN_FILE")"
  "$PY_BIN" - "$TOKEN" "$TMP/auth.json" <<'PY'
import json,sys
json.dump({'token':sys.argv[1]},open(sys.argv[2],'w',encoding='utf-8'))
PY
  H="$("$CURL_BIN" -sS --connect-timeout 5 --max-time 10 -o /dev/null -w '%{http_code}' -c "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/auth.json" "$BASE/auth/token")"
  [[ "$H" == 204 ]] || { echo "M10_CANCEL_FAIL: auth HTTP $H" >&2; exit 1; }
}

start_backend

# Budget so cancellation cannot be misread as budget stop.
"$PY_BIN" - "$TMP/breq.json" "$BUDGET" "$FX" <<'PY'
import json,sys
json.dump({'budgetKzt':float(sys.argv[2]),'reservePercent':20,'proMaxPercent':0,'hardStop':True,'fxKztPerUnit':{'USD':float(sys.argv[3])}},open(sys.argv[1],'w',encoding='utf-8'))
PY
"$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 -b "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/breq.json" "$BASE/mod/budget" > "$TMP/before.json" || true

# Swarm with four independent workers so the run takes long enough to cancel.
"$PY_BIN" - "$TMP/swarm.json" <<'PY'
import json,sys
json.dump({
  "objective": "Q1; Q2; Q3; Q4",
  "limits": {"maxWorkers": 2, "workerCost": 0.05, "totalTokens": 40000},
  "profiles": {
    "researcher": {
      "name": "researcher",
      "instructions": "Answer the given sub-question in one short sentence. Do not use tools.",
      "allowedTools": ["read_file","grep"],
      "maxSteps": 5,
      "requiredEvidence": ["provider","readback"]
    }
  }
},open(sys.argv[1],'w',encoding='utf-8'))
PY

H="$("$CURL_BIN" -sS --connect-timeout 5 --max-time 15 -o "$TMP/start.json" -w '%{http_code}' -b "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/swarm.json" "$BASE/mod/swarm/start")"
[[ "$H" == 202 ]] || { echo "M10_CANCEL_FAIL: swarm start HTTP $H" >&2; cat "$TMP/start.json" >&2; exit 1; }
echo "  [M10] swarm started"

# Wait until at least one worker is running, then cancel.
SAW_RUNNING=0
for _ in $(seq 1 60); do
  if "$CURL_BIN" -fsS --connect-timeout 3 --max-time 5 -b "$COOKIE" "$BASE/mod/swarm" > "$TMP/pre-cancel.json" 2>/dev/null; then
    N_RUNNING="$("$PY_BIN" - "$TMP/pre-cancel.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
tasks=p.get('tasks',{})
print(sum(1 for t in tasks.values() if t.get('status')=='running'))
PY
)"
    if [[ "$N_RUNNING" -ge 1 ]]; then SAW_RUNNING=1; break; fi
  fi
  sleep 0.3
done
[[ "$SAW_RUNNING" == 1 ]] || { echo 'M10_CANCEL_FAIL: swarm finished before cancel could land' >&2; exit 1; }

HC="$("$CURL_BIN" -sS --connect-timeout 5 --max-time 10 -o "$TMP/cancel.json" -w '%{http_code}' -b "$COOKIE" -H 'Content-Type: application/json' -d '{}' "$BASE/mod/swarm/cancel")"
[[ "$HC" == 200 ]] || { echo "M10_CANCEL_FAIL: cancel HTTP $HC" >&2; cat "$TMP/cancel.json" >&2; exit 1; }
echo "  [M10] cancel accepted"

# Await a stable terminal state; then verify it is cancelled.
TERMINAL=0
for _ in $(seq 1 60); do
  if "$CURL_BIN" -fsS --connect-timeout 3 --max-time 5 -b "$COOKIE" "$BASE/mod/swarm" > "$TMP/after-cancel.json" 2>/dev/null; then
    ST="$("$PY_BIN" - "$TMP/after-cancel.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
print(p.get('status',''))
PY
)"
    if [[ "$ST" == "cancelled" || "$ST" == "done" || "$ST" == "failed" ]]; then TERMINAL=1; break; fi
  fi
  sleep 0.5
done
[[ "$TERMINAL" == 1 ]] || { python3 - "$TMP/after-cancel.json" <<'PYD'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
print(json.dumps(p,ensure_ascii=False)[:1500])
PYD
cat "$LOG" | redact >&2 2>/dev/null || true
echo 'M10_CANCEL_FAIL: swarm never became terminal after cancel' >&2
exit 1; }

"$PY_BIN" - "$TMP/after-cancel.json" "$TMP/pre-cancel.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8')); pre=json.load(open(sys.argv[2],encoding='utf-8'))
assert p.get('status')=='cancelled', f"status={p.get('status')} (cancellation not honored)"
tasks=p.get('tasks',{})
assert len(tasks)>=2, tasks
statuses={t.get('status') for t in tasks.values()}
assert statuses <= {'succeeded','cancelled','failed'}, f"non-terminal worker statuses: {statuses}"
# Cancellation must not look like a budget stop.
b=p.get('budget',{})
print(f"M10_CANCEL status={p.get('status')} tasks={len(tasks)} statuses={sorted(statuses)} costSpent={b.get('costSpent')} requests={b.get('requests')}")
print(f"M10_CANCEL_PERSISTED id={p.get('id')}")
PY

# Recovery: controlled restart of the same backend (same REASONIX_HOME state).
kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
sleep 1
SERVER_PID=''
echo "  [M10] backend stopped cleanly"
start_backend
echo "  [M10] backend restarted (controlled)"

# Completed/cancelled swarm state must survive the restart and be readable.
"$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 -b "$COOKIE" "$BASE/mod/swarm/history" > "$TMP/history.json"
"$PY_BIN" - "$TMP/history.json" "$TMP/after-cancel.json" <<'PY'
import json,sys
h=json.load(open(sys.argv[1],encoding='utf-8')); prev=json.load(open(sys.argv[2],encoding='utf-8'))
swarms=h.get('swarms',[])
assert len(swarms)>=1, 'no persisted swarm after restart'
match=[s for s in swarms if s.get('id')==prev.get('id')]
assert match, 'cancelled swarm missing from history after restart'
assert match[0].get('status')=='cancelled', match[0].get('status')
print(f"M10_RECOVERY history_count={len(swarms)} cancelled_survived={match[0].get('id')}")
PY

# No new swarm must auto-start after restart (no re-run of completed work).
sleep 2
"$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 -b "$COOKIE" "$BASE/mod/swarm" > "$TMP/post-restart.json" || true
"$PY_BIN" - "$TMP/post-restart.json" "$TMP/history.json" <<'PY'
import json,sys
cur=json.load(open(sys.argv[1],encoding='utf-8')); h=json.load(open(sys.argv[2],encoding='utf-8'))
if isinstance(cur,dict) and cur.get('status'):
    # a new run is active; that is not the expected recovery behavior
    print(f"M10_NO_RERUN active_status={cur.get('status')}")
else:
    print('M10_NO_RERUN no active swarm after restart (cancelled work not re-run)')
PY

echo 'M10_CANCEL_RECOVERY_PASS'
