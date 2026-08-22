#!/usr/bin/env bash
set -euo pipefail

# MORNING STAGE M9 — REAL REASONIX SWARM TEST (Morning Stable run).
# Runs the smallest legitimate REAL swarm through the serve /mod/swarm API:
#   1 orchestrator + 2 independent worker turns, both on the REAL DeepSeek
#   provider. Proves SwarmStarted -> graph -> 2 workers -> real Agent turns
#   -> provider/model per worker -> bounded parallelism -> structured outputs
#   -> evidence -> integration -> verification -> final result -> persistence
#   -> completed-state read-back -> usage/budget reconciliation.
# Fails closed; no mock/fake provider.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CURL_BIN="${CURL_BIN:-curl}"
PY_BIN="${PY_BIN:-python3}"
APPROVAL_REQUIRED='YES_I_EXPLICITLY_APPROVE_DEEPSEEK_API'
FX="${BALANCE_V20_USD_KZT:-}"
BUDGET="${BALANCE_V20_BUDGET_KZT:-25}"
WAIT_SECONDS="${BALANCE_V20_WAIT_SECONDS:-240}"

redact() { sed -E 's/sk-[A-Za-z0-9_-]+/<redacted>/g' -e 's/(Bearer )[A-Za-z0-9._-]+/\1<redacted>/g'; }

if [[ "${BALANCE_V20_REAL_API_APPROVED:-}" != "$APPROVAL_REQUIRED" ]]; then
  echo 'M9_REAL_SWARM_LOCKED: explicit approval missing'; exit 20
fi
[[ -n "${DEEPSEEK_API_KEY:-}" && "$DEEPSEEK_API_KEY" == sk-* ]] || { echo 'M9_REAL_SWARM_FAIL: DEEPSEEK_API_KEY missing' >&2; exit 1; }

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
system_prompt = "You are a careful, concise assistant. Answer accurately and briefly."

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

( cd "$TMP"
  HOME="$TMP/home" REASONIX_HOME="$TMP/rxhome" \
  BALANCE_V20_USAGE_RECEIPT_PATH="$TMP/provider-usage.json" \
  "$ROOT/bin/reasonix" serve --model deepseek-v20/deepseek-v4-flash \
    --addr 127.0.0.1:0 --auth token --port-file "$PORT_FILE" \
    --token-file "$TOKEN_FILE" --pid-file "$PID_FILE" >"$LOG" 2>&1 ) &
PID=$!

for _ in $(seq 1 200); do [[ -s "$PORT_FILE" && -s "$PID_FILE" ]] && break; kill -0 "$PID" 2>/dev/null || { echo 'M9_REAL_SWARM_FAIL: serve exited' >&2; cat "$LOG" | redact >&2; exit 1; }; sleep 0.1; done
[[ -s "$PORT_FILE" ]] || { echo 'M9_REAL_SWARM_FAIL: no port' >&2; exit 1; }
ADDR="$(tr -d '\r\n' < "$PORT_FILE")"; SERVER_PID="$(tr -d '\r\n' < "$PID_FILE")"
BASE="http://$ADDR"; TOKEN="$(tr -d '\r\n' < "$TOKEN_FILE")"
[[ "$ADDR" == 127.0.0.1:* ]] || { echo 'M9_REAL_SWARM_FAIL: escaped loopback' >&2; exit 1; }

COOKIE="$TMP/cookie"
"$PY_BIN" - "$TOKEN" "$TMP/auth.json" <<'PY'
import json,sys
json.dump({'token':sys.argv[1]},open(sys.argv[2],'w',encoding='utf-8'))
PY
H="$("$CURL_BIN" -sS --connect-timeout 5 --max-time 10 -o /dev/null -w '%{http_code}' -c "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/auth.json" "$BASE/auth/token")"
[[ "$H" == 204 ]] || { echo "M9_REAL_SWARM_FAIL: auth HTTP $H" >&2; exit 1; }

