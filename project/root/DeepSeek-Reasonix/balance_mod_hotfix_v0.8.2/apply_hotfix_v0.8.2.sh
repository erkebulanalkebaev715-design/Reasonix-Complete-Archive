#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.8.2.patch"

if [[ ! -f "$TARGET/go.mod" || ! -f "$TARGET/internal/serve/mod_bridge.go" ]]; then
  echo "ERROR: Reasonix source tree not found: $TARGET" >&2
  exit 2
fi
cd "$TARGET"

if grep -q 'Persistence map\[string\]any.*json:"persistence"' internal/serve/mod_bridge.go; then
  echo "Balance Mod v0.8.2 hotfix already applied."
  exit 0
fi
if ! grep -q 'balance-mod-v0.8' internal/serve/mod_bridge.go; then
  echo "ERROR: expected applied Balance Mod v0.8/v0.8.1 baseline." >&2
  exit 3
fi
if ! grep -q 'Persistence: s.modPersistenceStatus()' internal/serve/mod_bridge.go; then
  echo "ERROR: v0.8.1 persistence wiring not detected; refusing partial repair." >&2
  exit 3
fi
if ! git apply --check "$PATCH"; then
  echo "ERROR: v0.8.2 hotfix does not cleanly apply. No files were changed." >&2
  exit 4
fi

git apply "$PATCH"
if command -v gofmt >/dev/null 2>&1; then
  gofmt -w internal/serve/mod_bridge.go
elif [[ -x /usr/local/go/bin/gofmt ]]; then
  /usr/local/go/bin/gofmt -w internal/serve/mod_bridge.go
fi

echo "Balance Mod v0.8.2 compile hotfix applied."
echo "Fix: modStatusPayload now declares the persistence field already used by v0.8.1."
echo "No provider/API key was read, written, or used."
