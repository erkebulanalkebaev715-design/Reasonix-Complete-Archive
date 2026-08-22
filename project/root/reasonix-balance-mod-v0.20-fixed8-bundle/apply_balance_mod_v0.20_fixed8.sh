#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PAYLOAD="$HERE/payload"
PY_BIN="${PY_BIN:-python3}"
GO_BIN="${GO_BIN:-/usr/local/go/bin/go}"
GOFMT_BIN="${GOFMT_BIN:-/usr/local/go/bin/gofmt}"

[[ -d "$TARGET/.git" ]] || { echo "ERROR: Reasonix git tree not found: $TARGET" >&2; exit 2; }
cd "$TARGET"
for p in internal/serve/mod_bridge.go internal/agent/run_budget.go internal/agent/sampling_request.go internal/agent/reasoning_replay.go internal/agent/run_loop.go internal/provider/retry.go; do
  [[ -f "$p" ]] || { echo "ERROR: required core file missing: $p" >&2; exit 3; }
done
grep -q 'const balanceModVersion = "balance-mod-v0.20"' internal/serve/mod_bridge.go || { echo 'ERROR: Fixed8 requires installed balance-mod-v0.20' >&2; exit 3; }
[[ -x "$GO_BIN" ]] || { echo "ERROR: Go not executable: $GO_BIN" >&2; exit 3; }
[[ -x "$GOFMT_BIN" ]] || { echo "ERROR: gofmt not executable: $GOFMT_BIN" >&2; exit 3; }

STOPPED="$($PY_BIN - <<'PYINNER'
import os,signal
me=os.getpid(); parent=os.getppid(); stopped=0
for name in os.listdir('/proc'):
    if not name.isdigit(): continue
    pid=int(name)
    if pid in (me,parent): continue
    try: argv=[x.decode('utf-8','replace') for x in open(f'/proc/{pid}/cmdline','rb').read().split(b'\0') if x]
    except Exception: continue
    if not argv: continue
    gate=any(os.path.basename(a)=='balance_mod_v020_real_gate.sh' for a in argv)
    reasonix=('serve' in argv and any(a=='deepseek-v20/deepseek-v4-flash' for a in argv))
    if gate or reasonix:
        try: os.kill(pid,signal.SIGTERM); stopped+=1
        except (ProcessLookupError,PermissionError): pass
print(stopped)
PYINNER
)"
sleep 0.25
rm -rf /tmp/reasonix-v020-real-gate.lock 2>/dev/null || true
echo "v0.20 stale online-gate cleanup: PASS (stopped=$STOPPED)"

PATHS=(
  configs/balance_mod_v020_real_provider_manifest.json
  configs/reasonix.balance.v020.real.template.toml
  docs/BALANCE_MOD_V020.md
  docs/BALANCE_MOD_V020_FIXED.md
  docs/BALANCE_MOD_V020_FIXED8.md
  scripts/balance_mod_v020_reconcile.py
  scripts/balance_mod_v020_real_gate.sh
  scripts/balance_mod_v020_targeted.sh
  scripts/balance_mod_smoke_quick_v020.sh
  scripts/balance_mod_v020_preflight.sh
  scripts/balance_mod_smoke_v020.sh
  internal/agent/balance_strict_retry_fixed8_test.go
  internal/provider/balance_strict_retry_fixed8_test.go
)
for p in "${PATHS[@]}"; do [[ -f "$PAYLOAD/$p" ]] || { echo "ERROR: payload missing $p" >&2; exit 2; }; done

# Previous v0.20 hotfixes used versioned test filenames while keeping the same
# Go test function names. Keeping two generations side-by-side makes Go reject
# the package before production code is even tested. Treat ONLY these generated
# balance-mod test files as replaceable installer state; production *_test.go
# files are never matched or removed.
shopt -s nullglob
STALE_TESTS=()
for p in internal/agent/balance_strict_retry_fixed*_test.go internal/provider/balance_strict_retry_fixed*_test.go; do
  case "$p" in
    internal/agent/balance_strict_retry_fixed8_test.go|internal/provider/balance_strict_retry_fixed8_test.go) ;;
    *) STALE_TESTS+=("$p") ;;
  esac
