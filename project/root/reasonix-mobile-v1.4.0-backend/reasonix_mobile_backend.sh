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
bridge_cmdline(){
  local p="${1:-}"
  [[ "$p" =~ ^[0-9]+$ && -r "/proc/$p/cmdline" ]] || return 1
  tr '\0' ' ' < "/proc/$p/cmdline" 2>/dev/null || true
}
bridge_cmd_owned(){
  local cmd="${1:-}"
  [[ -n "$cmd" ]] || return 1
  # Old releases run from a different absolute directory, so match the script
  # basename/path suffix, then pin ownership to this mobile state or bind addr.
  [[ "$cmd" == *"reasonix_mobile_bridge.py"* ]] || return 1
  if [[ "$cmd" == *" --state $STATE "* || "$cmd" == *" --state=$STATE "* ]]; then return 0; fi
  if [[ "$cmd" == *" --addr $BRIDGE_BIND "* || "$cmd" == *" --addr=$BRIDGE_BIND "* ]]; then return 0; fi
  return 1
}
bridge_pid_ok(){
  local p="${1:-}" cmd=""
  alive "$p" || return 1
  cmd="$(bridge_cmdline "$p" || true)"
  bridge_cmd_owned " $cmd "
}
bridge_url(){
  local a=""; [[ -s "$BPORT" ]] && a="$(readf "$BPORT")" || true
  [[ -n "$a" ]] || a="$BRIDGE_BIND"
  printf 'http://%s' "$a"
}
bridge_process_records(){
  # Do not use /proc/net here. Android 10+ blocks app access to /proc/net,
  # and PRoot exposes Android's /proc, so socket-inode discovery is unreliable.
  python3 - "$STATE" "$BRIDGE_BIND" <<'PYPROC'
import glob,os,sys
state,bind=sys.argv[1:3]
for proc in glob.glob('/proc/[0-9]*'):
    pid=proc.rsplit('/',1)[-1]
    try:
        raw=open(proc+'/cmdline','rb').read().replace(b'\0',b' ').decode('utf-8','replace').strip()
    except OSError:
        continue
    if 'reasonix_mobile_bridge.py' not in raw:
        continue
    owned=(f'--state {state}' in raw or f'--state={state}' in raw or
           f'--addr {bind}' in raw or f'--addr={bind}' in raw)
    if owned:
        print(pid+'\t'+raw)
PYPROC
}
port_bind_probe(){
  python3 - "$BRIDGE_BIND" <<'PYBIND'
import socket,sys
host,port=sys.argv[1].rsplit(':',1); port=int(port)
s=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
try:
    s.bind((host,port))
except OSError as e:
    print(f'busy errno={getattr(e,"errno",None)} error={e}')
    raise SystemExit(1)
finally:
    s.close()
print('free')
PYBIND
}
health_probe(){
  python3 - "$BRIDGE_BIND" <<'PYHEALTH'
import json,sys,urllib.request
addr=sys.argv[1]
try:
    with urllib.request.urlopen('http://'+addr+'/mobile/health',timeout=1.5) as r:
        body=r.read().decode('utf-8','replace')
    try:
        obj=json.loads(body)
        print(json.dumps(obj,ensure_ascii=False,separators=(',',':')))
    except Exception:
        print(body[:500])
except Exception as e:
    print('unreachable:'+repr(e))
PYHEALTH
}
kill_owned_bridge_processes(){
  local p cmd any=0
  while IFS=$'\t' read -r p cmd; do
    [[ -n "${p:-}" ]] || continue
    [[ "$p" != "$$" ]] || continue
    any=1
    echo "Stopping Reasonix mobile bridge pid=$p"
    kill "$p" 2>/dev/null || true
  done < <(bridge_process_records)
  if (( any )); then
    for _ in $(seq 1 80); do
      local left="$(bridge_process_records || true)"
      [[ -z "$left" ]] && break
      sleep .1
    done
    while IFS=$'\t' read -r p cmd; do
      [[ -n "${p:-}" ]] || continue
      echo "Force-killing Reasonix mobile bridge pid=$p"
      kill -9 "$p" 2>/dev/null || true
    done < <(bridge_process_records)
  fi
}
reclaim_orphan_bridge(){
  # First remove every bridge instance belonging to this state/address. This is
  # version/path agnostic, so an old v1.2 bridge is recognized correctly.
  kill_owned_bridge_processes
  rm -f "$BPID" "$BPORT"
  if port_bind_probe >/dev/null 2>&1; then return 0; fi
  echo "ERROR: $BRIDGE_BIND is still occupied after stopping all visible Reasonix mobile bridge processes." >&2
  echo "health_probe=$(health_probe)" >&2
  echo "visible_bridge_processes:" >&2
  bridge_process_records >&2 || true
  echo "NOTE: Android 10+ restricts /proc/net for app UIDs, so port-owner discovery via /proc/net is intentionally not used." >&2
  return 1
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
  if ! reclaim_orphan_bridge; then
    echo "stop incomplete: $BRIDGE_BIND is still occupied" >&2
    return 1
  fi
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
cmd_builtin_pack(){ cmd_api /mobile/builtins/install POST; }
cmd_integration_audit(){ cmd_api /mobile/integration-audit GET; }
cmd_auto_approval(){
  [[ -s "$TOKEN" ]] || { echo "token not ready" >&2; exit 1; }
  python3 - "$TOKEN" "$BPORT" "$BRIDGE_BIND" <<'PYAUTO'
import json,os,sys,urllib.request
_,tf,pf,default_addr=sys.argv
token=open(tf,encoding='utf-8').read().strip()
addr=open(pf,encoding='utf-8').read().strip() if os.path.exists(pf) else default_addr
req=urllib.request.Request('http://'+addr+'/tool-approval-mode',data=json.dumps({'mode':'auto'}).encode(),method='POST',headers={'X-Reasonix-Mobile-Token':token,'Content-Type':'application/json'})
with urllib.request.urlopen(req,timeout=10) as r:
    body=r.read().decode('utf-8','replace').strip()
    print(body or 'TOOL_APPROVAL_MODE_AUTO_PASS')
PYAUTO
}
cmd_port_debug(){
  echo "bind=$BRIDGE_BIND"
  echo '--- bind probe ---'
  port_bind_probe || true
  echo '--- health probe ---'
  health_probe || true
  echo '--- visible Reasonix mobile bridge processes ---'
  bridge_process_records || true
}
case "${1:-status}" in start)cmd_start;; stop)cmd_stop;; restart)cmd_stop;cmd_start;; status)cmd_status;; token)cmd_token;; log)cmd_log;; diagnose)cmd_diagnose;; port-debug)cmd_port_debug;; install-android-tools)cmd_android_tools;; install-builtins)cmd_builtin_pack;; integration-audit)cmd_integration_audit;; approval-auto)cmd_auto_approval;; *)echo "usage: $0 start|stop|restart|status|token|log|diagnose|port-debug|install-android-tools|install-builtins|integration-audit|approval-auto";exit 2;; esac
