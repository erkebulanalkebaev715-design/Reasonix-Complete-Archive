#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PAYLOAD="$HERE/payload"
PY_BIN="${PY_BIN:-python3}"

if [[ ! -d "$TARGET/.git" ]]; then
  echo "ERROR: Reasonix git tree not found: $TARGET" >&2
  exit 2
fi
command -v git >/dev/null 2>&1 || { echo "ERROR: git missing" >&2; exit 2; }
command -v diff >/dev/null 2>&1 || { echo "ERROR: diff missing" >&2; exit 2; }
command -v "$PY_BIN" >/dev/null 2>&1 || { echo "ERROR: python3 missing" >&2; exit 2; }

cd "$TARGET"

# This fixed installer is intentionally rebased on the actually-passed v0.17
# baseline. v0.17 did not bump mod_bridge's public version marker, so the
# expected pre-v0.18 marker is still balance-mod-v0.16.
for f in \
  internal/serve/mod_bridge.go \
  internal/sessioninbox/completion_receipt.go \
  internal/sessioninbox/completion_receipt_test.go \
  internal/sessioninbox/types.go \
  scripts/balance_mod_v017_targeted.sh \
  scripts/balance_mod_smoke_quick_v017.sh \
  scripts/balance_mod_smoke_v017.sh; do
  [[ -f "$f" ]] || { echo "ERROR: v0.17 baseline file missing: $f" >&2; exit 3; }
done

grep -q 'const SchemaVersion = 3' internal/sessioninbox/types.go || {
  echo "ERROR: v0.17 inbox schema marker missing; refusing v0.18." >&2
  exit 3
}
grep -q 'CommitCompletion' internal/sessioninbox/completion_receipt.go || {
  echo "ERROR: v0.17 completion-receipt runtime missing; refusing v0.18." >&2
  exit 3
}
if grep -q 'meta, ok := m.item(id)' internal/sessioninbox/completion_receipt.go; then
  echo "ERROR: unfixed v0.17 unused-meta code detected; refusing v0.18." >&2
  exit 3
fi
grep -q 's.runID = foreignRunID' internal/sessioninbox/completion_receipt_test.go || {
  echo "ERROR: passed v0.17 crash-simulation hotfix marker missing; refusing v0.18." >&2
  exit 3
}

VERSION="$($PY_BIN - <<'PY'
from pathlib import Path
import re
s=Path('internal/serve/mod_bridge.go').read_text(encoding='utf-8')
m=re.search(r'const\s+balanceModVersion\s*=\s*"([^"]+)"', s)
if not m:
    raise SystemExit('ERROR: balanceModVersion marker not found')
print(m.group(1))
PY
)"
case "$VERSION" in
  balance-mod-v0.16|balance-mod-v0.18) ;;
  *)
    echo "ERROR: unexpected Balance Mod marker '$VERSION'; refusing v0.18." >&2
    exit 3
    ;;
esac

# Never overwrite a conflicting v0.18 file. Existing byte-identical files are
# accepted so a partially-completed/idempotent install can be safely resumed.
PAYLOAD_PATHS=(
  configs/reasonix.balance.v018.toml
  configs/balance_mod_v018_rc_manifest.json
  docs/BALANCE_MOD_V018.md
  scripts/balance_mod_v018_targeted.sh
  scripts/balance_mod_v018_rc.sh
  scripts/balance_mod_smoke_quick_v018.sh
  scripts/balance_mod_smoke_v018.sh
)
for path in "${PAYLOAD_PATHS[@]}"; do
  [[ -f "$PAYLOAD/$path" ]] || { echo "ERROR: bundle payload missing: $path" >&2; exit 2; }
  if [[ -e "$path" ]] && ! cmp -s "$path" "$PAYLOAD/$path"; then
    echo "ERROR: target already has a conflicting v0.18 file: $path" >&2
    echo "Refusing overwrite." >&2
    exit 4
  fi
done

TMP="$(mktemp -d)"
DYN_PATCH="$TMP/v0.18.rebased.patch"
: > "$DYN_PATCH"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

# Build the patch against THIS exact tree, not guessed line numbers.
if [[ "$VERSION" == "balance-mod-v0.16" ]]; then
  mkdir -p "$TMP/internal/serve"
  cp internal/serve/mod_bridge.go "$TMP/internal/serve/mod_bridge.go"
  "$PY_BIN" - "$TMP/internal/serve/mod_bridge.go" <<'PY'
