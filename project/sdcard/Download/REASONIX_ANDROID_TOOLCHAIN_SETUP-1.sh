#!/bin/bash
set -euo pipefail

# Reasonix Android Toolchain Setup — run INSIDE Debian PRoot as root.
ROOT_DIR=/root/reasonix-android-tools
BIN_DIR="$ROOT_DIR/bin"
mkdir -p "$BIN_DIR"
chmod 700 "$ROOT_DIR"

[ -f /etc/debian_version ] || { echo 'ERROR: run inside Debian PRoot'; exit 20; }
export PATH="/usr/local/go/bin:/root/.opencode/bin:$BIN_DIR:$PATH"
export GOTOOLCHAIN=local

if [ "${SKIP_APT:-0}" != 1 ]; then
  apt-get update
  apt-get install -y ca-certificates curl jq python3 git rsync unzip zip file default-jdk-headless openssl procps findutils coreutils || true
  for p in aapt2 aapt apksigner zipalign adb android-sdk-build-tools android-sdk-platform-tools android-sdk; do
    apt-cache show "$p" >/dev/null 2>&1 && apt-get install -y "$p" || true
  done
fi

find_tool() {
  n="$1"
  command -v "$n" 2>/dev/null && return 0
  find /usr/bin /usr/local/bin /usr/lib/android-sdk /opt/android-sdk /root/Android/Sdk /root/android-sdk /data/data/com.termux/files/usr/bin \
    -type f -name "$n" -perm -111 2>/dev/null | sort -V | tail -n1
}

AAPT2="$(find_tool aapt2 || true)"
AAPT="$(find_tool aapt || true)"
APKSIGNER="$(find_tool apksigner || true)"
ZIPALIGN="$(find_tool zipalign || true)"
ADB="$(find_tool adb || true)"
D8="$(find_tool d8 || true)"
JAVA="$(find_tool java || true)"
JAVAC="$(find_tool javac || true)"
KEYTOOL="$(find_tool keytool || true)"
R8_JAR=""
[ -n "$D8" ] || R8_JAR="$(find /usr/lib/android-sdk /opt/android-sdk /root/Android/Sdk /root/android-sdk -type f \( -name r8.jar -o -name d8.jar \) 2>/dev/null | sort -V | tail -n1)"

cat > "$ROOT_DIR/env.sh" <<EOF
export REASONIX_ANDROID_TOOLS="$ROOT_DIR"
export PATH="$BIN_DIR:\$PATH"
export GOTOOLCHAIN=local
export AAPT2_BIN="$AAPT2"
export AAPT_BIN="$AAPT"
export APKSIGNER_BIN="$APKSIGNER"
export ZIPALIGN_BIN="$ZIPALIGN"
export ADB_BIN="$ADB"
export D8_BIN="$D8"
export R8_JAR="$R8_JAR"
export JAVA_BIN="$JAVA"
export JAVAC_BIN="$JAVAC"
export KEYTOOL_BIN="$KEYTOOL"
EOF
chmod 600 "$ROOT_DIR/env.sh"

cat > "$BIN_DIR/reasonix-toolchain-check" <<'EOF'
#!/bin/bash
set -u
. /root/reasonix-android-tools/env.sh
show(){ [ -n "$2" ] && [ -e "$2" ] && printf '%-12s OK  %s\n' "$1" "$2" || printf '%-12s MISSING\n' "$1"; }
echo '=== REASONIX ANDROID TOOLCHAIN ==='
show JAVA "${JAVA_BIN:-}"; show JAVAC "${JAVAC_BIN:-}"; show KEYTOOL "${KEYTOOL_BIN:-}"
show AAPT2 "${AAPT2_BIN:-}"; show AAPT "${AAPT_BIN:-}"; show APKSIGNER "${APKSIGNER_BIN:-}"
show ZIPALIGN "${ZIPALIGN_BIN:-}"; show D8 "${D8_BIN:-}"; show R8_JAR "${R8_JAR:-}"; show ADB "${ADB_BIN:-}"
command -v go >/dev/null 2>&1 && { echo "GO           OK  $(command -v go)"; go version || true; } || echo 'GO           MISSING'
command -v opencode >/dev/null 2>&1 && echo "OPENCODE     OK  $(command -v opencode)" || echo 'OPENCODE     MISSING'
echo 'ADB is optional for this APK build pass.'
EOF
chmod 755 "$BIN_DIR/reasonix-toolchain-check"

cat > "$BIN_DIR/reasonix-apk-verify" <<'EOF'
#!/bin/bash
set -euo pipefail
APK="${1:-}"; EXPECTED_PACKAGE="${2:-}"; EXPECTED_CERT="${3:-}"
[ -f "$APK" ] || { echo 'usage: reasonix-apk-verify APK [package] [cert_sha256]'; exit 2; }
. /root/reasonix-android-tools/env.sh

