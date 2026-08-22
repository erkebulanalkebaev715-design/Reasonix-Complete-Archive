#!/usr/bin/env bash
set -euo pipefail
ROOT="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.2.patch"
cd "$ROOT"

git apply --check "$PATCH"
git apply "$PATCH"

echo "Balance Mod v0.2 applied."
echo "Next: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
