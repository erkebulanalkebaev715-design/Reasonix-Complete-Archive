#!/usr/bin/env bash
set -euo pipefail
ROOT="${1:-$HOME/DeepSeek-Reasonix}"
PATCH_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"
git apply --check "$PATCH_DIR/reasonix-balance-mod-v0.1.1-hotfix.patch"
git apply "$PATCH_DIR/reasonix-balance-mod-v0.1.1-hotfix.patch"
echo "Balance Mod v0.1.1 hotfix applied."
echo "Next: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
