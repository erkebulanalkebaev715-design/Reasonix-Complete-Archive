#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
STATE="${REASONIX_MOBILE_STATE:-$HOME/.reasonix-mobile-v1}"
PIDF="$STATE/bridge.pid"
if [[ -s "$PIDF" ]]; then
  p="$(tr -d '\r\n' < "$PIDF" || true)"
  if [[ "$p" =~ ^[0-9]+$ ]] && kill -0 "$p" 2>/dev/null; then
    cmd="$(tr '\0' ' ' < "/proc/$p/cmdline" 2>/dev/null || true)"
    if [[ " $cmd " == *" reasonix_mobile_bridge.py "* ]]; then
      echo "Stopping old mobile bridge pid=$p"
      kill "$p" 2>/dev/null || true
    else
      echo "Ignoring stale bridge pid=$p (cmdline mismatch)" >&2
      p=""
    fi
    if [[ -n "$p" ]]; then
      for _ in $(seq 1 60); do kill -0 "$p" 2>/dev/null || break; sleep .1; done
      kill -0 "$p" 2>/dev/null && kill -9 "$p" 2>/dev/null || true
    fi
  fi
fi
rm -f "$STATE/bridge.pid" "$STATE/bridge.port"
cd "$HERE"
bash ./reasonix_mobile_backend.sh start
# Install/refresh the built-in Android MCP tool server. This performs no model call.
if bash ./reasonix_mobile_backend.sh install-android-tools; then
  echo "ANDROID_TOOLS_INSTALL_PASS"
else
  echo "ANDROID_TOOLS_INSTALL_WARN (backend remains usable)" >&2
fi
bash ./reasonix_mobile_backend.sh diagnose
