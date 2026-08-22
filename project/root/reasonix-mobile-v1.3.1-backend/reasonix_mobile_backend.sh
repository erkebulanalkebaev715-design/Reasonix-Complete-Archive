#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT="${REASONIX_ROOT:-$HOME/DeepSeek-Reasonix}"
BIN="${REASONIX_BIN:-$ROOT/bin/reasonix}"
MODEL="${REASONIX_MOBILE_MODEL:-custom-api-deepseek-com/deepseek-v4-flash}"
STATE="${REASONIX_MOBILE_STATE:-$HOME/.reasonix-mobile-v1}"
BRIDGE_BIND="${REASONIX_MOBILE_BRIDGE_ADDR:-127.0.0.1:37914}"
BRIDGE_PY="${REASONIX_MOBILE_BRIDGE_PY:-$SCRIPT_DIR/reasonix_mobile_bridge.py}"
mkdir -p "$STATE"; chmod 700 "$STATE"
TOKEN="$STATE/serve.token"; BPORT="$STATE/bridge.port"; BPID="$STATE/bridge.pid"; BLOG="$STATE/bridge.log"
readf(){ tr -d '\r\n' < "$1" 2>/dev/null || true; }
alive(){ [[ "${1:-}" =~ ^[0-9]+$ ]] && kill -0 "$1" 2>/dev/null; }
bridge_pid_ok(){
  local p="${1:-}" cmd=""
  alive "$p" || return 1
  [[ -r "/proc/$p/cmdline" ]] || return 1
  cmd="$(tr '\0' ' ' < "/proc/$p/cmdline" 2>/dev/null || true)"
  [[ " $cmd " == *" reasonix_mobile_bridge.py "* || " $cmd " == *" $BRIDGE_PY "* ]]
}
bridge_url(){
  local a=""; [[ -s "$BPORT" ]] && a="$(readf "$BPORT")" || true
  [[ -n "$a" ]] || a="$BRIDGE_BIND"
  printf 'http://%s' "$a"
}
bridge_listener_records(){
  python3 - "$BRIDGE_BIND" <<'PYPORT'
import glob,os,sys
addr=sys.argv[1]
try: port=int(addr.rsplit(':',1)[1])
except Exception: raise SystemExit(2)
inodes=set()
for fn in ('/proc/net/tcp','/proc/net/tcp6'):
    try:
        lines=open(fn,encoding='ascii',errors='ignore').read().splitlines()[1:]
    except OSError:
        continue
    for line in lines:
        f=line.split()
        if len(f)<10 or f[3] != '0A':
            continue
        try: p=int(f[1].rsplit(':',1)[1],16)
        except Exception: continue
        if p == port:
            inodes.add(f[9])
seen=set()
for proc in glob.glob('/proc/[0-9]*'):
    pid=proc.rsplit('/',1)[-1]
    hit=False
    for fd in glob.glob(proc+'/fd/*'):
        try: link=os.readlink(fd)
        except OSError: continue
        if link.startswith('socket:[') and link[8:-1] in inodes:
            hit=True;break
    if not hit or pid in seen: continue
    seen.add(pid)
    try:
        raw=open(proc+'/cmdline','rb').read().replace(b'\0',b' ').decode('utf-8','replace').strip()
    except OSError:
        raw=''
    print(pid+'\t'+raw)
PYPORT
}
reclaim_orphan_bridge(){
  local rec p cmd found=0 owned=0
  while IFS=$'\t' read -r p cmd; do
    [[ -n "${p:-}" ]] || continue
    found=1
    if [[ "$cmd" == *"reasonix_mobile_bridge.py"* ]]; then
      owned=1
      echo "Stopping orphan mobile bridge pid=$p on $BRIDGE_BIND"
      kill "$p" 2>/dev/null || true
      for _ in $(seq 1 60); do alive "$p" || break; sleep .1; done
      alive "$p" && kill -9 "$p" 2>/dev/null || true
    else
      echo "ERROR: $BRIDGE_BIND is occupied by non-Reasonix process pid=$p cmd=$cmd" >&2
    fi
  done < <(bridge_listener_records)
  if (( found && ! owned )); then return 1; fi
  # Port must now be free. If not, refuse to launch and show the real owner.
  rec="$(bridge_listener_records || true)"
  if [[ -n "$rec" ]]; then
    echo "ERROR: $BRIDGE_BIND is still occupied after cleanup:" >&2
    printf '%s\n' "$rec" >&2
    return 1
  fi
}
cmd_start(){
  local p="" bg=""
  [[ -s "$BPID" ]] && p="$(readf "$BPID")" || true
  if bridge_pid_ok "$p"; then echo "already running"; cmd_status; return; fi
  [[ -n "$p" ]] && rm -f "$BPID" "$BPORT"
  rm -f "$BPORT" "$BPID"
  reclaim_orphan_bridge
  if [[ ! -s "$TOKEN" ]]; then
    python3 - "$TOKEN" <<'PYTOKEN'
import os,secrets,sys
p=sys.argv[1];fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600)
with os.fdopen(fd,'w',encoding='utf-8') as f:f.write(secrets.token_hex(32))
os.chmod(p,0o600)
PYTOKEN
  fi
  nohup python3 "$BRIDGE_PY" --root "$ROOT" --binary "$BIN" --state "$STATE" --token-file "$TOKEN" \
    --model "$MODEL" --addr "$BRIDGE_BIND" --port-file "$BPORT" --pid-file "$BPID" >"$BLOG" 2>&1 &
  bg=$!
  for _ in $(seq 1 260); do
    [[ -s "$BPORT" && -s "$BPID" ]] && break
    kill -0 "$bg" 2>/dev/null || { echo "Reasonix Mobile bridge exited" >&2; tail -120 "$BLOG" >&2; exit 1; }
    sleep .1
  done
  [[ -s "$BPORT" && -s "$BPID" ]] || { echo "Reasonix Mobile startup timeout" >&2; tail -120 "$BLOG" >&2; exit 1; }
  chmod 600 "$TOKEN" "$BPORT" "$BPID" 2>/dev/null || true
  echo "REASONIX_MOBILE_BACKEND_STARTED"; cmd_status
}
cmd_stop(){
  local p=""; [[ -s "$BPID" ]] && p="$(readf "$BPID")" || true
  if bridge_pid_ok "$p"; then kill "$p" 2>/dev/null || true; for _ in $(seq 1 60);do alive "$p" || break;sleep .1;done; alive "$p" && kill -9 "$p" 2>/dev/null || true; fi
  rm -f "$BPID" "$BPORT"
  reclaim_orphan_bridge || true
  echo stopped
}
cmd_status(){
  local p="" addr=""; [[ -s "$BPID" ]] && p="$(readf "$BPID")" || true
  if bridge_pid_ok "$p"; then
    addr="$(readf "$BPORT")"; echo "status=running bridge_pid=$p"; [[ -n "$addr" ]] && echo "url=http://$addr"
    [[ -s "$STATE/model" ]] && echo "model=$(readf "$STATE/model")" || echo "model=$MODEL"
    [[ -s "$STATE/reasonix.port" ]] && echo "reasonix_url=http://$(readf "$STATE/reasonix.port")"
    echo "token_file=$TOKEN"
  else echo "status=stopped"; fi
}
cmd_token(){ [[ -s "$TOKEN" ]] || { echo "token not ready" >&2; exit 1; }; cat "$TOKEN"; echo; }
cmd_log(){ echo '--- mobile bridge ---'; tail -n "${LINES:-100}" "$BLOG" 2>/dev/null || true; echo '--- reasonix ---'; tail -n "${LINES:-100}" "$STATE/reasonix.log" 2>/dev/null || true; }
cmd_api(){
  local path="$1" method="${2:-GET}"
  [[ -s "$TOKEN" ]] || { echo "token not ready" >&2; exit 1; }
  python3 - "$path" "$method" "$TOKEN" "$BPORT" "$BRIDGE_BIND" <<'PYAPI'
import json,sys,urllib.request,urllib.error
path,method,tf,pf,default_addr=sys.argv[1:6]
token=open(tf,encoding='utf-8').read().strip()
base=open(pf,encoding='utf-8').read().strip() if __import__('os').path.exists(pf) else default_addr
req=urllib.request.Request('http://'+base+path,data=(b'{}' if method=='POST' else None),method=method,headers={'X-Reasonix-Mobile-Token':token,'Content-Type':'application/json'})
try:
 with urllib.request.urlopen(req,timeout=20) as r: print(r.read().decode('utf-8','replace'))
except urllib.error.HTTPError as e:
 print(e.read().decode('utf-8','replace'),file=sys.stderr);raise SystemExit(1)
PYAPI
}
cmd_diagnose(){ cmd_status; echo '--- health ---'; python3 - <<'PYH'
import urllib.request
import os
state=os.environ.get('REASONIX_MOBILE_STATE',os.path.expanduser('~/.reasonix-mobile-v1'))
p=os.path.join(state,'bridge.port')
addr=open(p,encoding='utf-8').read().strip() if os.path.exists(p) else os.environ.get('REASONIX_MOBILE_BRIDGE_ADDR','127.0.0.1:37914')
try:
 print(urllib.request.urlopen('http://'+addr+'/mobile/health',timeout=5).read().decode())
except Exception as e: print('health_error='+str(e))
PYH
 echo '--- diagnostics ---'; cmd_api /mobile/diagnostics GET; }
cmd_android_tools(){ cmd_api /mobile/android/install-tools POST; }
case "${1:-status}" in start)cmd_start;; stop)cmd_stop;; restart)cmd_stop;cmd_start;; status)cmd_status;; token)cmd_token;; log)cmd_log;; diagnose)cmd_diagnose;; install-android-tools)cmd_android_tools;; *)echo "usage: $0 start|stop|restart|status|token|log|diagnose|install-android-tools";exit 2;; esac
