#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$HOME/DeepSeek-Reasonix}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH="$HERE/balance_mod_v0.16.patch"
TEST_FILE="$ROOT/internal/agent/precall_budget_test.go"

if ! git -C "$ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "ERROR: Reasonix git tree not found: $ROOT" >&2
  exit 1
fi
[[ -f "$PATCH" ]] || { echo "ERROR: patch missing: $PATCH" >&2; exit 1; }

# Install the v0.16 runtime only when it is not already present.  This also
# makes the bundle safe if an earlier v0.16 installer was already run.
if grep -q 'balance-mod-v0.16' "$ROOT/internal/serve/mod_bridge.go" 2>/dev/null; then
  echo "Balance Mod v0.16 runtime already present; checking corrected test wiring."
else
  if ! git -C "$ROOT" apply --check "$PATCH"; then
    echo "ERROR: v0.16 patch does not cleanly apply. No v0.16 runtime files were changed." >&2
    echo "Expected base: the v0.15 tree that passed BALANCE_MOD_V15_SMOKE_PASS." >&2
    exit 1
  fi
  git -C "$ROOT" apply "$PATCH"
  echo "Balance Mod v0.16 runtime applied."
fi

if [[ ! -f "$TEST_FILE" ]]; then
  echo "ERROR: expected v0.16 test file missing: $TEST_FILE" >&2
  exit 1
fi

# Correct the one stale constructor call shipped in the first v0.16 bundle.
python3 - "$TEST_FILE" <<'PY'
from pathlib import Path
import sys
p = Path(sys.argv[1])
s = p.read_text()
needle = 'NewCoordinator('
changes = 0
pos = 0
out = []
while True:
    i = s.find(needle, pos)
    if i < 0:
        out.append(s[pos:])
        break
    out.append(s[pos:i])
    start = i + len(needle)
    depth = 1
    j = start
    in_str = None
    esc = False
    while j < len(s) and depth:
        c = s[j]
        if in_str:
            if esc:
                esc = False
            elif c == '\\':
                esc = True
            elif c == in_str:
                in_str = None
        else:
            if c in ('"', "'", '`'):
                in_str = c
            elif c == '(':
                depth += 1
            elif c == ')':
                depth -= 1
        j += 1
    if depth != 0:
        raise SystemExit('ERROR: unterminated NewCoordinator call')
    body = s[start:j-1]
    args=[]; last=0; d=0; ins=None; esc=False
    for k,c in enumerate(body):
        if ins:
            if esc: esc=False
            elif c=='\\': esc=True
            elif c==ins: ins=None
            continue
        if c in ('"', "'", '`'):
            ins=c
        elif c in '([{': d+=1
        elif c in ')]}': d-=1
        elif c==',' and d==0:
            args.append(body[last:k].strip()); last=k+1
    args.append(body[last:].strip())
    if len(args) == 8:
        args.insert(4, 'Options{}')
        out.append(needle + ', '.join(args) + ')')
        changes += 1
    else:
        out.append(s[i:j])
    pos = j
new = ''.join(out)
if changes:
    p.write_text(new)
    print(f'Corrected {changes} stale NewCoordinator call(s).')
else:
    # Idempotent success when the constructor is already corrected.
    if 'NewCoordinator' in s and 'Options{}' in s:
        print('Corrected constructor wiring already present.')
    else:
        raise SystemExit('ERROR: expected v0.16 NewCoordinator test call was not found')
PY

if [[ -x /usr/local/go/bin/gofmt ]]; then
  /usr/local/go/bin/gofmt -w "$TEST_FILE"
elif command -v gofmt >/dev/null 2>&1; then
  gofmt -w "$TEST_FILE"
fi

if ! git -C "$ROOT" diff --check; then
  echo "ERROR: whitespace/diff validation failed after v0.16 install." >&2
  exit 1
fi

echo "Balance Mod v0.16 corrected bundle applied."
echo "No provider/API key was read, written, or used."
echo "Quick: cd '$ROOT' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_quick.sh"
echo "Full:  cd '$ROOT' && PATH=/usr/local/go/bin:\$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh"
