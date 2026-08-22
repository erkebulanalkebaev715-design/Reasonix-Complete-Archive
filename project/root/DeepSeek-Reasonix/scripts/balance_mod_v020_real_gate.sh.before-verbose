#!/usr/bin/env bash
set -euo pipefail
APPROVAL_REQUIRED='YES_I_EXPLICITLY_APPROVE_DEEPSEEK_API'
if [[ "${BALANCE_V20_REAL_API_APPROVED:-}" != "$APPROVAL_REQUIRED" ]]; then echo 'BALANCE_V20_REAL_GATE_LOCKED: explicit user approval missing'; exit 20; fi
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"; GO_BIN="${GO_BIN:-go}"; CURL_BIN="${CURL_BIN:-curl}"; PY_BIN="${PY_BIN:-python3}"; cd "$ROOT"
for x in "$GO_BIN" "$CURL_BIN" "$PY_BIN"; do command -v "$x" >/dev/null || { echo "BALANCE_V20_REAL_FAIL: missing $x" >&2; exit 1; }; done
MANIFEST="$ROOT/configs/balance_mod_v020_real_provider_manifest.json"; TEMPLATE="$ROOT/configs/reasonix.balance.v020.real.template.toml"; FX="${BALANCE_V20_USD_KZT:-}"; BUDGET="${BALANCE_V20_BUDGET_KZT:-10}"
"$PY_BIN" - "$MANIFEST" "$FX" "$BUDGET" <<'PY'
import json,sys
from datetime import date
m=json.load(open(sys.argv[1]));
try: fx=float(sys.argv[2]); budget=float(sys.argv[3])
except: raise SystemExit('BALANCE_V20_REAL_FAIL: FX/budget must be numeric')
if fx<=0: raise SystemExit('BALANCE_V20_REAL_FAIL: BALANCE_V20_USD_KZT must be positive')
if not 0<budget<=float(m['hardGate']['maximumBudgetKzt']): raise SystemExit('BALANCE_V20_REAL_FAIL: budget outside hard maximum')
age=(date.today()-date.fromisoformat(m['pricingSnapshot']['asOf'])).days
if age<0 or age>int(m['pricingSnapshot']['maxAgeDays']): raise SystemExit(f'BALANCE_V20_REAL_FAIL: pricing snapshot stale ({age} days)')
PY
grep -q 'const balanceModVersion = "balance-mod-v0.20"' internal/serve/mod_bridge.go || { echo 'BALANCE_V20_REAL_FAIL: v0.20 marker missing' >&2; exit 1; }
mkdir -p bin; PATH="$(dirname "$(command -v "$GO_BIN")"):$PATH" GOTOOLCHAIN=local CGO_ENABLED=0 "$GO_BIN" build -o bin/reasonix ./cmd/reasonix
TMP="$(mktemp -d)"; PID=''; SERVER_PID=''; SSE_PID=''; redact(){ [[ -f "$TMP/serve.log" ]] && tail -n 80 "$TMP/serve.log" | sed -E 's/(Bearer )[A-Za-z0-9._-]+/\1<redacted>/g;s/sk-[A-Za-z0-9_-]+/<redacted>/g' >&2 || true; }; cleanup(){ [[ -n "$SSE_PID" ]] && kill "$SSE_PID" 2>/dev/null||true; [[ -n "$SERVER_PID" ]]&&kill "$SERVER_PID" 2>/dev/null||true; [[ -n "$PID" ]]&&kill "$PID" 2>/dev/null||true; rm -rf "$TMP"; }; trap cleanup EXIT
cp "$TEMPLATE" "$TMP/reasonix.toml"; PORT="$TMP/port"; TOKENF="$TMP/token"; PIDF="$TMP/pid"; COOKIE="$TMP/cookie"; LOG="$TMP/serve.log"
"$PY_BIN" - "$TOKENF" <<'PY'
import os,secrets,sys
fd=os.open(sys.argv[1],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600)
with os.fdopen(fd,'w') as f:f.write(secrets.token_hex(32))
os.chmod(sys.argv[1],0o600)
PY
(cd "$TMP"; "$ROOT/bin/reasonix" serve --model deepseek-v20/deepseek-v4-flash --addr 127.0.0.1:0 --auth token --port-file "$PORT" --token-file "$TOKENF" --pid-file "$PIDF" >"$LOG" 2>&1)& PID=$!
for _ in $(seq 1 200); do [[ -s "$PORT" && -s "$PIDF" ]]&&break; kill -0 "$PID" 2>/dev/null||{ redact; echo 'BALANCE_V20_REAL_FAIL: serve startup failed' >&2; exit 1; }; sleep .1; done
[[ -s "$PORT" && -s "$PIDF" ]]||{ redact; echo 'BALANCE_V20_REAL_FAIL: supervisor files missing' >&2; exit 1; }
ADDR="$(tr -d '\r\n'<"$PORT")"; SERVER_PID="$(tr -d '\r\n'<"$PIDF")"; BASE="http://$ADDR"; TOKEN="$(tr -d '\r\n'<"$TOKENF")"; [[ "$ADDR" == 127.0.0.1:* ]]||exit 1
"$PY_BIN" - "$TOKEN" "$TMP/auth.json" <<'PY'
import json,sys;json.dump({'token':sys.argv[1]},open(sys.argv[2],'w'))
PY
chmod 600 "$TMP/auth.json"; H="$($CURL_BIN -sS -o "$TMP/a" -w '%{http_code}' -c "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/auth.json" "$BASE/auth/token")"; [[ "$H" == 204 ]]||{ echo "BALANCE_V20_REAL_FAIL: auth HTTP $H" >&2; exit 1; }
"$PY_BIN" - "$TMP/breq.json" "$BUDGET" "$FX" <<'PY'
import json,sys;json.dump({'budgetKzt':float(sys.argv[2]),'reservePercent':20,'proMaxPercent':0,'hardStop':True,'fxKztPerUnit':{'USD':float(sys.argv[3])}},open(sys.argv[1],'w'))
PY
"$CURL_BIN" -fsS -b "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/breq.json" "$BASE/mod/budget">"$TMP/before.json"
"$PY_BIN" - "$TMP/before.json" <<'PY'
import json,sys;p=json.load(open(sys.argv[1]));b=p['budget'];g=p['taskCostGate'];assert float(b['spentKzt'])==0;assert g['applied'] and g['preCall'] and g['singleAgent'] and g['currency']=='USD' and float(g['providerLimit'])>0
PY
set +e; "$CURL_BIN" -sS -N --max-time 60 -b "$COOKIE" "$BASE/mod/events">"$TMP/events.sse" 2>"$TMP/events.err"& SSE_PID=$!; set -e; sleep .2
"$PY_BIN" - "$TMP/task.json" <<'PY'
import json,sys;json.dump({'input':'Reply exactly BALANCE_V20_REAL_PROVIDER_OK. Do not use any tool. Do not add any other text.'},open(sys.argv[1],'w'))
PY
H="$($CURL_BIN -sS -o "$TMP/start" -w '%{http_code}' -b "$COOKIE" -H 'Content-Type: application/json' --data-binary @"$TMP/task.json" "$BASE/mod/app/task/start")"; [[ "$H" == 202 ]]||{ redact; echo "BALANCE_V20_REAL_FAIL: task HTTP $H" >&2; exit 1; }
PASS=0; for _ in $(seq 1 900); do if "$CURL_BIN" -fsS -b "$COOKIE" "$BASE/mod/live/history?limit=300">"$TMP/live.json";then grep -q BALANCE_V20_REAL_PROVIDER_OK "$TMP/live.json"&&{ PASS=1;break;};fi; kill -0 "$SERVER_PID" 2>/dev/null||{ redact;exit 1;};sleep .1;done; [[ "$PASS" == 1 ]]||{ redact;echo 'BALANCE_V20_REAL_FAIL: marker not observed' >&2;exit 1;}
"$CURL_BIN" -fsS -b "$COOKIE" "$BASE/mod/budget">"$TMP/after.json"; sleep .5; kill "$SSE_PID" 2>/dev/null||true;wait "$SSE_PID" 2>/dev/null||true;SSE_PID=''
! grep -Eqi 'deepseek-v4-pro|deepseek-pro' "$TMP/live.json" "$TMP/events.sse"||{ echo 'BALANCE_V20_REAL_FAIL: Pro appeared' >&2;exit 1;}
"$PY_BIN" scripts/balance_mod_v020_reconcile.py --manifest "$MANIFEST" --before "$TMP/before.json" --after "$TMP/after.json" --fx "$FX" "$TMP/live.json" "$TMP/events.sse"
"$PY_BIN" - "$TMP/after.json" "$BUDGET" <<'PY'
import json,sys;p=json.load(open(sys.argv[1]));s=float(p['budget']['spentKzt']);c=float(sys.argv[2]);assert 0<s<=c,(s,c)
PY
echo BALANCE_MOD_V20_REAL_GATE_PASS
