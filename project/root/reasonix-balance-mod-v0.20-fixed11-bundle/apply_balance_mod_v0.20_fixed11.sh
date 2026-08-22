#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PAYLOAD="$HERE/payload"
PY_BIN="${PY_BIN:-python3}"
GO_BIN="${GO_BIN:-/usr/local/go/bin/go}"
[[ -d "$TARGET/.git" ]] || { echo "ERROR: Reasonix git tree not found: $TARGET" >&2; exit 2; }
cd "$TARGET"
for f in internal/agent/run_usage.go internal/agent/run_budget.go internal/agent/sampling_request.go internal/serve/mod_bridge.go; do [[ -f "$f" ]] || { echo "ERROR: required core file missing: $f" >&2; exit 3; }; done
grep -q 'const balanceModVersion = "balance-mod-v0.20"' internal/serve/mod_bridge.go || { echo 'ERROR: v0.20 marker missing' >&2; exit 3; }
grep -q 'strict-one-paid-attempt-v0.20-fixed9' internal/agent/run_budget.go || { echo 'ERROR: validated Fixed9 hard-budget core missing' >&2; exit 3; }
! grep -q 'strictPreCallRetryFactor' internal/agent/run_budget.go || { echo 'ERROR: obsolete 66-way reserve present' >&2; exit 3; }
grep -q 'WithRequestRetryLimit(ctx, 0)' internal/agent/sampling_request.go || { echo 'ERROR: strict provider retry=0 wiring missing' >&2; exit 3; }
REQ=(internal/agent/balance_v20_usage_receipt_fixed11.go internal/agent/balance_v20_usage_receipt_fixed11_test.go configs/balance_mod_v020_real_provider_manifest.json configs/reasonix.balance.v020.real.template.toml scripts/balance_mod_v020_reconcile.py scripts/balance_mod_v020_completion_check.py scripts/balance_mod_v020_real_gate.sh scripts/balance_mod_v020_targeted.sh scripts/balance_mod_smoke_quick_v020.sh scripts/balance_mod_v020_preflight.sh scripts/balance_mod_smoke_v020.sh docs/BALANCE_MOD_V020.md docs/BALANCE_MOD_V020_FIXED.md docs/BALANCE_MOD_V020_FIXED10.md docs/BALANCE_MOD_V020_FIXED11.md)
for p in "${REQ[@]}"; do [[ -f "$PAYLOAD/$p" ]] || { echo "ERROR: payload missing $p" >&2; exit 4; }; done
bash -n "$PAYLOAD/scripts/balance_mod_v020_real_gate.sh"
PC="$(mktemp -d)"; PYTHONPYCACHEPREFIX="$PC" "$PY_BIN" -m py_compile "$PAYLOAD/scripts/balance_mod_v020_reconcile.py" "$PAYLOAD/scripts/balance_mod_v020_completion_check.py"; rm -rf "$PC"
"$PY_BIN" "$PAYLOAD/scripts/balance_mod_v020_reconcile.py" --self-test >/dev/null
"$PY_BIN" "$PAYLOAD/scripts/balance_mod_v020_completion_check.py" --self-test >/dev/null
echo 'v0.20 Fixed11 harness self-tests: PASS'
TMP="$(mktemp -d)"; BACKUP="$TARGET/.balance_mod_backups/v0.20-fixed11-$(date +%Y%m%d-%H%M%S)"; mkdir -p "$BACKUP"
PATHS=(internal/agent/run_usage.go internal/agent/balance_v20_usage_receipt_fixed11.go internal/agent/balance_v20_usage_receipt_fixed11_test.go configs/balance_mod_v020_real_provider_manifest.json configs/reasonix.balance.v020.real.template.toml scripts/balance_mod_v020_reconcile.py scripts/balance_mod_v020_completion_check.py scripts/balance_mod_v020_real_gate.sh scripts/balance_mod_v020_targeted.sh scripts/balance_mod_smoke_quick_v020.sh scripts/balance_mod_v020_preflight.sh scripts/balance_mod_smoke_v020.sh docs/BALANCE_MOD_V020.md docs/BALANCE_MOD_V020_FIXED.md docs/BALANCE_MOD_V020_FIXED10.md docs/BALANCE_MOD_V020_FIXED11.md)
for p in "${PATHS[@]}"; do if [[ -e "$p" ]]; then mkdir -p "$BACKUP/$(dirname "$p")"; cp -a "$p" "$BACKUP/$p"; fi; done
APPLIED=0
rollback(){ rc=$?; set +e; if [[ $rc -ne 0 && $APPLIED -eq 1 ]]; then echo 'Fixed11 local validation failed; restoring pre-install files...' >&2; for p in "${PATHS[@]}"; do if [[ -e "$BACKUP/$p" ]]; then mkdir -p "$(dirname "$p")"; cp -a "$BACKUP/$p" "$p"; else rm -f "$p"; fi; done; echo 'Fixed11 rollback: PASS' >&2; fi; rm -rf "$TMP"; exit $rc; }
trap rollback EXIT INT TERM
cp internal/agent/run_usage.go "$TMP/run_usage.go"
"$PY_BIN" - "$TMP/run_usage.go" <<'PYFIX11'
from pathlib import Path
import sys
p=Path(sys.argv[1]); s=p.read_text(encoding='utf-8'); marker='writeBalanceV20UsageReceipt(a.modelRef, usage)'
if marker in s: print('already-patched'); raise SystemExit
old='\ta.svc.sink.Emit(e)\n\treturn e.CostQuote\n'; new='\ta.svc.sink.Emit(e)\n\twriteBalanceV20UsageReceipt(a.modelRef, usage)\n\treturn e.CostQuote\n'
count=s.count(old)
if count!=1: raise SystemExit(f'ERROR: exact emitTurnUsage sink/return anchor count={count}, expected 1')
p.write_text(s.replace(old,new,1),encoding='utf-8'); print('patched')
PYFIX11
PATCH="$TMP/run_usage.patch"; : > "$PATCH"
if ! cmp -s internal/agent/run_usage.go "$TMP/run_usage.go"; then rc=0; diff -u --label a/internal/agent/run_usage.go --label b/internal/agent/run_usage.go internal/agent/run_usage.go "$TMP/run_usage.go" > "$PATCH" || rc=$?; [[ $rc -eq 1 ]] || { echo 'ERROR: failed to create exact run_usage patch' >&2; exit 5; }; git apply --check "$PATCH"; echo 'v0.20 Fixed11 exact run_usage apply check: PASS'; git apply "$PATCH"; fi
APPLIED=1
for p in "${REQ[@]}"; do mkdir -p "$(dirname "$p")"; tmpdst="$(dirname "$p")/.fixed11.$(basename "$p").$$"; cp "$PAYLOAD/$p" "$tmpdst"; chmod --reference="$PAYLOAD/$p" "$tmpdst" 2>/dev/null || true; mv -f "$tmpdst" "$p"; done
chmod +x scripts/balance_mod_v020_*.sh scripts/balance_mod_v020_reconcile.py scripts/balance_mod_v020_completion_check.py scripts/balance_mod_smoke_*v020.sh
"$(dirname "$GO_BIN")/gofmt" -w internal/agent/run_usage.go internal/agent/balance_v20_usage_receipt_fixed11.go internal/agent/balance_v20_usage_receipt_fixed11_test.go
grep -q 'writeBalanceV20UsageReceipt(a.modelRef, usage)' internal/agent/run_usage.go || { echo 'ERROR: usage receipt tap missing' >&2; exit 6; }
grep -q 'BALANCE_V20_USAGE_RECEIPT_PATH' internal/agent/balance_v20_usage_receipt_fixed11.go || { echo 'ERROR: receipt env gate missing' >&2; exit 6; }
grep -q "HARNESS_REV='v0.20-fixed11-exact-usage-receipt'" scripts/balance_mod_v020_real_gate.sh || { echo 'ERROR: Fixed11 gate marker missing' >&2; exit 6; }
grep -q -- '--usage-receipt "$TMP/provider-usage.json"' scripts/balance_mod_v020_real_gate.sh || { echo 'ERROR: exact receipt reconciliation wiring missing' >&2; exit 6; }
echo 'v0.20 Fixed11 targeted usage-receipt tests...'
PATH="$(dirname "$GO_BIN"):$PATH" GOTOOLCHAIN=local "$GO_BIN" test ./internal/agent -run '^(TestBalanceV20UsageReceiptFixed11|TestBalanceV20UsageReceiptDisabledWithoutEnvFixed11)$' -count=1
echo 'v0.20 Fixed11 touched-package compile-only gate...'
PATH="$(dirname "$GO_BIN"):$PATH" GOTOOLCHAIN=local "$GO_BIN" test ./internal/agent ./internal/serve -run '^$' -count=1
PATH="$(dirname "$GO_BIN"):$PATH" GOTOOLCHAIN=local CGO_ENABLED=0 "$GO_BIN" build -o "$TMP/reasonix-fixed11" ./cmd/reasonix
git diff --check; mkdir -p bin; mv -f "$TMP/reasonix-fixed11" bin/reasonix; chmod +x bin/reasonix
APPLIED=0; trap - EXIT INT TERM; rm -rf "$TMP"
echo 'v0.20 Fixed11 local validation: PASS'; echo '  Fixed9 hard-budget core: PRESERVED'; echo '  Fixed10 semantic completion: PRESERVED'; echo '  exact provider Usage receipt tap: PASS'; echo '  receipt outside explicit gate env: NO-OP'; echo '  exact usage reconciliation wiring: PASS'; echo "Backup: $BACKUP"; echo 'Installer made NO DeepSeek model/provider call.'; echo "ONLINE FINAL: cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_v020.sh"
