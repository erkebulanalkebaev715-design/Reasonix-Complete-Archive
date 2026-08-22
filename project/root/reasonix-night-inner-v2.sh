#!/bin/bash
set -uo pipefail

export PATH=/usr/local/go/bin:/root/.opencode/bin:/root/reasonix-android-tools/bin:$PATH
export GOTOOLCHAIN=local
export OPENCODE_DISABLE_AUTOUPDATE=1
export OPENCODE_DISABLE_LSP_DOWNLOAD=1
hash -r

REPO=/root/DeepSeek-Reasonix
PROMPT=/root/REASONIX_3_NIGHT_QA_FIX_MASTER_PROMPT.txt
LOG=/root/reasonix-night.log
REPORT=/root/DeepSeek-Reasonix/docs/REASONIX_3_NIGHT_QA_FIX_FINAL_REPORT.md
APK=/sdcard/Download/Reasonix-Mobile-v3.0.1-NIGHT.apk
STOP_FLAG=/sdcard/Download/.reasonix-night-budget-stop
MODEL="${OPENCODE_MODEL:?OPENCODE_MODEL missing}"
RUN_START_EPOCH="${RUN_START_EPOCH:?RUN_START_EPOCH missing}"
EXPECTED_PACKAGE="${EXPECTED_PACKAGE:?EXPECTED_PACKAGE missing}"
EXPECTED_CERT_SHA256="${EXPECTED_CERT_SHA256:?EXPECTED_CERT_SHA256 missing}"
final_rc=1

copy_outputs() {
  cp -f "$LOG" /sdcard/Download/reasonix-night.log 2>/dev/null || true
  [ -s "$REPORT" ] && cp -f "$REPORT" /sdcard/Download/REASONIX_3_NIGHT_QA_FIX_FINAL_REPORT.md 2>/dev/null || true
}
trap copy_outputs EXIT
trap 'echo "[$(date -Is)] INNER INTERRUPTED" >> "$LOG" 2>/dev/null || true; exit 130' INT TERM

artifact_fresh() {
  local f="$1" m=""
  [ -s "$f" ] || return 1
  m="$(stat -c %Y "$f" 2>/dev/null || echo 0)"
  [ "$m" -ge "$RUN_START_EPOCH" ]
}

validate_artifacts() {
  artifact_fresh "$REPORT" || { echo "VALIDATION: fresh report missing" | tee -a "$LOG"; return 1; }
  artifact_fresh "$APK" || { echo "VALIDATION: fresh APK missing" | tee -a "$LOG"; return 1; }

  local pkg="" cert="" aapt_cmd=""
  if command -v aapt >/dev/null; then aapt_cmd=aapt; else aapt_cmd=aapt2; fi
  pkg="$($aapt_cmd dump badging "$APK" 2>/dev/null | sed -n "s/^package: name='\([^']*\)'.*/\1/p" | head -1)"
  [ "$pkg" = "$EXPECTED_PACKAGE" ] || { echo "VALIDATION: package mismatch: ${pkg:-<none>}" | tee -a "$LOG"; return 1; }

  apksigner verify "$APK" >/dev/null 2>&1 || { echo "VALIDATION: apksigner verify failed" | tee -a "$LOG"; return 1; }
  cert="$(apksigner verify --print-certs "$APK" 2>/dev/null | awk -F': ' '/certificate SHA-256 digest/ {print $2; exit}' | tr -d ':' | tr 'A-F' 'a-f')"
  [ "$cert" = "$EXPECTED_CERT_SHA256" ] || { echo "VALIDATION: signing cert mismatch" | tee -a "$LOG"; return 1; }

  grep -qE 'PASS|READY_FOR_DEVICE_TEST|BLOCKED' "$REPORT" || { echo "VALIDATION: report lacks status evidence" | tee -a "$LOG"; return 1; }
  echo "VALIDATION: mandatory APK/report checks passed" | tee -a "$LOG"
  return 0
}

run_agent() {
  local mode="$1" text="$2" rc=0
  echo "[$(date -Is)] OPENCODE $mode model=$MODEL" | tee -a "$LOG"
  if [ "$mode" = "INITIAL" ]; then
    nice -n 5 opencode run --auto -m "$MODEL" "$text" 2>&1 | tee -a "$LOG"
  else
    nice -n 5 opencode run --auto --continue -m "$MODEL" "$text" 2>&1 | tee -a "$LOG"
  fi
  rc=${PIPESTATUS[0]}
  echo "[$(date -Is)] OPENCODE $mode EXIT rc=$rc" | tee -a "$LOG"
  return "$rc"
}

cd "$REPO" || exit 10
: > "$LOG"
echo "[$(date -Is)] NIGHT V2 START" | tee -a "$LOG"
echo "GO=$(go version)" | tee -a "$LOG"
echo "OPENCODE=$(opencode --version 2>/dev/null | head -1 || true)" | tee -a "$LOG"
echo "MODEL=$MODEL" | tee -a "$LOG"
echo "PROMPT_SHA256=$(sha256sum "$PROMPT" | awk '{print $1}')" | tee -a "$LOG"
echo "BASELINE=$(git rev-parse HEAD 2>/dev/null || true)" | tee -a "$LOG"
echo "RUN_START_EPOCH=$RUN_START_EPOCH" | tee -a "$LOG"

initial_rc=0
run_agent INITIAL "$(cat "$PROMPT")" || initial_rc=$?

if validate_artifacts; then
  final_rc=0
else
  if [ -s "$STOP_FLAG" ]; then
    echo "[$(date -Is)] Budget watchdog stop flag present; no continuation." | tee -a "$LOG"
    final_rc=90
  else
    echo "[$(date -Is)] Mandatory artifacts are not yet valid. One continuation pass only." | tee -a "$LOG"
    cont='CONTINUE THE SAME EXISTING Reasonix night repair from the current repo and current OpenCode session. DO NOT restart the project, redo completed work, or repeat unchanged paid tests. Inspect what is already changed and the existing night log/report state. Finish only the remaining mandatory gates. A successful finish requires a fresh /sdcard/Download/Reasonix-Mobile-v3.0.1-NIGHT.apk with package com.reasonix.mobile.installfix and preserved signing lineage, plus /root/DeepSeek-Reasonix/docs/REASONIX_3_NIGHT_QA_FIX_FINAL_REPORT.md with evidence and PASS/READY_FOR_DEVICE_TEST/BLOCKED matrix. If blocked, write the truthful report and stop. No subagents, Flash only, local-first.'
    continuation_rc=0
    run_agent CONTINUATION "$cont" || continuation_rc=$?
    if validate_artifacts; then
      final_rc=0
    else
      echo "[$(date -Is)] Mandatory artifact validation still failed after the single continuation." | tee -a "$LOG"
      final_rc=42
      [ "$continuation_rc" -ne 0 ] && final_rc="$continuation_rc"
    fi
  fi
fi

echo "[$(date -Is)] NIGHT V2 FINISH rc=$final_rc initial_rc=$initial_rc" | tee -a "$LOG"
if [ -s "$APK" ]; then
  echo "APK_SHA256=$(sha256sum "$APK" | awk '{print $1}')" | tee -a "$LOG"
  ls -lh "$APK" | tee -a "$LOG"
else
  echo "APK_NOT_FOUND" | tee -a "$LOG"
fi
if [ -s "$REPORT" ]; then
  echo "REPORT_FOUND=$REPORT" | tee -a "$LOG"
else
  echo "REPORT_NOT_FOUND" | tee -a "$LOG"
fi

exit "$final_rc"
