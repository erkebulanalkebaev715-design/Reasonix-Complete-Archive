#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
STATE="${REASONIX_MOBILE_STATE:-$HOME/.reasonix-mobile-v1}"
PIDF="$STATE/bridge.pid"
# v1.3.1: delegate stale-PID and orphan-listener cleanup to the supervisor.
cd "$HERE"
bash ./reasonix_mobile_backend.sh stop

bash ./reasonix_mobile_backend.sh start
# Install/refresh the built-in Android MCP tool server. This performs no model call.
if bash ./reasonix_mobile_backend.sh install-android-tools; then
  echo "ANDROID_TOOLS_INSTALL_PASS"
else
  echo "ANDROID_TOOLS_INSTALL_WARN (backend remains usable)" >&2
fi
bash ./reasonix_mobile_backend.sh diagnose
