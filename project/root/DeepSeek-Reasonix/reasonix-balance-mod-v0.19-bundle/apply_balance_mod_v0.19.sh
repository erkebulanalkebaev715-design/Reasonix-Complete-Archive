#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PAYLOAD="$HERE/payload"
PY_BIN="${PY_BIN:-python3}"

[[ -d "$TARGET/.git" ]] || { echo "ERROR: Reasonix git tree not found: $TARGET" >&2; exit 2; }
command -v git >/dev/null 2>&1 || { echo "ERROR: git missing" >&2; exit 2; }
command -v diff >/dev/null 2>&1 || { echo "ERROR: diff missing" >&2; exit 2; }
command -v "$PY_BIN" >/dev/null 2>&1 || { echo "ERROR: python3 missing" >&2; exit 2; }
cd "$TARGET"

for f in \
  internal/serve/mod_bridge.go \
  configs/reasonix.balance.v018.toml \
  configs/balance_mod_v018_rc_manifest.json \
  scripts/balance_mod_v018_targeted.sh \
  scripts/balance_mod_smoke_quick_v018.sh \
  scripts/balance_mod_smoke_v018.sh \
  scripts/reasonix_android_backend.sh; do
  [[ -f "$f" ]] || { echo "ERROR: verified v0.18 baseline file missing: $f" >&2; exit 3; }
done

grep -q 'BALANCE_MOD_V18_SMOKE_PASS' scripts/balance_mod_smoke_v018.sh || {
  echo "ERROR: v0.18 full smoke marker missing; refusing v0.19." >&2; exit 3;
}

VERSION="$($PY_BIN - <<'PY'
from pathlib import Path
import re
s=Path('internal/serve/mod_bridge.go').read_text(encoding='utf-8')
m=re.search(r'const\s+balanceModVersion\s*=\s*"([^"]+)"',s)
if not m: raise SystemExit('ERROR: balanceModVersion marker not found')
print(m.group(1))
PY
)"
case "$VERSION" in
  balance-mod-v0.18|balance-mod-v0.19) ;;
  *) echo "ERROR: expected v0.18 baseline, found '$VERSION'; refusing v0.19." >&2; exit 3 ;;
esac

PAYLOAD_PATHS=(
  configs/reasonix.balance.v019.toml
  configs/balance_mod_v019_apk_backend_manifest.json
  docs/BALANCE_MOD_V019.md
  scripts/balance_mod_v019_contract_probe.py
  scripts/balance_mod_v019_targeted.sh
  scripts/balance_mod_smoke_quick_v019.sh
  scripts/balance_mod_smoke_v019.sh
)
for path in "${PAYLOAD_PATHS[@]}"; do
  [[ -f "$PAYLOAD/$path" ]] || { echo "ERROR: bundle payload missing: $path" >&2; exit 2; }
  if [[ -e "$path" ]] && ! cmp -s "$path" "$PAYLOAD/$path"; then
    echo "ERROR: target already has conflicting v0.19 file: $path" >&2
    echo "Refusing overwrite." >&2
    exit 4
  fi
done

TMP="$(mktemp -d)"
PATCH="$TMP/v0.19.rebased.patch"
: > "$PATCH"
cleanup(){ rm -rf "$TMP"; }
trap cleanup EXIT

if [[ "$VERSION" == "balance-mod-v0.18" ]]; then
  mkdir -p "$TMP/internal/serve"
  cp internal/serve/mod_bridge.go "$TMP/internal/serve/mod_bridge.go"
  "$PY_BIN" - "$TMP/internal/serve/mod_bridge.go" <<'PY'
from pathlib import Path
import re,sys
p=Path(sys.argv[1]); s=p.read_text(encoding='utf-8')
s,n=re.subn(r'(const\s+balanceModVersion\s*=\s*)"balance-mod-v0\.18"',r'\1"balance-mod-v0.19"',s,count=1)
if n != 1: raise SystemExit('ERROR: could not stage v0.19 version marker')
p.write_text(s,encoding='utf-8')
PY
  rc=0
  diff -u --label a/internal/serve/mod_bridge.go --label b/internal/serve/mod_bridge.go \
    internal/serve/mod_bridge.go "$TMP/internal/serve/mod_bridge.go" >> "$PATCH" || rc=$?
  [[ "$rc" == "1" ]] || { echo "ERROR: could not generate version patch" >&2; exit 4; }
fi

for path in "${PAYLOAD_PATHS[@]}"; do
  if [[ ! -e "$path" ]]; then
    rc=0
    diff -u --label /dev/null --label "b/$path" /dev/null "$PAYLOAD/$path" >> "$PATCH" || rc=$?
    [[ "$rc" == "1" ]] || { echo "ERROR: could not stage payload: $path" >&2; exit 4; }
  fi
done

if [[ -s "$PATCH" ]]; then
  git apply --check "$PATCH" || { echo "ERROR: v0.19 dynamic patch apply check failed; refusing fuzzy/force apply." >&2; exit 4; }
  echo "v0.19 dynamic patch apply check: PASS"
  git apply "$PATCH"
else
  echo "Balance Mod v0.19: exact payload already present."
fi

chmod +x \
  scripts/balance_mod_v019_contract_probe.py \
  scripts/balance_mod_v019_targeted.sh \
  scripts/balance_mod_smoke_quick_v019.sh \
  scripts/balance_mod_smoke_v019.sh

postcheck(){
  grep -q 'const balanceModVersion = "balance-mod-v0.19"' internal/serve/mod_bridge.go || return 1
  bash -n scripts/balance_mod_v019_targeted.sh || return 1
  bash -n scripts/balance_mod_smoke_quick_v019.sh || return 1
  bash -n scripts/balance_mod_smoke_v019.sh || return 1
  "$PY_BIN" -m py_compile scripts/balance_mod_v019_contract_probe.py || return 1
  "$PY_BIN" - <<'PY'
import json
from pathlib import Path
m=json.loads(Path('configs/balance_mod_v019_apk_backend_manifest.json').read_text(encoding='utf-8'))
assert m['modVersion']=='balance-mod-v0.19'
assert m['baseline']=='balance-mod-v0.18'
assert m['apkContract']=='balance-apk-v1'
assert m['transport']['authMode']=='token'
assert m['transport']['bind']=='127.0.0.1:0'
assert m['provider']['kind']=='mock' and m['provider']['apiKeyRequired'] is False
assert m['contractMinimums']=={'endpoints':67,'eventTypes':68}
cfg=Path('configs/reasonix.balance.v019.toml').read_text(encoding='utf-8')
for marker in ('default_model = "balance-apk-mock"','kind = "mock"','model = "smoke"','offline = true','bash = "off"'):
    assert marker in cfg, marker
PY
  git diff --check || return 1
}

if ! postcheck; then
  echo "ERROR: v0.19 post-apply checks failed." >&2
  if [[ -s "$PATCH" ]] && git apply --reverse --check "$PATCH"; then
    git apply --reverse "$PATCH" || true
    echo "v0.19 patch rolled back." >&2
  fi
  exit 5
fi

if [[ -s "$PATCH" ]]; then
  git apply --reverse --check "$PATCH" || { echo "ERROR: v0.19 reverse-apply check failed." >&2; exit 6; }
  echo "v0.19 reverse-apply check: PASS"
else
  echo "v0.19 reverse-apply check: already-applied idempotent path"
fi

echo "Balance Mod v0.19 applied; diff check: PASS"
echo "No Go build/test and no provider/API call was performed by this installer."
echo "Targeted: cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_v019_targeted.sh"
echo "Quick:    cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_quick_v019.sh"
echo "Full:     cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_v019.sh"