from pathlib import Path
import re,sys
p=Path(sys.argv[1])
s=p.read_text(encoding='utf-8')
new,n=re.subn(
    r'(const\s+balanceModVersion\s*=\s*)"balance-mod-v0\.16"',
    r'\1"balance-mod-v0.18"',
    s,
    count=1,
)
if n != 1:
    raise SystemExit('ERROR: could not stage v0.18 version marker')
p.write_text(new, encoding='utf-8')
PY
  diff -u \
    --label a/internal/serve/mod_bridge.go \
    --label b/internal/serve/mod_bridge.go \
    internal/serve/mod_bridge.go "$TMP/internal/serve/mod_bridge.go" >> "$DYN_PATCH" || rc=$?
  if [[ "${rc:-0}" != "1" ]]; then
    echo "ERROR: could not generate mod_bridge patch" >&2
    exit 4
  fi
  unset rc
fi

for path in "${PAYLOAD_PATHS[@]}"; do
  if [[ ! -e "$path" ]]; then
    diff -u --label /dev/null --label "b/$path" /dev/null "$PAYLOAD/$path" >> "$DYN_PATCH" || rc=$?
    if [[ "${rc:-0}" != "1" ]]; then
      echo "ERROR: could not stage payload patch: $path" >&2
      exit 4
    fi
    unset rc
  fi
done

if [[ ! -s "$DYN_PATCH" ]]; then
  echo "Balance Mod v0.18 fixed bundle: target already contains the exact v0.18 payload."
else
  if ! git apply --check "$DYN_PATCH"; then
    echo "ERROR: dynamically rebased v0.18 patch failed git apply --check." >&2
    echo "Refusing force/fuzzy apply." >&2
    exit 4
  fi
  echo "v0.18 dynamic patch apply check: PASS"
  git apply "$DYN_PATCH"
fi

chmod +x \
  scripts/balance_mod_v018_targeted.sh \
  scripts/balance_mod_v018_rc.sh \
  scripts/balance_mod_smoke_quick_v018.sh \
  scripts/balance_mod_smoke_v018.sh

postcheck() {
  grep -q 'const balanceModVersion = "balance-mod-v0.18"' internal/serve/mod_bridge.go || return 1
  bash -n scripts/balance_mod_v018_targeted.sh || return 1
  bash -n scripts/balance_mod_v018_rc.sh || return 1
  bash -n scripts/balance_mod_smoke_quick_v018.sh || return 1
  bash -n scripts/balance_mod_smoke_v018.sh || return 1

  "$PY_BIN" - <<'PY'
import json
from pathlib import Path
p=Path('configs/balance_mod_v018_rc_manifest.json')
data=json.loads(p.read_text(encoding='utf-8'))
assert data['modVersion']=='balance-mod-v0.18'
assert data['baseline']=='balance-mod-v0.17'
assert data['apkContract']=='balance-apk-v1'
assert data['provider']['kind']=='mock'
assert data['provider']['apiKeyRequired'] is False
assert data['budget']['hardStop'] is True
assert data['budget']['budgetKzt']==1000
assert data['budget']['reservePercent']==15
assert data['budget']['proMaxPercent']==25
cfg=Path('configs/reasonix.balance.v018.toml').read_text(encoding='utf-8')
for marker in (
    'default_model = "balance-rc-mock"',
    'kind = "mock"',
    'model = "smoke"',
    'offline = true',
    'bash = "off"',
):
    assert marker in cfg, marker
PY

  git diff --check || return 1
}

if ! postcheck; then
  echo "ERROR: v0.18 post-apply checks failed." >&2
  if [[ -s "$DYN_PATCH" ]] && git apply --reverse --check "$DYN_PATCH"; then
    git apply --reverse "$DYN_PATCH" || true
    echo "v0.18 content patch rolled back." >&2
  fi
  exit 5
fi

if [[ -s "$DYN_PATCH" ]]; then
  if ! git apply --reverse --check "$DYN_PATCH"; then
    echo "ERROR: v0.18 reverse-apply check failed after installation." >&2
    exit 6
  fi
  echo "v0.18 reverse-apply check: PASS"
else
  echo "v0.18 reverse-apply check: already-applied idempotent path"
fi

echo "Balance Mod v0.18 fixed bundle applied; diff check: PASS"
echo "No Go build/test and no provider/API call was performed by this installer."
echo "Targeted: cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_v018_targeted.sh"
echo "Quick:    cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_quick_v018.sh"
echo "Full:     cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_v018.sh"
