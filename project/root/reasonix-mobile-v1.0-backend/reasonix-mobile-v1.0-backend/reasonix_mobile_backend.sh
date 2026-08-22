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
cmd_start(){
  local p="" bg=""
  [[ -s "$BPID" ]] && p="$(readf "$BPID")" || true
  if alive "$p"; then echo "already running"; cmd_status; return; fi
  rm -f "$BPORT" "$BPID"
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
  if alive "$p"; then kill "$p" 2>/dev/null || true; for _ in $(seq 1 60);do alive "$p" || break;sleep .1;done; alive "$p" && kill -9 "$p" 2>/dev/null || true; fi
  rm -f "$BPID" "$BPORT"; echo stopped
}
cmd_status(){
  local p="" addr=""; [[ -s "$BPID" ]] && p="$(readf "$BPID")" || true
  if alive "$p"; then
    addr="$(readf "$BPORT")"; echo "status=running bridge_pid=$p"; [[ -n "$addr" ]] && echo "url=http://$addr"
    [[ -s "$STATE/model" ]] && echo "model=$(readf "$STATE/model")" || echo "model=$MODEL"
    [[ -s "$STATE/reasonix.port" ]] && echo "reasonix_url=http://$(readf "$STATE/reasonix.port")"
    echo "token_file=$TOKEN"
  else echo "status=stopped"; fi
}
cmd_token(){ [[ -s "$TOKEN" ]] || { echo "token not ready" >&2; exit 1; }; cat "$TOKEN"; echo; }
cmd_log(){ echo '--- mobile bridge ---'; tail -n "${LINES:-100}" "$BLOG" 2>/dev/null || true; echo '--- reasonix ---'; tail -n "${LINES:-100}" "$STATE/reasonix.log" 2>/dev/null || true; }
case "${1:-status}" in start)cmd_start;; stop)cmd_stop;; restart)cmd_stop;cmd_start;; status)cmd_status;; token)cmd_token;; log)cmd_log;; *)echo "usage: $0 start|stop|restart|status|token|log";exit 2;; esac
