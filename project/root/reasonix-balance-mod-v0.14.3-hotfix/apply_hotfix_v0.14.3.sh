#!/usr/bin/env bash
set -euo pipefail
ROOT="${1:-$HOME/DeepSeek-Reasonix}"
SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH_FILE="$SELF_DIR/balance_mod_v0.14.3.patch"

if [[ ! -d "$ROOT" || ! -f "$ROOT/go.mod" ]]; then
  echo "ERROR: Reasonix root not found: $ROOT" >&2
  exit 1
fi
cd "$ROOT"
if ! git apply --check "$PATCH_FILE"; then
  echo "ERROR: v0.14.3 hotfix does not cleanly apply. No files were changed." >&2
  exit 1
fi
git apply "$PATCH_FILE"
echo "Balance Mod v0.14.3 hotfix applied."
echo "Fixed offline prototype budget JSON path: /mod/budget returns {budget:{...}, taskCostGate:{...}}."
echo "Next: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
