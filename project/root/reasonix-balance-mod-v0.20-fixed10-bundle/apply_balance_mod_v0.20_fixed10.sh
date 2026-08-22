#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PAYLOAD="$HERE/payload"
PY_BIN="${PY_BIN:-python3}"
GO_BIN="${GO_BIN:-/usr/local/go/bin/go}"

[[ -d "$TARGET/.git" ]] || { echo "ERROR: Reasonix git tree not found: $TARGET" >&2; exit 2; }
cd "$TARGET"
[[ -f internal/serve/mod_bridge.go ]] || { echo 'ERROR: mod_bridge.go missing' >&2; exit 3; }
grep -q 'const balanceModVersion = "balance-mod-v0.20"' internal/serve/mod_bridge.go || { echo 'ERROR: Fixed10 requires balance-mod-v0.20' >&2; exit 3; }
grep -q 'strict-one-paid-attempt-v0.20-fixed9' internal/agent/run_budget.go || { echo 'ERROR: Fixed10 requires validated Fixed9 hard-budget core' >&2; exit 3; }
! grep -q 'strictPreCallRetryFactor' internal/agent/run_budget.go || { echo 'ERROR: obsolete 66-way reserve still present' >&2; exit 3; }
grep -q 'WithRequestRetryLimit(ctx, 0)' internal/agent/sampling_request.go || { echo 'ERROR: strict provider retry=0 wiring missing' >&2; exit 3; }
grep -q '!a.StrictPreCallBudget() && provider.IsStreamInterrupted' internal/agent/sampling_request.go || { echo 'ERROR: strict body replay suppression missing' >&2; exit 3; }

STOPPED="$($PY_BIN - <<'PY'
import os,signal
me=os.getpid(); parent=os.getppid(); n=0
for name in os.listdir('/proc'):
    if not name.isdigit(): continue
    pid=int(name)
    if pid in (me,parent): continue
    try: argv=[x.decode('utf-8','replace') for x in open(f'/proc/{pid}/cmdline','rb').read().split(b'\0') if x]
    except Exception: continue
    if not argv: continue
    gate=any(os.path.basename(a)=='balance_mod_v020_real_gate.sh' for a in argv)
    srv=('serve' in argv and any(a=='deepseek-v20/deepseek-v4-flash' for a in argv))
    if gate or srv:
        try: os.kill(pid,signal.SIGTERM); n+=1
        except (ProcessLookupError,PermissionError): pass
print(n)
PY
)"
sleep 0.2
rm -rf /tmp/reasonix-v020-real-gate.lock 2>/dev/null || true
echo "v0.20 stale online-gate cleanup: PASS (stopped=$STOPPED)"

PATHS=(
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
)
for p in "${PATHS[@]}"; do [[ -f "$PAYLOAD/$p" ]] || { echo "ERROR: payload missing $p" >&2; exit 4; }; done

# Validate only the harness payload. Fixed10 changes no Go source, and the
# installed Fixed9 core was already validated by its own installer.
bash -n "$PAYLOAD/scripts/balance_mod_v020_real_gate.sh"
bash -n "$PAYLOAD/scripts/balance_mod_smoke_v020.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM
PYTHONPYCACHEPREFIX="$TMP/pycache" "$PY_BIN" -m py_compile \
  "$PAYLOAD/scripts/balance_mod_v020_reconcile.py" \
  "$PAYLOAD/scripts/balance_mod_v020_completion_check.py"
"$PY_BIN" "$PAYLOAD/scripts/balance_mod_v020_reconcile.py" --self-test >/dev/null
"$PY_BIN" "$PAYLOAD/scripts/balance_mod_v020_completion_check.py" --self-test >/dev/null
echo 'v0.20 Fixed10 harness self-tests: PASS'

BACKUP="$TARGET/.balance_mod_backups/v0.20-fixed10-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP"
for p in "${PATHS[@]}"; do
  if [[ -e "$p" ]]; then mkdir -p "$BACKUP/$(dirname "$p")"; cp -a "$p" "$BACKUP/$p"; fi
done

APPLIED=0
rollback() {
  rc=$?
  set +e
  if [[ $rc -ne 0 && $APPLIED -eq 1 ]]; then
    echo 'Fixed10 install failed; restoring harness files...' >&2
    for p in "${PATHS[@]}"; do
      if [[ -e "$BACKUP/$p" ]]; then mkdir -p "$(dirname "$p")"; cp -a "$BACKUP/$p" "$p"; else rm -f "$p"; fi
    done
    echo 'Fixed10 rollback: PASS' >&2
  fi
  rm -rf "$TMP"
  exit $rc
}
trap rollback EXIT INT TERM

CHANGED=0
for p in "${PATHS[@]}"; do
  if [[ -e "$p" ]] && cmp -s "$p" "$PAYLOAD/$p"; then continue; fi
  CHANGED=1
  mkdir -p "$(dirname "$p")"
  tmpdst="$(dirname "$p")/.fixed10.$(basename "$p").$$"
  cp "$PAYLOAD/$p" "$tmpdst"
  chmod --reference="$PAYLOAD/$p" "$tmpdst" 2>/dev/null || true
  mv -f "$tmpdst" "$p"
done
APPLIED=$CHANGED
chmod +x scripts/balance_mod_v020_*.sh scripts/balance_mod_v020_reconcile.py scripts/balance_mod_smoke_*v020.sh

grep -q "HARNESS_REV='v0.20-fixed10-semantic-completion'" scripts/balance_mod_v020_real_gate.sh || { echo 'ERROR: Fixed10 harness marker missing after install' >&2; exit 5; }
grep -q 'clean provider completion + positive ledger spend' scripts/balance_mod_v020_real_gate.sh || { echo 'ERROR: Fixed10 semantic completion gate missing' >&2; exit 5; }
grep -q 'v020-fixed10-last' scripts/balance_mod_v020_real_gate.sh || { echo 'ERROR: Fixed10 diagnostic snapshot path missing' >&2; exit 5; }
bash -n scripts/balance_mod_v020_real_gate.sh
git diff --check

APPLIED=0
trap - EXIT INT TERM
rm -rf "$TMP"
echo 'v0.20 Fixed10 harness validation: PASS'
echo '  Fixed9 hard-budget core: PRESERVED'
echo '  provider success criterion: chat.message + clean turn.done + positive spend'
echo '  exact marker: informational (live/SSE visibility only)'
echo '  generic terminal data.error/cancelled: FAIL-FAST'
echo '  failure diagnostics: PRESERVED + SANITIZED'
echo "Backup: $BACKUP"
echo 'Installer made NO DeepSeek model/provider call.'
echo "ONLINE FINAL: cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_v020.sh"
