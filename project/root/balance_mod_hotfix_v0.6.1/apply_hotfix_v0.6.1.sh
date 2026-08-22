#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.6.1-hotfix.patch"
if [[ ! -d "$TARGET" || ! -f "$TARGET/go.mod" ]]; then echo "ERROR: Reasonix source tree not found: $TARGET" >&2; exit 1; fi
if ! grep -q 'balance-mod-v0.6' "$TARGET/internal/serve/mod_bridge.go" 2>/dev/null; then echo "ERROR: Balance Mod v0.6 baseline not detected." >&2; exit 1; fi
cd "$TARGET"
if git apply --reverse --check "$PATCH" >/dev/null 2>&1; then echo "Balance Mod v0.6.1 hotfix already applied."; exit 0; fi
if ! git apply --check "$PATCH"; then echo "ERROR: v0.6.1 hotfix does not apply cleanly. Do not force it." >&2; exit 1; fi
git apply "$PATCH"
echo "Balance Mod v0.6.1 hotfix applied."
echo "Fix: ProviderVisible now honors runtime sessionHidden deny projection."
