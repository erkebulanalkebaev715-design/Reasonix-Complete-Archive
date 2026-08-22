#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}";HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")"&&pwd)";PAYLOAD="$HERE/payload";PY_BIN="${PY_BIN:-python3}";[[ -d "$TARGET/.git" ]]||{ echo "ERROR: Reasonix git tree not found: $TARGET" >&2;exit 2;};cd "$TARGET"
for f in internal/serve/mod_bridge.go scripts/balance_mod_smoke_v019.sh scripts/balance_mod_smoke_quick_v019.sh scripts/reasonix_android_backend.sh;do [[ -f "$f" ]]||{ echo "ERROR: v0.19 baseline missing: $f" >&2;exit 3;};done;grep -q BALANCE_MOD_V19_SMOKE_PASS scripts/balance_mod_smoke_v019.sh||{ echo 'ERROR: v0.19 smoke marker missing' >&2;exit 3;}
VERSION="$($PY_BIN - <<'PY'
from pathlib import Path
import re
m=re.search(r'const\s+balanceModVersion\s*=\s*"([^"]+)"',Path('internal/serve/mod_bridge.go').read_text());
if not m:raise SystemExit('marker missing')
print(m.group(1))
PY
)";case "$VERSION" in balance-mod-v0.19|balance-mod-v0.20);;*)echo "ERROR: expected v0.19, got $VERSION" >&2;exit 3;;esac
PATHS=(configs/balance_mod_v020_real_provider_manifest.json configs/reasonix.balance.v020.real.template.toml docs/BALANCE_MOD_V020.md scripts/balance_mod_v020_reconcile.py scripts/balance_mod_v020_real_gate.sh scripts/balance_mod_v020_targeted.sh scripts/balance_mod_smoke_quick_v020.sh scripts/balance_mod_v020_preflight.sh scripts/balance_mod_smoke_v020.sh);for p in "${PATHS[@]}";do [[ -f "$PAYLOAD/$p" ]]||exit 2;if [[ -e "$p" ]]&&! cmp -s "$p" "$PAYLOAD/$p";then echo "ERROR: conflicting $p" >&2;exit 4;fi;done
TMP="$(mktemp -d)";PATCH="$TMP/p";:>"$PATCH";trap 'rm -rf "$TMP"' EXIT;if [[ "$VERSION" == balance-mod-v0.19 ]];then mkdir -p "$TMP/internal/serve";cp internal/serve/mod_bridge.go "$TMP/internal/serve/mod_bridge.go";$PY_BIN - "$TMP/internal/serve/mod_bridge.go" <<'PY'
from pathlib import Path
import re,sys
p=Path(sys.argv[1]);s=p.read_text();s,n=re.subn(r'(const\s+balanceModVersion\s*=\s*)"balance-mod-v0\.19"',r'\1"balance-mod-v0.20"',s,count=1);assert n==1;p.write_text(s)
PY
rc=0;diff -u --label a/internal/serve/mod_bridge.go --label b/internal/serve/mod_bridge.go internal/serve/mod_bridge.go "$TMP/internal/serve/mod_bridge.go">>"$PATCH"||rc=$?;[[ "$rc" == 1 ]]||exit 4;fi
for p in "${PATHS[@]}";do if [[ ! -e "$p" ]];then rc=0;diff -u --label /dev/null --label "b/$p" /dev/null "$PAYLOAD/$p">>"$PATCH"||rc=$?;[[ "$rc" == 1 ]]||exit 4;fi;done
if [[ -s "$PATCH" ]];then git apply --check "$PATCH"||{ echo 'ERROR: v0.20 apply check failed; no fuzzy apply' >&2;exit 4;};echo 'v0.20 patch apply check: PASS';git apply "$PATCH";else echo 'v0.20 already exact';fi
chmod +x scripts/balance_mod_v020_*.sh scripts/balance_mod_v020_reconcile.py scripts/balance_mod_smoke_*v020.sh
bash -n scripts/balance_mod_v020_real_gate.sh;bash -n scripts/balance_mod_v020_targeted.sh;$PY_BIN -m py_compile scripts/balance_mod_v020_reconcile.py;$PY_BIN scripts/balance_mod_v020_reconcile.py --self-test>/dev/null;git diff --check
if [[ -s "$PATCH" ]];then git apply --reverse --check "$PATCH"||{ echo 'ERROR: reverse apply check failed' >&2;exit 6;};echo 'v0.20 reverse-apply check: PASS';fi
echo 'Balance Mod v0.20 applied; diff check: PASS';echo 'Installer performed no Go build/test, provider call, or API-key read/write/use.';echo "Targeted: cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_v020_targeted.sh";echo "Quick: cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_quick_v020.sh";echo "Preflight: cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_v020_preflight.sh";echo 'Real/final gate LOCKED pending explicit approval.'