echo "APK=$APK"
unzip -t "$APK" >/dev/null && echo ZIP_OK
sha256sum "$APK"
BADGING=''
if [ -n "${AAPT2_BIN:-}" ] && [ -x "$AAPT2_BIN" ]; then BADGING="$($AAPT2_BIN dump badging "$APK" 2>/dev/null || true)";
elif [ -n "${AAPT_BIN:-}" ] && [ -x "$AAPT_BIN" ]; then BADGING="$($AAPT_BIN dump badging "$APK" 2>/dev/null || true)"; fi
if [ -n "$BADGING" ]; then
  printf '%s\n' "$BADGING" | grep -E '^package:|^launchable-activity:' | head -5
  if [ -n "$EXPECTED_PACKAGE" ]; then
    ACTUAL="$(printf '%s\n' "$BADGING" | sed -n "s/^package: name='\([^']*\)'.*/\1/p" | head -1)"
    [ "$ACTUAL" = "$EXPECTED_PACKAGE" ] || { echo "PACKAGE_MISMATCH actual=$ACTUAL expected=$EXPECTED_PACKAGE"; exit 10; }
    echo "PACKAGE_OK=$ACTUAL"
  fi
else echo BADGING_NOT_VERIFIED; fi

if [ -n "${APKSIGNER_BIN:-}" ] && [ -x "$APKSIGNER_BIN" ]; then
  "$APKSIGNER_BIN" verify --verbose --print-certs "$APK"
  if [ -n "$EXPECTED_CERT" ]; then
    ACTUAL="$($APKSIGNER_BIN verify --print-certs "$APK" 2>/dev/null | sed -n 's/^Signer #1 certificate SHA-256 digest: //p' | head -1 | tr 'A-F' 'a-f' | tr -d ':[:space:]')"
    WANT="$(printf '%s' "$EXPECTED_CERT" | tr 'A-F' 'a-f' | tr -d ':[:space:]')"
    [ "$ACTUAL" = "$WANT" ] || { echo "CERT_MISMATCH actual=$ACTUAL expected=$WANT"; exit 11; }
    echo "CERT_OK=$ACTUAL"
  fi
else echo SIGNATURE_NOT_VERIFIED; fi

if [ -n "${ZIPALIGN_BIN:-}" ] && [ -x "$ZIPALIGN_BIN" ]; then "$ZIPALIGN_BIN" -c -v 4 "$APK" >/dev/null && echo ZIPALIGN_OK; else echo ZIPALIGN_NOT_VERIFIED; fi
for f in AndroidManifest.xml classes.dex resources.arsc; do unzip -l "$APK" | awk '{print $4}' | grep -Fxq "$f" && echo "$f: PRESENT" || echo "$f: MISSING"; done
echo APK_VERIFY_COMPLETE
EOF
chmod 755 "$BIN_DIR/reasonix-apk-verify"

cat > "$BIN_DIR/reasonix-apk-copy" <<'EOF'
#!/bin/bash
set -euo pipefail
APK="${1:-}"; NAME="${2:-Reasonix-Mobile-v3.0.0.apk}"; DEST="/sdcard/Download/$NAME"
[ -f "$APK" ] || { echo 'usage: reasonix-apk-copy APK [name.apk]'; exit 2; }
cp -f "$APK" "$DEST"; sync; echo "COPIED=$DEST"; sha256sum "$DEST"
EOF
chmod 755 "$BIN_DIR/reasonix-apk-copy"

cat > "$BIN_DIR/reasonix-backend-probe" <<'EOF'
#!/bin/bash
set -u
echo '=== REASONIX BACKEND PROBE ==='
command -v ss >/dev/null 2>&1 && ss -ltn 2>/dev/null | grep -E '127\.0\.0\.1:(37914|[0-9]+)' | head -30 || true
for u in http://127.0.0.1:37914/ http://127.0.0.1:37914/mobile/integration-audit; do
  c="$(curl -sS -o /tmp/rxprobe.$$ -w '%{http_code}' --max-time 3 "$u" 2>/dev/null || true)"; echo "$u -> ${c:-NO_RESPONSE}"
done
rm -f /tmp/rxprobe.$$ 2>/dev/null || true
EOF
chmod 755 "$BIN_DIR/reasonix-backend-probe"

cat > "$ROOT_DIR/SKILL.md" <<'EOF'
# Reasonix Android Build Skill

Use for all future Reasonix Mobile APK work.

- Continue the existing Reasonix project; never create a random replacement app.
- Locate and reuse the real mobile lineage that produced the latest known-good APK.
- APK is UI/control-plane only; Reasonix remains the runtime.
- Preserve package/update/signing lineage unless evidence proves a migration is necessary.
- Preserve ordinary Android Activity + WebView; never regress to NativeActivity or handwritten DEX hacks.
- Stable localhost bridge/backend contracts must be read from current source, not guessed.
- Thinking != Activity. Approvals/tools/Skills/MCP/projects/Swarm must map to real backend primitives.
- No dead/decorative controls and no fake success.
- Build PASS != install PASS != launch PASS != feature PASS.
- Physical behavior remains READY_FOR_DEVICE_TEST until manually observed.
- For every defect: SYMPTOM -> REPRODUCE -> ROOT CAUSE -> INVARIANT -> FIX -> REGRESSION TEST -> RELEASE GATE.

