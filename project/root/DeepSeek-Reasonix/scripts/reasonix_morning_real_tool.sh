#!/usr/bin/env bash
set -euo pipefail

# MORNING STAGE M6 — REAL TOOL TEST (Morning Stable run).
# Proves: user -> Agent -> ToolDispatch -> ToolResult -> provider continuation
# -> final answer, with the REAL DeepSeek provider and REAL tools (bash read
# only, disposable workspace). Fails closed.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-go}"
CURL_BIN="${CURL_BIN:-curl}"
PY_BIN="${PY_BIN:-python3}"
APPROVAL_REQUIRED='YES_I_EXPLICITLY_APPROVE_DEEPSEEK_API'
FX="${BALANCE_V20_USD_KZT:-}"
BUDGET="${BALANCE_V20_BUDGET_KZT:-20}"
WAIT_SECONDS="${BALANCE_V20_WAIT_SECONDS:-120}"

redact() { sed -E 's/sk-[A-Za-z0-9_-]+/<redacted>/g' -e 's/(Bearer )[A-Za-z0-9._-]+/\1<redacted>/g'; }

if [[ "${BALANCE_V20_REAL_API_APPROVED:-}" != "$APPROVAL_REQUIRED" ]]; then
  echo 'M6_REAL_TOOL_LOCKED: explicit approval missing'; exit 20
fi
[[ -n "${DEEPSEEK_API_KEY:-}" && "$DEEPSEEK_API_KEY" == sk-* ]] || { echo 'M6_REAL_TOOL_FAIL: DEEPSEEK_API_KEY missing' >&2; exit 1; }

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
system_prompt = "You are a careful assistant. Use the available tools to answer the question accurately and briefly."

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

# Disposable fixture the agent must read with a REAL tool (placed in the serve
# process working directory, which is the default session workspace).
cat > "$TMP/REASONIX_TOOL_FIXTURE.txt" <<'FIX'
MORNING_REAL_TOOL_MARKER: Reasonix real tool dispatch works.
FIX

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

for _ in $(seq 1 200); do [[ -s "$PORT_FILE" && -s "$PID_FILE" ]] && break; kill -0 "$PID" 2>/dev/null || { echo 'M6_REAL_TOOL_FAIL: serve exited' >&2; cat "$LOG" | redact >&2; exit 1; }; sleep 0.1; done
[[ -s "$PORT_FILE" ]] || { echo 'M6_REAL_TOOL_FAIL: no port' >&2; exit 1; }
ADDR="$(tr -d '\r\n' < "$PORT_FILE")"; SERVER_PID="$(tr -d '\r\n' < "$PID_FILE")"
BASE="http://$ADDR"; TOKEN="$(tr -d '\r\n' < "$TOKEN_FILE")"
[[ "$ADDR" == 127.0.0.1:* ]] || { echo 'M6_REAL_TOOL_FAIL: escaped loopback' >&2; exit 1; }

COOKIE="$TMP/cookie"
"$PY_BIN" - "$TOKEN" "$TMP/auth.json" <<'PY'
import json,sys
json.dump({'token':sys.argv[1]},open(sys.argv[2],'w',encoding='utf-8'))
PY
H="$("$CURL_BIN" -sS --connect-timeout 5 --max-time 10 -o /dev/null -w '%{http_code}' -c "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/auth.json" "$BASE/auth/token")"
[[ "$H" == 204 ]] || { echo "M6_REAL_TOOL_FAIL: auth HTTP $H" >&2; exit 1; }

"$PY_BIN" - "$TMP/breq.json" "$BUDGET" "$FX" <<'PY'
import json,sys
json.dump({'budgetKzt':float(sys.argv[2]),'reservePercent':20,'proMaxPercent':0,'hardStop':True,'fxKztPerUnit':{'USD':float(sys.argv[3])}},open(sys.argv[1],'w',encoding='utf-8'))
PY
"$CURL_BIN" -fsS --connect-timeout 5 --max-time 10 -b "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/breq.json" "$BASE/mod/budget" > "$TMP/before.json"

