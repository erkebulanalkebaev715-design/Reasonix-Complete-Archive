#!/data/data/com.termux/files/usr/bin/bash
set -u

LOGDIR="$HOME/reasonix-morning-supervisor"
mkdir -p "$LOGDIR"

RESTARTS=0
WINDOW_START="$(date +%s)"
FIRST=1

run_agent() {
  MODE="$1"

  proot-distro login debian -- /bin/bash -lc '
    export PATH=/usr/local/go/bin:/root/.opencode/bin:$PATH
    export GOTOOLCHAIN=local

    PROMPT_FILE=/root/REASONIX_3_MORNING_STABLE_WITH_API.txt
    [ -f "$PROMPT_FILE" ] || { echo "PROMPT_FILE_MISSING"; exit 21; }

    KEY="$(sed -n "s/^DEEPSEEK_API_KEY=//p" "$PROMPT_FILE" | head -n1)"
    [ -n "$KEY" ] || { echo "API_SET=no"; exit 22; }
    export DEEPSEEK_API_KEY="$KEY"
    unset KEY

    cd /root/DeepSeek-Reasonix || exit 20

    if [ -f /root/reasonix-night-permissions.json ]; then
      export OPENCODE_CONFIG_CONTENT="$(cat /root/reasonix-night-permissions.json)"
    fi

    echo "PROJECT=/root/DeepSeek-Reasonix"
    echo "MODEL=deepseek/deepseek-v4-flash"
    echo "API_SET=$([ -n "$DEEPSEEK_API_KEY" ] && echo yes || echo no)"

    if [ "'"$MODE"'" = "FIRST" ]; then
      exec opencode run \
        --auto \
        --model deepseek/deepseek-v4-flash \
        --title "Reasonix 3.0 Morning Stable" \
        "Read /root/REASONIX_3_MORNING_STABLE_WITH_API.txt completely first. Then execute that MORNING STABLE completion plan autonomously from Stage M0 through M19 against the existing /root/DeepSeek-Reasonix project. Do not start from scratch."
    else
      exec opencode run \
        --continue \
        --auto \
        --model deepseek/deepseek-v4-flash \
        "Resume the existing Reasonix 3.0 Morning Stable session after an infrastructure interruption. Re-read /root/REASONIX_3_MORNING_STABLE_WITH_API.txt as needed, continue from the exact unfinished stage through M19, and do not repeat already successful paid tests."
    fi
  '
}

while true; do
  NOW="$(date +%s)"

  if [ $((NOW - WINDOW_START)) -gt 1800 ]; then
    WINDOW_START="$NOW"
    RESTARTS=0
  fi

  if [ "$RESTARTS" -ge 4 ]; then
    echo "$(date -Is) STOP: too many infrastructure failures in 30 minutes" \
      | tee -a "$LOGDIR/supervisor.log"
    exit 70
  fi

  STAMP="$(date +%Y%m%d-%H%M%S)"
  LOG="$LOGDIR/opencode-$STAMP.log"

  echo "$(date -Is) START attempt=$((RESTARTS + 1))" \
    | tee -a "$LOGDIR/supervisor.log"

  if [ "$FIRST" -eq 1 ]; then
    run_agent FIRST 2>&1 | tee "$LOG"
  else
    run_agent RESUME 2>&1 | tee "$LOG"
  fi

  RC=${PIPESTATUS[0]}

  echo "$(date -Is) EXIT rc=$RC" \
    | tee -a "$LOGDIR/supervisor.log"

  if [ "$RC" -eq 0 ]; then
    echo "$(date -Is) DONE normally" \
      | tee -a "$LOGDIR/supervisor.log"
    exit 0
  fi

  FIRST=0
  RESTARTS=$((RESTARTS + 1))

  echo "$(date -Is) retry in 15 seconds" \
    | tee -a "$LOGDIR/supervisor.log"

  sleep 15
done
