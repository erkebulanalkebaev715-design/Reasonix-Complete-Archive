#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PAYLOAD="$HERE/payload"
PY_BIN="${PY_BIN:-python3}"
GO_BIN="${GO_BIN:-/usr/local/go/bin/go}"
[[ -d "$TARGET/.git" ]] || { echo "ERROR: Reasonix git tree not found: $TARGET" >&2; exit 2; }
cd "$TARGET"
for f in internal/agent/run_usage.go internal/agent/run_budget.go internal/agent/sampling_request.go internal/serve/mod_bridge.go; do
  [[ -f "$f" ]] || { echo "ERROR: required core file missing: $f" >&2; exit 3; }
done
grep -q 'const balanceModVersion = "balance-mod-v0.20"' internal/serve/mod_bridge.go || { echo 'ERROR: v0.20 marker missing' >&2; exit 3; }
grep -q 'strict-one-paid-attempt-v0.20-fixed9' internal/agent/run_budget.go || { echo 'ERROR: validated Fixed9 hard-budget core missing' >&2; exit 3; }
! grep -q 'strictPreCallRetryFactor' internal/agent/run_budget.go || { echo 'ERROR: obsolete 66-way reserve present' >&2; exit 3; }
grep -q 'WithRequestRetryLimit(ctx, 0)' internal/agent/sampling_request.go || { echo 'ERROR: strict provider retry=0 wiring missing' >&2; exit 3; }

REQ=(
  internal/agent/balance_v20_usage_receipt_fixed12.go
  internal/agent/balance_v20_usage_receipt_fixed12_test.go
  configs/balance_mod_v020_real_provider_manifest.json
  configs/reasonix.balance.v020.real.template.toml
  scripts/balance_mod_v020_reconcile.py
  scripts/balance_mod_v020_completion_check.py
  scripts/balance_mod_v020_real_gate.sh
  scripts/balance_mod_v020_targeted.sh
  scripts/balance_mod_smoke_quick_v020.sh
  scripts/balance_mod_v020_preflight.sh
  scripts/balance_mod_smoke_v020.sh
  docs/BALANCE_MOD_V020.md
  docs/BALANCE_MOD_V020_FIXED.md
  docs/BALANCE_MOD_V020_FIXED10.md
  docs/BALANCE_MOD_V020_FIXED12.md
)
for p in "${REQ[@]}"; do [[ -f "$PAYLOAD/$p" ]] || { echo "ERROR: payload missing $p" >&2; exit 4; }; done
bash -n "$PAYLOAD/scripts/balance_mod_v020_real_gate.sh"
PC="$(mktemp -d)"
PYTHONPYCACHEPREFIX="$PC" "$PY_BIN" -m py_compile \
  "$PAYLOAD/scripts/balance_mod_v020_reconcile.py" \
  "$PAYLOAD/scripts/balance_mod_v020_completion_check.py"
rm -rf "$PC"
"$PY_BIN" "$PAYLOAD/scripts/balance_mod_v020_reconcile.py" --self-test >/dev/null
"$PY_BIN" "$PAYLOAD/scripts/balance_mod_v020_completion_check.py" --self-test >/dev/null
echo 'v0.20 Fixed12 harness self-tests: PASS'

TMP="$(mktemp -d)"
BACKUP="$TARGET/.balance_mod_backups/v0.20-fixed12-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP"
PATHS=(
  internal/agent/run_usage.go
  internal/agent/balance_v20_usage_receipt_fixed11.go
  internal/agent/balance_v20_usage_receipt_fixed11_test.go
  internal/agent/balance_v20_usage_receipt_fixed12.go
  internal/agent/balance_v20_usage_receipt_fixed12_test.go
  configs/balance_mod_v020_real_provider_manifest.json
  configs/reasonix.balance.v020.real.template.toml
  scripts/balance_mod_v020_reconcile.py
  scripts/balance_mod_v020_completion_check.py
  scripts/balance_mod_v020_real_gate.sh
  scripts/balance_mod_v020_targeted.sh
  scripts/balance_mod_smoke_quick_v020.sh
  scripts/balance_mod_v020_preflight.sh
  scripts/balance_mod_smoke_v020.sh
  docs/BALANCE_MOD_V020.md
  docs/BALANCE_MOD_V020_FIXED.md
  docs/BALANCE_MOD_V020_FIXED10.md
  docs/BALANCE_MOD_V020_FIXED11.md
  docs/BALANCE_MOD_V020_FIXED12.md
)
for p in "${PATHS[@]}"; do
  if [[ -e "$p" ]]; then
    mkdir -p "$BACKUP/$(dirname "$p")"
    cp -a "$p" "$BACKUP/$p"
  fi
