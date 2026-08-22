#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/balance_mod_v0.11.patch"

if [[ ! -d "$ROOT/.git" ]]; then
  echo "ERROR: Reasonix git tree not found: $ROOT" >&2
  exit 1
fi
cd "$ROOT"

if grep -q 'const balanceModVersion = "balance-mod-v0.11"' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "Balance Mod v0.11 is already applied."
  exit 0
fi
if ! grep -q 'const balanceModVersion = "balance-mod-v0.10"' internal/serve/mod_bridge.go 2>/dev/null; then
  echo "ERROR: v0.11 expects the passed v0.10 tree. No files were changed." >&2
  exit 1
fi
# v0.8.2 was a compile hotfix that is part of the user's known-good v0.10
# baseline. Refuse an older reconstructed tree instead of repeating that bug.
if ! grep -q 'Persistence map\[string\]any.*json:"persistence"' internal/serve/mod_bridge.go; then
  echo "ERROR: expected v0.8.2 Persistence hotfix is missing. No files were changed." >&2
  exit 1
fi

if ! git apply --check "$PATCH"; then
  echo "ERROR: v0.11 patch does not cleanly apply. No files were changed." >&2
  exit 1
fi
git apply "$PATCH"

echo "Balance Mod v0.11 applied."
echo "Unified power/economy engine + native turn-evidence bridge installed."
echo "Pending routes are applied only at an idle boundary; no mid-turn model switch is attempted."
echo "No provider/API key was read, written, or used."
echo "Next: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
