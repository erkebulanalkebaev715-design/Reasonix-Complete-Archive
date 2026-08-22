#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PAYLOAD="$HERE/payload"
PY_BIN="${PY_BIN:-python3}"

[[ -d "$TARGET/.git" ]] || { echo "ERROR: Reasonix git tree not found: $TARGET" >&2; exit 2; }
cd "$TARGET"
[[ -f internal/serve/mod_bridge.go ]] || { echo 'ERROR: mod_bridge.go missing' >&2; exit 3; }

VERSION="$($PY_BIN - <<'PY'
from pathlib import Path
import re
m=re.search(r'const\s+balanceModVersion\s*=\s*"([^"]+)"',Path('internal/serve/mod_bridge.go').read_text())
if not m: raise SystemExit('marker missing')
print(m.group(1))
PY
)"
case "$VERSION" in
  balance-mod-v0.19|balance-mod-v0.20) ;;
  *) echo "ERROR: expected v0.19/v0.20 baseline, got $VERSION" >&2; exit 3 ;;
esac

# Stop only stale v0.20 real-gate processes from interrupted previous attempts.
# Match argv tokens, not free-form `ps` text, so the installer cannot kill its own parent shell.
STOPPED="$($PY_BIN - <<'PY'
import os,signal
me=os.getpid(); parent=os.getppid(); stopped=0
for name in os.listdir('/proc'):
    if not name.isdigit():
        continue
    pid=int(name)
    if pid in (me,parent):
        continue
    try:
        raw=open(f'/proc/{pid}/cmdline','rb').read()
        argv=[x.decode('utf-8','replace') for x in raw.split(b'\\0') if x]
    except Exception:
        continue
    if not argv:
        continue
    gate=any(os.path.basename(a)=='balance_mod_v020_real_gate.sh' for a in argv)
    reasonix=('serve' in argv and any(a=='deepseek-v20/deepseek-v4-flash' for a in argv))
    if gate or reasonix:
        try:
            os.kill(pid, signal.SIGTERM); stopped += 1
        except (ProcessLookupError,PermissionError):
            pass
print(stopped)
PY
)"
sleep 0.2
rm -rf /tmp/reasonix-v020-real-gate.lock 2>/dev/null || true
echo "v0.20 stale online-gate cleanup: PASS (stopped=$STOPPED)"

PATHS=(
  configs/balance_mod_v020_real_provider_manifest.json
  configs/reasonix.balance.v020.real.template.toml
  docs/BALANCE_MOD_V020.md
  docs/BALANCE_MOD_V020_FIXED.md
  scripts/balance_mod_v020_reconcile.py
  scripts/balance_mod_v020_real_gate.sh
  scripts/balance_mod_v020_targeted.sh
  scripts/balance_mod_smoke_quick_v020.sh
  scripts/balance_mod_v020_preflight.sh
  scripts/balance_mod_smoke_v020.sh
)
for p in "${PATHS[@]}"; do
  [[ -f "$PAYLOAD/$p" ]] || { echo "ERROR: payload missing $p" >&2; exit 2; }
done

TMP="$(mktemp -d)"
PATCH="$TMP/v020-fixed.patch"
BACKUP="$TARGET/.balance_mod_backups/v0.20-fixed-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP"
: > "$PATCH"
trap 'rm -rf "$TMP"' EXIT

# Build exact patch against the ACTUAL current tree; no fuzzy application.
if [[ "$VERSION" == balance-mod-v0.19 ]]; then
  mkdir -p "$TMP/internal/serve"
  cp internal/serve/mod_bridge.go "$TMP/internal/serve/mod_bridge.go"
  "$PY_BIN" - "$TMP/internal/serve/mod_bridge.go" <<'PY'
from pathlib import Path
import re,sys
p=Path(sys.argv[1]); s=p.read_text()
s,n=re.subn(r'(const\s+balanceModVersion\s*=\s*)"balance-mod-v0\.19"',r'\1"balance-mod-v0.20"',s,count=1)
assert n==1
p.write_text(s)
PY
  rc=0
  diff -u --label a/internal/serve/mod_bridge.go --label b/internal/serve/mod_bridge.go \
    internal/serve/mod_bridge.go "$TMP/internal/serve/mod_bridge.go" >> "$PATCH" || rc=$?
  [[ "$rc" == 1 ]] || { echo 'ERROR: could not construct version patch' >&2; exit 4; }
fi

for p in "${PATHS[@]}"; do
  mkdir -p "$BACKUP/$(dirname "$p")"
  if [[ -e "$p" ]]; then
    if cmp -s "$p" "$PAYLOAD/$p"; then
      continue
    fi
    cp -a "$p" "$BACKUP/$p"
    rc=0
    diff -u --label "a/$p" --label "b/$p" "$p" "$PAYLOAD/$p" >> "$PATCH" || rc=$?
    [[ "$rc" == 1 ]] || { echo "ERROR: diff failed for $p" >&2; exit 4; }
  else
    rc=0
    diff -u --label /dev/null --label "b/$p" /dev/null "$PAYLOAD/$p" >> "$PATCH" || rc=$?
    [[ "$rc" == 1 ]] || { echo "ERROR: add-file diff failed for $p" >&2; exit 4; }
  fi
done

if [[ -s "$PATCH" ]]; then
  git apply --check "$PATCH" || { echo 'ERROR: v0.20 Fixed2 exact apply check failed; refusing fuzzy apply' >&2; exit 4; }
  echo 'v0.20 Fixed2 patch apply check: PASS'
  git apply "$PATCH"
else
  echo 'v0.20 Fixed2 already exact'
fi

chmod +x scripts/balance_mod_v020_*.sh scripts/balance_mod_v020_reconcile.py scripts/balance_mod_smoke_*v020.sh
bash -n scripts/balance_mod_v020_real_gate.sh
bash -n scripts/balance_mod_smoke_v020.sh
bash -n scripts/balance_mod_v020_preflight.sh
"$PY_BIN" -m py_compile scripts/balance_mod_v020_reconcile.py
"$PY_BIN" scripts/balance_mod_v020_reconcile.py --self-test >/dev/null
git diff --check

if [[ -s "$PATCH" ]]; then
  git apply --reverse --check "$PATCH" || { echo 'ERROR: v0.20 Fixed2 reverse-apply check failed' >&2; exit 6; }
  echo 'v0.20 Fixed2 reverse-apply check: PASS'
fi

echo 'Balance Mod v0.20 Fixed2 applied; diff check: PASS'
echo "Backup of replaced v0.20 files: $BACKUP"
echo 'No Go build/test, API-key read, or provider call was performed by this installer.'
echo "ONLINE FINAL: cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_v020.sh"