done
APPLIED=0
rollback(){
  rc=$?
  set +e
  if [[ $rc -ne 0 && $APPLIED -eq 1 ]]; then
    echo 'Fixed12 local validation failed; restoring pre-install files...' >&2
    for p in "${PATHS[@]}"; do
      if [[ -e "$BACKUP/$p" ]]; then
        mkdir -p "$(dirname "$p")"
        cp -a "$BACKUP/$p" "$p"
      else
        rm -f "$p"
      fi
    done
    echo 'Fixed12 rollback: PASS' >&2
  fi
  rm -rf "$TMP"
  exit $rc
}
trap rollback EXIT INT TERM

cp internal/agent/run_usage.go "$TMP/run_usage.go"
"$PY_BIN" - "$TMP/run_usage.go" <<'PYFIX12'
from pathlib import Path
import sys

p=Path(sys.argv[1])
s=p.read_text(encoding='utf-8')
marker='writeBalanceV20UsageReceipt(a.modelRef, usage)'
if marker in s:
    print('already-patched')
    raise SystemExit(0)

sig='func (a *Agent) emitTurnUsage('
if s.count(sig) != 1:
    raise SystemExit(f'ERROR: emitTurnUsage signature count={s.count(sig)}, expected 1')
start=s.index(sig)
brace=s.find('{', start)
if brace < 0:
    raise SystemExit('ERROR: emitTurnUsage opening brace not found')

# Find the matching closing brace while ignoring braces inside Go strings,
# rune literals, raw strings, and comments. This deliberately avoids any
# formatting/version-specific sink/return text anchor.
i=brace
depth=0
state='code'
escaped=False
end=None
while i < len(s):
    c=s[i]
    n=s[i+1] if i+1 < len(s) else ''
    if state == 'code':
        if c=='/' and n=='/': state='line'; i+=2; continue
        if c=='/' and n=='*': state='block'; i+=2; continue
        if c=='"': state='string'; escaped=False; i+=1; continue
        if c=="'": state='rune'; escaped=False; i+=1; continue
        if c=='`': state='raw'; i+=1; continue
        if c=='{': depth += 1
        elif c=='}':
            depth -= 1
            if depth == 0:
                end=i
                break
        i+=1
        continue
    if state == 'line':
        if c=='\n': state='code'
        i+=1; continue
    if state == 'block':
        if c=='*' and n=='/': state='code'; i+=2
        else: i+=1
        continue
    if state == 'raw':
        if c=='`': state='code'
        i+=1; continue
    if state in ('string','rune'):
        if escaped:
            escaped=False; i+=1; continue
        if c=='\\':
            escaped=True; i+=1; continue
        if (state=='string' and c=='"') or (state=='rune' and c=="'"):
            state='code'
        i+=1; continue

if end is None:
    raise SystemExit('ERROR: emitTurnUsage closing brace not found')
body=s[brace+1:end]
if 'a.svc.sink.Emit' not in body:
    raise SystemExit('ERROR: emitTurnUsage does not contain a.svc.sink.Emit')

hook='\twriteBalanceV20UsageReceipt(a.modelRef, usage)\n'
# Newer Reasonix returns a CostQuote after sink.Emit; the hook must execute
# before that final return. Legacy Reasonix has no final return, so place it
# immediately before the function's closing brace.
final_return='\treturn e.CostQuote\n'
rel=body.rfind(final_return)
if rel >= 0:
    insert_at=brace+1+rel
else:
    insert_at=end
    if insert_at > 0 and s[insert_at-1] != '\n':
        hook='\n'+hook
s=s[:insert_at]+hook+s[insert_at:]
p.write_text(s,encoding='utf-8')
print('patched-structurally')
PYFIX12

