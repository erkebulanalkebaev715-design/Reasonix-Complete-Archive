#!/usr/bin/env bash
set -euo pipefail
ROOT="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/balance_mod_v0.12.patch"
cd "$ROOT"
if ! grep -q 'const balanceModVersion = "balance-mod-v0.12"' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "ERROR: v0.12 is not the active tree; refusing rollback." >&2
  exit 1
fi
if ! git apply --check -R "$PATCH"; then
  echo "ERROR: v0.12 rollback does not cleanly apply. No files were changed." >&2
  exit 1
fi
git apply -R "$PATCH"
echo "Balance Mod v0.12 rolled back to v0.11."
