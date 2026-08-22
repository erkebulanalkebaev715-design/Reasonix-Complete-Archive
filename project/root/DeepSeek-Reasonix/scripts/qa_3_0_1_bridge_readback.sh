#!/usr/bin/env bash
# QA 3.0.1 bridge read-back gate: workflows, system prompt, projects, retry/rewind route, web capability.
# Local only — no paid provider calls.
set -u
BACKEND_DIR="${REASONIX_MOBILE_BACKEND:-/root/reasonix-mobile-v1.5.1-backend}"
BASE="http://127.0.0.1:37914"
TOKEN=$(bash "$BACKEND_DIR/reasonix_mobile_backend.sh" token 2>/dev/null | tail -1)
if [ -z "$TOKEN" ]; then echo "FAIL no bridge token"; exit 1; fi
H="X-Reasonix-Mobile-Token: $TOKEN"
jqget(){ python3 -c "import json,sys;d=json.load(sys.stdin);print(d$1)" 2>/dev/null; }
fail=0
ok(){ echo "PASS $1"; }
bad(){ echo "FAIL $1 :: $2"; fail=1; }

# workflows read-back
r=$(curl -s -m 60 -X POST -H "$H" -H "Content-Type: application/json" -d '{"rounds":true}' "$BASE/mobile/workflows")
e=$(echo "$r" | jqget "['workflows']['rounds']['enabled']")
[ "$e" = "True" ] && ok "workflow rounds ON read-back" || bad "workflow rounds ON" "$e"
r=$(curl -s -m 60 -X POST -H "$H" -H "Content-Type: application/json" -d '{"rounds":false}' "$BASE/mobile/workflows")
e=$(echo "$r" | jqget "['workflows']['rounds']['enabled']")
[ "$e" = "False" ] && ok "workflow rounds OFF read-back" || bad "workflow rounds OFF" "$e"
r=$(curl -s -m 10 -H "$H" "$BASE/mobile/workflows")
e=$(echo "$r" | jqget "['workflows']['verify']['enabled']")
[ "$e" = "False" ] && ok "workflow verify read-back stable" || bad "workflow verify" "$e"

# system prompt round-trip (multi-line)
orig=$(curl -s -m 8 -H "$H" "$BASE/mobile/system-prompt" | jqget "['prompt']")
r=$(curl -s -m 60 -X POST -H "$H" -H "Content-Type: application/json" -d '{"prompt":"QA301_MARKER\nsecond line"}' "$BASE/mobile/system-prompt")
read=$(curl -s -m 8 -H "$H" "$BASE/mobile/system-prompt" | jqget "['prompt']")
if [ "$read" = "QA301_MARKER
second line" ]; then ok "system prompt multi-line save/read-back"; else bad "system prompt round-trip" "$read"; fi
# restore
python3 -c "import json,sys;print(json.dumps({'prompt':sys.argv[1]}))" "$orig" >/tmp/qa301_restore.json
curl -s -m 60 -X POST -H "$H" -H "Content-Type: application/json" -d @/tmp/qa301_restore.json "$BASE/mobile/system-prompt" >/dev/null
restored=$(curl -s -m 8 -H "$H" "$BASE/mobile/system-prompt" | jqget "['prompt']")
[ "$restored" = "$orig" ] && ok "system prompt restore" || bad "system prompt restore" "${restored:0:40}"

# projects read-back
r=$(curl -s -m 8 -H "$H" "$BASE/mobile/projects")
n=$(echo "$r" | python3 -c "import json,sys;print(len(json.load(sys.stdin).get('projects') or []))" 2>/dev/null || echo 0)
ok "projects read-back ($n)"

# web capability truthful
r=$(curl -s -m 8 -H "$H" "$BASE/mod/capabilities")
has=$(echo "$r" | python3 -c "import json,sys;print(any(c['name']=='web_fetch' for c in json.load(sys.stdin).get('capabilities',[])))")
[ "$has" = "True" ] && ok "web_fetch capability present (Search web is real)" || bad "web_fetch capability" "$has"

# retry /rewind route reachable through bridge (upstream answer, not bridge 404)
code=$(curl -s -m 8 -o /dev/null -w "%{http_code}" -X POST -H "$H" -H "Content-Type: application/json" -d '{"turn":0,"scope":"conversation"}' "$BASE/rewind")
if [ "$code" != "404" ]; then ok "bridge forwards /rewind (HTTP $code)"; else bad "/rewind forwarding" "$code"; fi

echo "bridge read-back gate fail=$fail"
exit $fail
