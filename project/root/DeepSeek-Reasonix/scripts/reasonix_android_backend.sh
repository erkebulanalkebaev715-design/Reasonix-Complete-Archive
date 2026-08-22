#!/usr/bin/env bash
set -euo pipefail

# Minimal Debian-side supervisor for the future Android APK. It intentionally
# owns only the Reasonix process; all agent/model/tool policy stays in Reasonix.
CMD="${1:-status}"
WORKSPACE="${2:-$PWD}"
STATE_DIR="${REASONIX_ANDROID_STATE_DIR:-$HOME/.reasonix/balance/android-backend}"
PORT="${REASONIX_ANDROID_PORT:-8787}"
BIN="${REASONIX_BIN:-$HOME/DeepSeek-Reasonix/bin/reasonix}"
PID_FILE="$STATE_DIR/backend.pid"
TOKEN_FILE="$STATE_DIR/backend.token"
WORKSPACE_FILE="$STATE_DIR/workspace"
LOG_FILE="$STATE_DIR/backend.log"

mkdir -p "$STATE_DIR"
chmod 700 "$STATE_DIR" 2>/dev/null || true

is_running() {
  [[ -f "$PID_FILE" ]] || return 1
  local pid
  pid="$(cat "$PID_FILE" 2>/dev/null || true)"
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  kill -0 "$pid" 2>/dev/null
}

make_token() {
  umask 077
  if [[ ! -s "$TOKEN_FILE" ]]; then
    head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n' > "$TOKEN_FILE"
  fi
  chmod 600 "$TOKEN_FILE" 2>/dev/null || true
}

print_status() {
  if is_running; then
    printf 'RUNNING pid=%s port=%s workspace=%s token_file=%s log=%s\n' \
      "$(cat "$PID_FILE")" "$PORT" "$(cat "$WORKSPACE_FILE" 2>/dev/null || true)" "$TOKEN_FILE" "$LOG_FILE"
  else
    printf 'STOPPED port=%s token_file=%s log=%s\n' "$PORT" "$TOKEN_FILE" "$LOG_FILE"
  fi
}

case "$CMD" in
  start)
    if is_running; then print_status; exit 0; fi
    [[ -x "$BIN" ]] || { echo "ERROR: reasonix binary not found: $BIN" >&2; exit 2; }
    [[ -d "$WORKSPACE" ]] || { echo "ERROR: workspace not found: $WORKSPACE" >&2; exit 2; }
    WORKSPACE="$(cd "$WORKSPACE" && pwd -P)"
    make_token
    TOKEN="$(cat "$TOKEN_FILE")"
    printf '%s\n' "$WORKSPACE" > "$WORKSPACE_FILE"
    chmod 600 "$WORKSPACE_FILE" 2>/dev/null || true
    (
      cd "$WORKSPACE"
      nohup "$BIN" serve --addr "127.0.0.1:$PORT" --auth token --token "$TOKEN" \
        </dev/null >>"$LOG_FILE" 2>&1 &
      echo $! > "$PID_FILE"
    )
    sleep 0.25
    if ! is_running; then
      echo "ERROR: backend exited during startup" >&2
      tail -n 30 "$LOG_FILE" >&2 || true
      exit 3
    fi
    print_status
    ;;
  stop)
    if is_running; then
      pid="$(cat "$PID_FILE")"
      kill "$pid" 2>/dev/null || true
      for _ in 1 2 3 4 5 6 7 8 9 10; do
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.1
      done
      kill -9 "$pid" 2>/dev/null || true
    fi
    rm -f "$PID_FILE"
    print_status
    ;;
  restart)
    "$0" stop "$WORKSPACE" >/dev/null
    exec "$0" start "$WORKSPACE"
    ;;
  status)
    print_status
    ;;
  token)
    make_token
    cat "$TOKEN_FILE"
    ;;
  log)
    tail -n "${REASONIX_ANDROID_LOG_LINES:-80}" "$LOG_FILE" 2>/dev/null || true
    ;;
  *)
    echo "Usage: $0 {start|stop|restart|status|token|log} [workspace]" >&2
    exit 2
    ;;
esac