Persistent commands:
- reasonix-toolchain-check
- reasonix-apk-verify APK [package] [cert_sha256]
- reasonix-apk-copy APK [name.apk]
- reasonix-backend-probe

Visual contract: minimal black/dark-gray/gray/white UI, tiny green status only; no neon or decorative gradients; composer `+ | message | mic | send`; sidebar New Chat/Search/Projects/history; Swarm collapsed by default and expandable.
EOF
chmod 600 "$ROOT_DIR/SKILL.md"



# Native Termux supervisor for the APK 3.0 OpenCode pass.
TERMUX_HOME=/data/data/com.termux/files/home
if [ -d "$TERMUX_HOME" ]; then
cat > "$TERMUX_HOME/reasonix-apk3-supervisor.sh" <<'EOF'
#!/data/data/com.termux/files/usr/bin/bash
set -u
LOGDIR="$HOME/reasonix-apk3-supervisor"
mkdir -p "$LOGDIR"
RESTARTS=0
WINDOW_START="$(date +%s)"
FIRST=1

run_agent() {
  MODE="$1"
  proot-distro login debian -- /bin/bash -lc '
    source /root/reasonix-android-tools/env.sh
    export PATH=/usr/local/go/bin:/root/.opencode/bin:/root/reasonix-android-tools/bin:$PATH
    export GOTOOLCHAIN=local
    PROMPT=/root/REASONIX_3_APK_BUILD_MASTER_PROMPT.txt
    [ -f "$PROMPT" ] || { echo PROMPT_FILE_MISSING; exit 21; }
    KEY="$(sed -n "s/^DEEPSEEK_API_KEY=//p" /root/REASONIX_3_MORNING_STABLE_WITH_API.txt 2>/dev/null | head -n1)"
    [ -n "$KEY" ] || { echo API_KEY_SOURCE_MISSING; exit 22; }
    export DEEPSEEK_API_KEY="$KEY"
    unset KEY
    cd /root/DeepSeek-Reasonix || exit 20
    if [ -f /root/reasonix-night-permissions.json ]; then
      export OPENCODE_CONFIG_CONTENT="$(cat /root/reasonix-night-permissions.json)"
    fi
    echo "PROJECT=/root/DeepSeek-Reasonix"
    echo "MODEL=deepseek/deepseek-v4-flash"
    echo "TOOLS=/root/reasonix-android-tools"
    echo "API_SET=$([ -n "$DEEPSEEK_API_KEY" ] && echo yes || echo no)"
    if [ "'"$MODE"'" = FIRST ]; then
      exec opencode run --auto --model deepseek/deepseek-v4-flash --title "Reasonix Mobile 3.0 APK Build" \
        "Read /root/REASONIX_3_APK_BUILD_MASTER_PROMPT.txt completely first, then execute it autonomously against the existing Reasonix project. Do not start a replacement app."
    else
      exec opencode run --continue --auto --model deepseek/deepseek-v4-flash \
        "Resume the existing Reasonix Mobile 3.0 APK build session after an infrastructure interruption. Re-read /root/REASONIX_3_APK_BUILD_MASTER_PROMPT.txt as needed and continue from the exact unfinished stage. Do not repeat completed work."
    fi
  '
}

while true; do
  NOW="$(date +%s)"
  if [ $((NOW-WINDOW_START)) -gt 1800 ]; then WINDOW_START="$NOW"; RESTARTS=0; fi
  if [ "$RESTARTS" -ge 4 ]; then echo "$(date -Is) STOP too many infrastructure failures" | tee -a "$LOGDIR/supervisor.log"; exit 70; fi
  STAMP="$(date +%Y%m%d-%H%M%S)"
  LOG="$LOGDIR/opencode-$STAMP.log"
  echo "$(date -Is) START attempt=$((RESTARTS+1))" | tee -a "$LOGDIR/supervisor.log"
  if [ "$FIRST" -eq 1 ]; then run_agent FIRST 2>&1 | tee "$LOG"; else run_agent RESUME 2>&1 | tee "$LOG"; fi
  RC=${PIPESTATUS[0]}
  echo "$(date -Is) EXIT rc=$RC" | tee -a "$LOGDIR/supervisor.log"
  if [ "$RC" -eq 0 ]; then echo "$(date -Is) DONE normally" | tee -a "$LOGDIR/supervisor.log"; exit 0; fi
  FIRST=0
  RESTARTS=$((RESTARTS+1))
  echo "$(date -Is) retry in 15s" | tee -a "$LOGDIR/supervisor.log"
  sleep 15
done
EOF
chmod 700 "$TERMUX_HOME/reasonix-apk3-supervisor.sh"
fi

{ echo "DATE=$(date -Is)"; "$BIN_DIR/reasonix-toolchain-check"; } | tee "$ROOT_DIR/TOOLCHAIN_REPORT.txt"
echo 'SETUP_DONE'
echo "TOOLS=$ROOT_DIR"
echo "SKILL=$ROOT_DIR/SKILL.md"
echo "NATIVE_SUPERVISOR=/data/data/com.termux/files/home/reasonix-apk3-supervisor.sh"
