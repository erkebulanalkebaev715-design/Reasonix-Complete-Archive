#!/usr/bin/env bash
set -euo pipefail

REPO="${1:-$HOME/DeepSeek-Reasonix}"
BASE_COMMIT="9e68643823943f05d13ab6a4578b7a629d490b07"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/reasonix-balance-mod-v0.1.patch"

cd "$REPO"
actual="$(git rev-parse HEAD)"
if [[ "$actual" != "$BASE_COMMIT" ]]; then
  echo "STOP: expected base $BASE_COMMIT, got $actual" >&2
  echo "No files were changed." >&2
  exit 2
fi
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "STOP: repository has local tracked changes; stash/backup them first." >&2
  exit 3
fi

git apply --check "$PATCH"
git branch balance-mod-backup-v0.1 "$BASE_COMMIT" 2>/dev/null || true
git apply "$PATCH"

echo "Balance Mod v0.1 applied."
echo "Next: GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
