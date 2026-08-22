#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-$HOME/DeepSeek-Reasonix}"
FILE="$TARGET/scripts/balance_mod_smoke.sh"

if [[ ! -f "$FILE" ]]; then
  echo "ERROR: $FILE not found" >&2
  exit 1
fi

MARKER='# Balance Mod v0.14.2: return from temp workspace before repo-relative tests.'
ANCHOR='echo "[38/44] v0.14 frozen APK v1 contract + bootstrap negotiation"'

if grep -Fq "$MARKER" "$FILE"; then
  echo "Balance Mod v0.14.2 hotfix already applied."
  exit 0
fi

if ! grep -Fq "$ANCHOR" "$FILE"; then
  echo "ERROR: expected v0.14 smoke-test anchor not found; no files were changed." >&2
  exit 1
fi

TMP="$(mktemp)"
awk -v marker="$MARKER" -v anchor="$ANCHOR" '
  $0 == anchor {
    print marker
    print "cd \"$ROOT\""
  }
  { print }
' "$FILE" > "$TMP"
cat "$TMP" > "$FILE"
rm -f "$TMP"
chmod +x "$FILE"

bash -n "$FILE"

echo "Balance Mod v0.14.2 hotfix applied."
echo "Fix: smoke test now returns to the Reasonix repo after temporary mock workspace before step 38."
echo "No runtime/agent/API logic was changed."
