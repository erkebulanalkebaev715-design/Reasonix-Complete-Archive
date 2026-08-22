#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
STATE="${REASONIX_MOBILE_STATE:-$HOME/.reasonix-mobile-v1}"
PIDF="$STATE/bridge.pid"
# v1.5.0: approval/control bridge + Android/PRoot-safe stale-PID/orphan cleanup.
cd "$HERE"
bash ./reasonix_mobile_backend.sh stop

bash ./reasonix_mobile_backend.sh start
# Install/refresh the built-in Android MCP tool server. This performs no model call.
if bash ./reasonix_mobile_backend.sh install-android-tools; then
  echo "ANDROID_TOOLS_INSTALL_PASS"
else
  echo "ANDROID_TOOLS_INSTALL_WARN (backend remains usable)" >&2
fi
# Install a small useful Reasonix Skill pack without overwriting user edits.
if bash ./reasonix_mobile_backend.sh install-builtins; then
  echo "BUILTIN_SKILLS_INSTALL_PASS"
else
  echo "BUILTIN_SKILLS_INSTALL_WARN" >&2
fi
# Mobile is an interactive frontend, but default to Reasonix Auto approval posture:
# ordinary policy-approved writes don't wedge headlessly; explicit ask/deny/fresh approvals still surface to APK.
if bash ./reasonix_mobile_backend.sh approval-auto; then
  echo "TOOL_APPROVAL_AUTO_PASS"
else
  echo "TOOL_APPROVAL_AUTO_WARN" >&2
fi
bash ./reasonix_mobile_backend.sh diagnose
echo '--- integration audit ---'
bash ./reasonix_mobile_backend.sh integration-audit || true