PATCH="$TMP/run_usage.patch"
: > "$PATCH"
if ! cmp -s internal/agent/run_usage.go "$TMP/run_usage.go"; then
  rc=0
  diff -u --label a/internal/agent/run_usage.go --label b/internal/agent/run_usage.go \
    internal/agent/run_usage.go "$TMP/run_usage.go" > "$PATCH" || rc=$?
  [[ $rc -eq 1 ]] || { echo 'ERROR: failed to create structural run_usage patch' >&2; exit 5; }
  git apply --check "$PATCH"
  echo 'v0.20 Fixed12 structural run_usage apply check: PASS'
  git apply "$PATCH"
fi
APPLIED=1

# Remove stale failed Fixed11 receipt files if they somehow exist. They are
# backed up above and restored on rollback.
rm -f internal/agent/balance_v20_usage_receipt_fixed11.go \
      internal/agent/balance_v20_usage_receipt_fixed11_test.go

for p in "${REQ[@]}"; do
  mkdir -p "$(dirname "$p")"
  tmpdst="$(dirname "$p")/.fixed12.$(basename "$p").$$"
  cp "$PAYLOAD/$p" "$tmpdst"
  chmod --reference="$PAYLOAD/$p" "$tmpdst" 2>/dev/null || true
  mv -f "$tmpdst" "$p"
done
chmod +x scripts/balance_mod_v020_*.sh scripts/balance_mod_v020_reconcile.py \
  scripts/balance_mod_v020_completion_check.py scripts/balance_mod_smoke_*v020.sh

"$(dirname "$GO_BIN")/gofmt" -w \
  internal/agent/run_usage.go \
  internal/agent/balance_v20_usage_receipt_fixed12.go \
  internal/agent/balance_v20_usage_receipt_fixed12_test.go

grep -q 'writeBalanceV20UsageReceipt(a.modelRef, usage)' internal/agent/run_usage.go || { echo 'ERROR: usage receipt tap missing' >&2; exit 6; }
grep -q 'BALANCE_V20_USAGE_RECEIPT_PATH' internal/agent/balance_v20_usage_receipt_fixed12.go || { echo 'ERROR: receipt env gate missing' >&2; exit 6; }
grep -q "HARNESS_REV='v0.20-fixed12-legacy-compatible-usage-receipt'" scripts/balance_mod_v020_real_gate.sh || { echo 'ERROR: Fixed12 gate marker missing' >&2; exit 6; }
grep -q -- '--usage-receipt "$TMP/provider-usage.json"' scripts/balance_mod_v020_real_gate.sh || { echo 'ERROR: exact receipt reconciliation wiring missing' >&2; exit 6; }

echo 'v0.20 Fixed12 targeted usage-receipt tests...'
PATH="$(dirname "$GO_BIN"):$PATH" GOTOOLCHAIN=local "$GO_BIN" test ./internal/agent \
  -run '^(TestBalanceV20UsageReceiptFixed12|TestBalanceV20UsageReceiptDisabledWithoutEnvFixed12)$' -count=1

echo 'v0.20 Fixed12 touched-package compile-only gate...'
PATH="$(dirname "$GO_BIN"):$PATH" GOTOOLCHAIN=local "$GO_BIN" test ./internal/agent ./internal/serve -run '^$' -count=1
PATH="$(dirname "$GO_BIN"):$PATH" GOTOOLCHAIN=local CGO_ENABLED=0 "$GO_BIN" build -o "$TMP/reasonix-fixed12" ./cmd/reasonix

git diff --check
mkdir -p bin
mv -f "$TMP/reasonix-fixed12" bin/reasonix
chmod +x bin/reasonix
APPLIED=0
trap - EXIT INT TERM
rm -rf "$TMP"
echo 'v0.20 Fixed12 local validation: PASS'
echo '  Fixed9 hard-budget core: PRESERVED'
echo '  Fixed10 semantic completion: PRESERVED'
echo '  legacy/new emitTurnUsage structural hook: PASS'
echo '  exact provider Usage receipt tap: PASS'
echo '  receipt outside explicit gate env: NO-OP'
echo '  exact usage reconciliation wiring: PASS'
echo "Backup: $BACKUP"
echo 'Installer made NO DeepSeek model/provider call.'
echo "ONLINE FINAL: cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_v020.sh"