done
shopt -u nullglob

TMP="$(mktemp -d)"
PATCH="$TMP/v020-fixed8.patch"
BACKUP="$TARGET/.balance_mod_backups/v0.20-fixed8-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP"
APPLIED=0
VALIDATED=0
CORE_PATHS=( internal/agent/run_budget.go internal/agent/sampling_request.go internal/agent/reasoning_replay.go internal/agent/run_loop.go internal/provider/retry.go )

rollback_on_error() {
  rc=$?
  set +e
  if [[ $rc -ne 0 && $APPLIED -eq 1 && $VALIDATED -eq 0 ]]; then
    echo "Fixed8 local validation failed; restoring pre-install files..." >&2
    for p in "${CORE_PATHS[@]}" "${PATHS[@]}" "${STALE_TESTS[@]}"; do
      if [[ -e "$BACKUP/$p" ]]; then
        mkdir -p "$(dirname "$p")"; cp -a "$BACKUP/$p" "$p"
      else
        rm -f "$p"
      fi
    done
    rm -rf scripts/__pycache__ 2>/dev/null || true
    echo 'Fixed8 rollback: PASS' >&2
  fi
  rm -rf "$TMP"
  exit $rc
}
trap rollback_on_error EXIT INT TERM

mkdir -p "$TMP/core/internal/agent" "$TMP/core/internal/provider"
for p in "${CORE_PATHS[@]}"; do mkdir -p "$TMP/core/$(dirname "$p")"; cp "$p" "$TMP/core/$p"; done

"$PY_BIN" - "$TMP/core" <<'PYCORE'
from pathlib import Path
import re,sys
root=Path(sys.argv[1])

# 1) Hard pre-call budget: remove obsolete 6*11 = 66 retry-envelope division.
p=root/'internal/agent/run_budget.go'; s=p.read_text()
pat=re.compile(r'// strictPreCallRetryFactor reserves enough room for the complete bounded retry.*?strictPreCallInputChargeMultiplier\s*=\s*2\.0\s*\n\)', re.S)
new="""// strict-one-paid-attempt-v0.20-fixed8: strict hard-budget mode allows exactly
// the current provider attempt against the current remaining allowance. Hidden
// provider/body/reasoning retries are disabled below, so a later paid attempt
// must return through host admission after ledger reconciliation.
const strictPreCallInputChargeMultiplier = 2.0"""
if pat.search(s):
    s=pat.sub(new,s,count=1)
elif any(m in s for m in ('strict-one-paid-attempt-v0.20-fixed5','strict-one-paid-attempt-v0.20-fixed6','strict-one-paid-attempt-v0.20-fixed7')):
    for m in ('strict-one-paid-attempt-v0.20-fixed5','strict-one-paid-attempt-v0.20-fixed6','strict-one-paid-attempt-v0.20-fixed7'):
        s=s.replace(m,'strict-one-paid-attempt-v0.20-fixed8')
elif 'strict-one-paid-attempt-v0.20-fixed8' not in s:
    raise SystemExit('ERROR: unrecognized run_budget strict-precall state')
s=s.replace('share := remaining / strictPreCallRetryFactor','share := remaining')
s=s.replace('share := remaining / float64(strictPreCallRetryFactor)','share := remaining')
s=s.replace('retry-reserved share %d <= prompt upper bound %d','current-attempt allowance %d <= prompt upper bound %d')
s=s.replace('retry-reserved share %.6f %s','current-attempt allowance %.6f %s')
if 'strictPreCallRetryFactor' in s: raise SystemExit('ERROR: obsolete strictPreCallRetryFactor survived Fixed8')
if 'strict-one-paid-attempt-v0.20-fixed8' not in s: raise SystemExit('ERROR: Fixed8 run_budget marker missing')
p.write_text(s)