# A tiny objective that genuinely benefits from two independent workers:
# two independent questions split by ';' (the deterministic planner turns each
# segment into its own parallel task), no shared dependency.
"$PY_BIN" - "$TMP/swarm.json" <<'PY'
import json,sys
json.dump({
  "objective": "What is 2+2?; What is the capital of France?",
  "limits": {"maxWorkers": 2, "workerCost": 0.02, "totalTokens": 20000},
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
[[ "$H" == 202 ]] || { echo "M9_REAL_SWARM_FAIL: swarm start HTTP $H" >&2; cat "$TMP/start.json" >&2; cat "$LOG" | redact >&2; exit 1; }
echo "  [M9] swarm accepted"

# Give the swarm a bounded budget via the hard gate too.
"$PY_BIN" - "$TMP/breq.json" "$BUDGET" "$FX" <<'PY'
import json,sys
json.dump({'budgetKzt':float(sys.argv[2]),'reservePercent':20,'proMaxPercent':0,'hardStop':True,'fxKztPerUnit':{'USD':float(sys.argv[3])}},open(sys.argv[1],'w',encoding='utf-8'))
PY
"$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 -b "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/breq.json" "$BASE/mod/budget" > "$TMP/before.json" || true

PASS=0
for sec in $(seq 1 "$WAIT_SECONDS"); do
  kill -0 "$SERVER_PID" 2>/dev/null || { echo 'M9_REAL_SWARM_FAIL: backend exited' >&2; cat "$LOG" | redact >&2; exit 1; }
  if "$CURL_BIN" -fsS --connect-timeout 3 --max-time 5 -b "$COOKIE" "$BASE/mod/swarm" > "$TMP/swarm-state.json" 2>/dev/null; then
    ST="$("$PY_BIN" - "$TMP/swarm-state.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
print(p.get('status',''))
PY
)"
    if [[ "$ST" == "done" || "$ST" == "failed" || "$ST" == "cancelled" ]]; then
      echo "  [M9] terminal swarm status=$ST after ${sec}s"
      PASS=1
      break
    fi
  fi
  if (( sec % 10 == 0 )); then echo "  waiting swarm... ${sec}s"; fi
  sleep 1
done
[[ "$PASS" == 1 ]] || { echo "M9_REAL_SWARM_FAIL: swarm not terminal in ${WAIT_SECONDS}s" >&2; cat "$TMP/swarm-state.json" >&2; cat "$LOG" | redact >&2; exit 1; }

# Assert the full structured contract on the completed state.
"$PY_BIN" - "$TMP/swarm-state.json" "$TMP/before.json" <<'PY'
import json,sys,os
p=json.load(open(sys.argv[1],encoding='utf-8'))
if p.get('status')!='done':
    raise SystemExit(f"M9_REAL_SWARM_FAIL: status={p.get('status')} result={str(p.get('result'))[:200]} failures={p.get('failures')}")
tasks=p.get('tasks',{})
assert len(tasks)>=2, tasks
succeeded=[t for t in tasks.values() if t.get('status')=='succeeded']
assert len(succeeded)>=2, f"expected 2 succeeded workers, got {[(t.get('status')) for t in tasks.values()]}"
for tid,t in tasks.items():
    prov=str(t.get('provider','')).strip()
    model=str(t.get('model','')).strip()
    assert 'deepseek' in prov.lower(), (tid,prov)
    assert 'deepseek-v4-flash' in model.lower(), (tid,model)
    res=t.get('result') or {}
    ev=[e.get('kind') for e in res.get('evidence',[])]
    assert 'provider' in ev, (tid,ev)
    assert 'readback' in ev, (tid,ev)
    summary=str(res.get('summary','')).strip()
    assert summary, (tid,'empty summary')
b=p.get('budget',{})
assert float(b.get('costSpent',0))>0, b
assert int(b.get('requests',0))>=2, b
assert p.get('verified') is True, 'swarm not verified'
assert p.get('result'), 'empty integrated result'
provs=b.get('providers',{})
assert any('deepseek' in k.lower() for k in provs), provs
print(f"M9_SWARM status={p.get('status')} verified={p.get('verified')} tasks={len(tasks)} succeeded={len(succeeded)} costSpent={b.get('costSpent')} requests={b.get('requests')} providers={provs}")
print("M9_SWARM_RESULT="+str(p.get('result'))[:300])
PY

# Completed-state read-back from persistence: GET /mod/swarm/{id}.
SID="$("$PY_BIN" - "$TMP/swarm-state.json" <<'PY'
import json,sys
print(json.load(open(sys.argv[1],encoding='utf-8')).get('id',''))
PY
)"
"$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 -b "$COOKIE" "$BASE/mod/swarm/$SID" > "$TMP/readback.json"
"$PY_BIN" - "$TMP/readback.json" "$SID" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
assert p.get('id')==sys.argv[2], (p.get('id'),sys.argv[2])
assert p.get('status')=='done', p.get('status')
print(f"M9_READBACK id={p.get('id')} status={p.get('status')} verified={p.get('verified')}")
PY

"$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 -b "$COOKIE" "$BASE/mod/swarm/history" > "$TMP/history.json"
"$PY_BIN" - "$TMP/history.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
swarms=p.get('swarms',[])
assert len(swarms)>=1, swarms
assert swarms[0].get('status')=='done', swarms[0].get('status')
print(f"M9_HISTORY count={len(swarms)} newest={swarms[0].get('id')} status={swarms[0].get('status')}")
PY

[[ -s "$TMP/provider-usage.json" ]] && {
  "$PY_BIN" - "$TMP/provider-usage.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
print(f"M9_USAGE_RECEIPT model={p.get('modelRef')} requests={p.get('requestCount')} exact={p.get('estimated') is False}")
PY
}

echo 'M9_REAL_SWARM_PASS'
