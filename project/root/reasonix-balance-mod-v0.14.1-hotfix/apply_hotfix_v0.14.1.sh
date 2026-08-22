#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
PATCH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/balance_mod_v0.14.1_hotfix.patch"
cd "$TARGET"
if git apply --reverse --check "$PATCH" >/dev/null 2>&1; then
  echo "Balance Mod v0.14.1 hotfix already applied."
  exit 0
fi
if ! git apply --check "$PATCH"; then
  echo "ERROR: v0.14.1 hotfix does not cleanly apply. No files were changed." >&2
  exit 1
fi
git apply "$PATCH"
echo "Balance Mod v0.14.1 hotfix applied."
echo "Fix: frozen APK contract test now checks the actual mutatingRequestsContentType key."
echo "Quick: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_quick.sh"
echo "Full:  PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