# 2) Provider connection/header retries become context-controllable. Strict = zero retries.
p=root/'internal/provider/retry.go'; s=p.read_text()
if 'WithRequestRetryLimit' not in s:
    anchor='type requestAttemptCounterKey struct{}\n'
    if anchor not in s: raise SystemExit('ERROR: retry.go requestAttemptCounterKey anchor missing')
    helper='''type requestAttemptCounterKey struct{}
type requestRetryLimitKey struct{}

// WithRequestRetryLimit caps SendWithRetry retries for this logical request.
// A value of 0 means exactly one HTTP attempt. The default remains MaxRetries.
func WithRequestRetryLimit(ctx context.Context, max int) context.Context {
\tif ctx == nil { ctx = context.Background() }
\tif max < 0 { max = 0 }
\tif max > MaxRetries { max = MaxRetries }
\treturn context.WithValue(ctx, requestRetryLimitKey{}, max)
}

func requestRetryLimit(ctx context.Context) int {
\tif ctx != nil {
\t\tif max, ok := ctx.Value(requestRetryLimitKey{}).(int); ok {
\t\t\tif max < 0 { return 0 }
\t\t\tif max > MaxRetries { return MaxRetries }
\t\t\treturn max
\t\t}
\t}
\treturn MaxRetries
}
'''
    s=s.replace(anchor,helper,1)
anchor2='func SendWithRetry(ctx context.Context, httpClient *http.Client, opts SendOptions, newReq func(context.Context) (*http.Request, error)) (*http.Response, error) {\n\tnotify := retryNotifyFromContext(ctx)\n'
if 'maxRetries := requestRetryLimit(ctx)' not in s:
    if anchor2 not in s: raise SystemExit('ERROR: SendWithRetry anchor missing')
    s=s.replace(anchor2,anchor2+'\tmaxRetries := requestRetryLimit(ctx)\n',1)
s=s.replace('for attempt := 0; attempt <= MaxRetries; attempt++ {','for attempt := 0; attempt <= maxRetries; attempt++ {',1)
s=s.replace('RetryInfo{Attempt: attempt, Max: MaxRetries, Delay: delay, Err: lastErr}','RetryInfo{Attempt: attempt, Max: maxRetries, Delay: delay, Err: lastErr}',1)
if 'for attempt := 0; attempt <= maxRetries; attempt++ {' not in s: raise SystemExit('ERROR: retry limit is not wired into SendWithRetry')
p.write_text(s)

# 3) Every strict provider call gets retry limit zero; body stream replay disabled.
p=root/'internal/agent/sampling_request.go'; s=p.read_text()
old='''func (a *Agent) streamProviderRequest(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
\tif err := a.applyStrictPreCallBudget(ctx, &req); err != nil {
\t\treturn nil, err
\t}
\treturn a.svc.prov.Stream(ctx, req)
}'''
new='''func (a *Agent) streamProviderRequest(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
\tif err := a.applyStrictPreCallBudget(ctx, &req); err != nil {
\t\treturn nil, err
\t}
\tif a.StrictPreCallBudget() {
\t\t// strict-one-paid-attempt-v0.20-fixed8: no hidden connection/header retry.
\t\tctx = provider.WithRequestRetryLimit(ctx, 0)
\t}
\treturn a.svc.prov.Stream(ctx, req)
}'''
if old in s: s=s.replace(old,new,1)
elif 'provider.WithRequestRetryLimit(ctx, 0)' in s:
    
    for m in ('strict-one-paid-attempt-v0.20-fixed5','strict-one-paid-attempt-v0.20-fixed6','strict-one-paid-attempt-v0.20-fixed7'):
        s=s.replace(m,'strict-one-paid-attempt-v0.20-fixed8')
else: raise SystemExit('ERROR: unrecognized streamProviderRequest strict state')
s=s.replace('if provider.IsStreamInterrupted(result.err) && attempt < maxSamplingAttempts {','if !a.StrictPreCallBudget() && provider.IsStreamInterrupted(result.err) && attempt < maxSamplingAttempts {',1)
if 'if !a.StrictPreCallBudget() && provider.IsStreamInterrupted(result.err)' not in s: raise SystemExit('ERROR: strict body replay suppression missing')
p.write_text(s)

