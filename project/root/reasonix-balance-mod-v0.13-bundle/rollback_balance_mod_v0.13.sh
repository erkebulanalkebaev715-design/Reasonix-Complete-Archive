#!/usr/bin/env bash
set -euo pipefail
ROOT="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/balance_mod_v0.13.patch"
cd "$ROOT"
if ! grep -q 'const balanceModVersion = "balance-mod-v0.13"' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "ERROR: v0.13 marker not found; refusing rollback." >&2
  exit 1
fi
if ! git apply --check --reverse "$PATCH"; then
  echo "ERROR: v0.13 reverse patch does not cleanly apply." >&2
  exit 1
fi
git apply --reverse "$PATCH"
echo "Balance Mod v0.13 rolled back to v0.12 source state."
