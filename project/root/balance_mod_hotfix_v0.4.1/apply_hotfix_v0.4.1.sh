#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.4.1-hotfix.patch"

if [[ ! -d "$TARGET" || ! -f "$TARGET/go.mod" ]]; then
  echo "ERROR: Reasonix source tree not found: $TARGET" >&2
  exit 1
fi
if ! grep -q 'balance-mod-v0.4' "$TARGET/internal/serve/mod_bridge.go" 2>/dev/null; then
  echo "ERROR: Balance Mod v0.4 baseline not detected." >&2
  exit 1
fi

# Idempotency: if both v0.4 POST tests already carry JSON Content-Type, stop cleanly.
if awk '/mod\/cycle\/reset/{seen=1} seen && /Content-Type.*application\/json/{a=1; seen=0} END{exit !a}' "$TARGET/internal/serve/mod_bridge_test.go" \
  && awk '/mod\/recovery\/rollback-last/{seen=1} seen && /Content-Type.*application\/json/{a=1; seen=0} END{exit !a}' "$TARGET/internal/serve/mod_bridge_test.go"; then
  echo "Balance Mod v0.4.1 hotfix already appears to be applied."
  exit 0
fi

cd "$TARGET"
if ! git apply --check "$PATCH"; then
  echo "ERROR: v0.4.1 hotfix does not apply cleanly. Do not force it; send the output." >&2
  exit 1
fi
git apply "$PATCH"

echo "Balance Mod v0.4.1 hotfix applied."
echo "Fix: v0.4 APK POST tests now obey Reasonix's existing JSON/CSRF contract."
echo "Next quick check: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local go test ./internal/serve -run '^(TestModCycleAPIAndHostRepairWiring|TestModRecoveryAPIIsFailClosedWithoutCheckpoint)$' -count=1"
echo "Then rerun: PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