# 4) Missing-reasoning fallback cannot open a hidden second paid request.
p=root/'internal/agent/reasoning_replay.go'; s=p.read_text()
if 'strict-one-paid-attempt-v0.20-fixed8' not in s:
    if any(m in s for m in ('strict-one-paid-attempt-v0.20-fixed5','strict-one-paid-attempt-v0.20-fixed6','strict-one-paid-attempt-v0.20-fixed7')):
        for m in ('strict-one-paid-attempt-v0.20-fixed5','strict-one-paid-attempt-v0.20-fixed6','strict-one-paid-attempt-v0.20-fixed7'):
            s=s.replace(m,'strict-one-paid-attempt-v0.20-fixed8')
    else:
        anchor=') (streamedTurn, bool) {\n\tif !a.activateMissingReasoningFallback() {'
        if anchor not in s: raise SystemExit('ERROR: reasoning fallback anchor missing')
        repl=') (streamedTurn, bool) {\n\tif a.StrictPreCallBudget() {\n\t\t// strict-one-paid-attempt-v0.20-fixed8: fallback would be a second paid call.\n\t\treturn streamedTurn{}, false\n\t}\n\tif !a.activateMissingReasoningFallback() {'
        s=s.replace(anchor,repl,1)
if 'if a.StrictPreCallBudget()' not in s: raise SystemExit('ERROR: strict reasoning fallback guard missing')
p.write_text(s)

# 5) Exact reasoning replay is also another paid request: suppress in strict mode.
p=root/'internal/agent/run_loop.go'; s=p.read_text()
if 'strict-one-paid-attempt-v0.20-fixed8' not in s:
    if any(m in s for m in ('strict-one-paid-attempt-v0.20-fixed5','strict-one-paid-attempt-v0.20-fixed6','strict-one-paid-attempt-v0.20-fixed7')):
        for m in ('strict-one-paid-attempt-v0.20-fixed5','strict-one-paid-attempt-v0.20-fixed6','strict-one-paid-attempt-v0.20-fixed7'):
            s=s.replace(m,'strict-one-paid-attempt-v0.20-fixed8')
    else:
        pat=re.compile(r'(\t\tmissing, shouldRetry := false, false\n\t\tswitch issue \{.*?\n\t\t\}\n)(\t\tif issue == ReasoningReplayOverflow)',re.S)
        m=pat.search(s)
        if not m: raise SystemExit('ERROR: missing-reasoning switch block not found')
        injected=m.group(1)+'''\t\tif a.StrictPreCallBudget() {\n\t\t\t// strict-one-paid-attempt-v0.20-fixed8: replay is another paid request.\n\t\t\tshouldRetry = false\n\t\t}\n'''+m.group(2)
        s=s[:m.start()]+injected+s[m.end():]
if 'shouldRetry = false' not in s: raise SystemExit('ERROR: strict reasoning replay suppression missing')
p.write_text(s)
PYCORE

# Back up everything that can be replaced.
for p in "${CORE_PATHS[@]}" "${PATHS[@]}" "${STALE_TESTS[@]}"; do
  if [[ -e "$p" ]]; then mkdir -p "$BACKUP/$(dirname "$p")"; cp -a "$p" "$BACKUP/$p"; fi
done

: > "$PATCH"
for p in "${CORE_PATHS[@]}"; do
  rc=0; diff -u --label "a/$p" --label "b/$p" "$p" "$TMP/core/$p" >> "$PATCH" || rc=$?
  [[ "$rc" == 0 || "$rc" == 1 ]] || { echo "ERROR: core diff failed: $p" >&2; exit 4; }
done
for p in "${PATHS[@]}"; do
  if [[ -e "$p" ]]; then
    cmp -s "$p" "$PAYLOAD/$p" && continue
    rc=0; diff -u --label "a/$p" --label "b/$p" "$p" "$PAYLOAD/$p" >> "$PATCH" || rc=$?
  else
    rc=0; diff -u --label /dev/null --label "b/$p" /dev/null "$PAYLOAD/$p" >> "$PATCH" || rc=$?
  fi
  [[ "$rc" == 1 ]] || { echo "ERROR: payload diff failed: $p" >&2; exit 4; }
done

