#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
need=(ecj d8 aapt zipalign apksigner zip)
missing=()
for x in "${need[@]}"; do command -v "$x" >/dev/null 2>&1 || missing+=("$x"); done
ANDROID_JAR="${PREFIX:-/data/data/com.termux/files/usr}/share/java/android.jar"
if ((${#missing[@]})) || [[ ! -f "$ANDROID_JAR" ]]; then
  echo "BUILD_TOOLS_MISSING: ${missing[*]} android.jar=$([[ -f "$ANDROID_JAR" ]] && echo ok || echo missing)"
  echo "Run in Termux: pkg install d8 aapt apksigner ecj zip -y"
  exit 20
fi
rm -rf build
mkdir -p build/classes build/dex
ecj -d build/classes \
  src/com/reasonix/mobile/installfix/MainActivity.java
mapfile -t classes < <(find build/classes -type f -name '*.class' -print)
((${#classes[@]})) || { echo NO_CLASSES; exit 21; }
d8 --lib "$ANDROID_JAR" --min-api 23 --output build/dex "${classes[@]}"
aapt package -f -M AndroidManifest.xml -S res -A assets -I "$ANDROID_JAR" -F build/base-unsigned.apk
cp build/base-unsigned.apk build/unsigned.apk
(cd build/dex && zip -q -u ../unsigned.apk classes.dex)
zipalign -f 4 build/unsigned.apk build/aligned.apk
apksigner sign \
  --ks reasonix-signing.p12 --ks-type PKCS12 --ks-key-alias reasonix \
  --ks-pass pass:reasonix-mobile-local --key-pass pass:reasonix-mobile-local \
  --out build/Reasonix-Mobile-v3.0.1.apk build/aligned.apk
apksigner verify --verbose --print-certs build/Reasonix-Mobile-v3.0.1.apk
sha256sum build/Reasonix-Mobile-v3.0.1.apk | tee build/SHA256.txt
cp build/Reasonix-Mobile-v3.0.1.apk /sdcard/Download/
cp build/SHA256.txt /sdcard/Download/Reasonix-Mobile-v3.0.1-SHA256.txt
printf '\nVOICE_BUILD_READY\nAPK=/sdcard/Download/Reasonix-Mobile-v3.0.1.apk\n'