"$PY_BIN" - "$TMP/task.json" <<'PY'
import json,sys
json.dump({'input':'Read the file REASONIX_TOOL_FIXTURE.txt in the workspace and report the exact marker text it contains.'},open(sys.argv[1],'w',encoding='utf-8'))
PY
H="$("$CURL_BIN" -sS --connect-timeout 5 --max-time 10 -o "$TMP/start.json" -w '%{http_code}' -b "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/task.json" "$BASE/mod/app/task/start")"
[[ "$H" == 202 ]] || { echo "M6_REAL_TOOL_FAIL: task start HTTP $H" >&2; cat "$TMP/start.json" >&2; exit 1; }

PASS=0
for sec in $(seq 1 "$WAIT_SECONDS"); do
  kill -0 "$SERVER_PID" 2>/dev/null || { echo 'M6_REAL_TOOL_FAIL: backend exited' >&2; exit 1; }
  "$CURL_BIN" -fsS --connect-timeout 2 --max-time 3 -b "$COOKIE" "$BASE/mod/live/history?limit=400" > "$TMP/live.json" || true
  "$CURL_BIN" -fsS --connect-timeout 2 --max-time 3 -b "$COOKIE" "$BASE/mod/budget" > "$TMP/after.json" || true
  STATE="$("$PY_BIN" scripts/balance_mod_v020_completion_check.py "$TMP/live.json" 2>/dev/null || true)"
  case "$STATE" in
    ERROR$'\t'*) echo "M6_REAL_TOOL_FAIL: ${STATE#*$'\t'}" >&2; exit 1 ;;
  esac
  if [[ "$STATE" == DONE$'\t'* ]]; then
    "$PY_BIN" - "$TMP/live.json" "$TMP/after.json" <<'PY'
import json,sys
live=json.load(open(sys.argv[1],encoding='utf-8')); after=json.load(open(sys.argv[2],encoding='utf-8'))
events=live.get('events',[])
tool_finished=[e for e in events if isinstance(e.get('data'),dict) and str(e.get('type','')).lower()=='live.tool.finished']
assert tool_finished, 'no live.tool.finished event -> no real tool ran'
texts=[e['data'].get('text','') for e in events if isinstance(e.get('data'),dict) and str(e.get('type','')).lower()=='live.chat.message']
nonempty=[t for t in texts if t and t.strip()]
assert nonempty, 'no final assistant message'
if 'MORNING_REAL_TOOL_MARKER' not in ' '.join(nonempty):
    print("M6_DEBUG_TOOL_EVENTS:", json.dumps(tool_finished, ensure_ascii=False)[:1500], file=sys.stderr)
    print("M6_DEBUG_FINAL:", json.dumps(nonempty, ensure_ascii=False), file=sys.stderr)
    raise SystemExit('assistant did not report the tool-read marker')
b=after['budget']; assert float(b['spentKzt'])>0, b
print(f"M6_TOOL_FINISHED={len(tool_finished)} final={nonempty[-1]!r} spentKzt={b['spentKzt']}")
PY
    PASS=1
    break
  fi
  sleep 1
done
[[ "$PASS" == 1 ]] || { echo "M6_REAL_TOOL_FAIL: no clean completion in ${WAIT_SECONDS}s" >&2; exit 1; }

[[ -s "$TMP/provider-usage.json" ]] || { echo 'M6_REAL_TOOL_FAIL: no provider usage receipt' >&2; exit 1; }
"$PY_BIN" - "$TMP/provider-usage.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1],encoding='utf-8'))
assert 'deepseek-v4-flash' in str(p.get('modelRef','')).lower(), p
assert int(p.get('requestCount',0))>=1, p
print(f"M6_USAGE_RECEIPT model={p.get('modelRef')} prompt={p.get('promptTokens')} completion={p.get('completionTokens')} requests={p.get('requestCount')} exact={p.get('estimated') is False}")
PY

echo 'M6_REAL_TOOL_PASS'
