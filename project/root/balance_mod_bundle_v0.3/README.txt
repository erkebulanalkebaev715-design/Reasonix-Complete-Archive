Reasonix Balance Mod v0.3 — Efficiency Decision Core

Requires: Balance Mod v0.2 already applied and passing.
No DeepSeek API key is required or used by the smoke test.

Added in v0.3:
1) Objective power router state machine:
   Flash -> alternative Flash -> short Pro diagnosis -> Flash execution.
   Pro escalation requires the same failure to survive distinct strategies.
   Budget Governor can block expensive escalation.
2) Verified Failure Cache:
   append-only JSONL journal, CRC32 per record, fsync, corrupt-record skipping.
   Only verified fixes are stored; cache returns hints and never auto-applies code.
3) Build Log Reducer:
   strips ANSI noise, extracts root-cause/error windows, caps lines/bytes, keeps a compact LLM-facing summary.
4) Patch Governor:
   parses git diff --numstat and rejects oversized repair patches by files/changed lines.
5) APK bridge extension:
   /mod/status now exposes router state and v0.3 feature versions.
   GET  /mod/router
   POST /mod/router/reset
   SSE initial state includes router telemetry.
6) Smoke test expanded to 10 stages.

IMPORTANT: v0.3 creates and tests the deterministic modules and APK-facing control state.
It does NOT yet auto-switch a live DeepSeek session between Flash/Pro, because API/provider
orchestration remains intentionally disconnected until offline layers pass. Live wiring is a later step.

Apply:
  bash balance_mod_bundle_v0.3/apply_balance_mod_v0.3.sh ~/DeepSeek-Reasonix

Test:
  cd ~/DeepSeek-Reasonix
  PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh

Expected final line:
  BALANCE_MOD_V03_SMOKE_PASS