if [[ -s "$PATCH" ]]; then
  git apply --check "$PATCH" || { echo 'ERROR: Fixed8 exact apply check failed; refusing fuzzy apply' >&2; exit 4; }
  echo 'v0.20 Fixed8 exact apply check: PASS'
  git apply "$PATCH"
  APPLIED=1
else
  echo 'v0.20 Fixed8 already exact: PASS'
fi

if (( ${#STALE_TESTS[@]} > 0 )); then
  APPLIED=1
  echo "v0.20 Fixed8 stale generated-test cleanup: ${#STALE_TESTS[@]} file(s)"
  rm -f -- "${STALE_TESTS[@]}"
fi
# Fail closed if any previous generated strict-retry test survived.
if find internal/agent internal/provider -maxdepth 1 -type f \
    -name 'balance_strict_retry_fixed*_test.go' \
    ! -name 'balance_strict_retry_fixed8_test.go' -print -quit | grep -q .; then
  echo 'ERROR: stale balance strict-retry test survived cleanup' >&2
  exit 5
fi

"$GOFMT_BIN" -w "${CORE_PATHS[@]}" internal/agent/balance_strict_retry_fixed8_test.go internal/provider/balance_strict_retry_fixed8_test.go
chmod +x scripts/balance_mod_v020_*.sh scripts/balance_mod_v020_reconcile.py scripts/balance_mod_smoke_*v020.sh
bash -n scripts/balance_mod_v020_real_gate.sh
bash -n scripts/balance_mod_smoke_v020.sh
PYTHONPYCACHEPREFIX="$TMP/pycache" "$PY_BIN" -m py_compile scripts/balance_mod_v020_reconcile.py
"$PY_BIN" scripts/balance_mod_v020_reconcile.py --self-test >/dev/null
git diff --check

echo 'v0.20 Fixed8 targeted tests...'
PATH="$(dirname "$GO_BIN"):$PATH" GOTOOLCHAIN=local "$GO_BIN" test ./internal/provider -run '^TestBalanceStrictRetryLimitZeroStartsOneHTTPAttempt$' -count=1
PATH="$(dirname "$GO_BIN"):$PATH" GOTOOLCHAIN=local "$GO_BIN" test ./internal/agent -run '^(TestBalanceStrictPreCallUsesCurrentAttemptEnvelope|TestBalanceStrictReasoningFallbackSuppressed)$' -count=1

echo 'v0.20 Fixed8 package compile/regression...'
PATH="$(dirname "$GO_BIN"):$PATH" GOTOOLCHAIN=local "$GO_BIN" test ./internal/provider ./internal/agent ./internal/serve
PATH="$(dirname "$GO_BIN"):$PATH" GOTOOLCHAIN=local CGO_ENABLED=0 "$GO_BIN" build -o "$TMP/reasonix-fixed8" ./cmd/reasonix
[[ -x "$TMP/reasonix-fixed8" ]] || { echo 'ERROR: Fixed8 temporary CLI build missing' >&2; exit 5; }

grep -q 'strict-one-paid-attempt-v0.20-fixed8' internal/agent/run_budget.go || { echo 'ERROR: Fixed8 marker missing after validation' >&2; exit 5; }
! grep -q 'strictPreCallRetryFactor' internal/agent/run_budget.go || { echo 'ERROR: obsolete 66-way reserve survived validation' >&2; exit 5; }
mkdir -p bin
BIN_TMP="bin/.reasonix-fixed8.$$"
cp "$TMP/reasonix-fixed8" "$BIN_TMP"
chmod 0755 "$BIN_TMP"
mv -f "$BIN_TMP" bin/reasonix
VALIDATED=1

echo 'v0.20 Fixed8 local validation: PASS'
echo '  stale generated mod tests: CLEAN'
echo '  obsolete 66-way reserve: REMOVED'
echo '  hidden provider retries under strict budget: 0'
echo '  hidden body/reasoning replay under strict budget: OFF'
echo '  current DeepSeek V4 Flash pricing loaded in real-gate config'
echo "Backup: $BACKUP"
echo 'Installer made NO DeepSeek model/provider call.'
echo "ONLINE FINAL: cd '$TARGET' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_v020.sh"
trap - EXIT INT TERM
rm -rf "$TMP"
