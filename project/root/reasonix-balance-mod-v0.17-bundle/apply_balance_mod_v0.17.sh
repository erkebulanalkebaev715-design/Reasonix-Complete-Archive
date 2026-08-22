#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/balance_mod_v0.17.patch"

if [[ ! -d "$TARGET/.git" ]]; then
  echo "ERROR: Reasonix git tree not found: $TARGET" >&2
  exit 2
fi
if [[ ! -f "$PATCH" ]]; then
  echo "ERROR: v0.17 patch missing: $PATCH" >&2
  exit 2
fi

cd "$TARGET"

# v0.17 is an overlay for the already-installed v0.16 baseline only.
if ! grep -Rqs 'balance-mod-v0.16' internal/serve internal/efficiency internal/agent 2>/dev/null; then
  echo "ERROR: v0.16 runtime marker not found; refusing to apply v0.17 to an unknown baseline." >&2
  exit 3
fi

if git apply --check "$PATCH"; then
  echo "v0.17 patch apply check: PASS"
  git apply "$PATCH"
elif git apply --reverse --check "$PATCH"; then
  echo "Balance Mod v0.17 is already applied; reverse-apply check: PASS"
else
  echo "ERROR: v0.17 patch does not match this tree." >&2
  echo "Refusing force/fuzzy apply. Keep the current tree and inspect the mismatch." >&2
  exit 4
fi

# Formatting is deterministic and local. No provider, network, or API key.
if command -v gofmt >/dev/null 2>&1; then
  GOFMT="$(command -v gofmt)"
elif [[ -x /usr/local/go/bin/gofmt ]]; then
  GOFMT=/usr/local/go/bin/gofmt
else
  echo "ERROR: gofmt not found" >&2
  exit 5
fi

"$GOFMT" -w \
  internal/control/inbox.go \
  internal/control/inbox_completion_receipt_test.go \
  internal/sessioninbox/completion_receipt.go \
  internal/sessioninbox/completion_receipt_test.go \
  internal/sessioninbox/manifest.go \
  internal/sessioninbox/ops.go \
  internal/sessioninbox/recovery.go \
  internal/sessioninbox/store.go \
  internal/sessioninbox/types.go

if ! git diff --check; then
  echo "ERROR: git diff --check failed after v0.17 apply" >&2
  exit 6
fi

echo "Balance Mod v0.17 applied; diff check: PASS"
echo "No Go build/test and no provider/API call was performed by this installer."
echo "Targeted: cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_v017_targeted.sh"
echo "Quick:    cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_quick_v017.sh"
echo "Full:     cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_v017.sh"
