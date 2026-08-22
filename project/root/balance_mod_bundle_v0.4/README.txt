Reasonix Balance Mod v0.4 — Unified Offline Repair/Recovery Cycle

Requires:
- Balance Mod v0.3 already applied
- v0.3 smoke ended with BALANCE_MOD_V03_SMOKE_PASS
- No DeepSeek API key is used or needed

v0.4 adds:
1) RepairCycle: host verification -> patch guard -> failure cache -> objective router -> finalization.
2) Completion cannot pass without required external checks.
3) Failure fixes enter persistent cache only after verification PASS.
4) Regression/oversized patch requests rollback through an adapter; no second backup engine.
5) reasonix serve adapter delegates rollback to native Reasonix checkpoint RewindCode.
6) APK API:
   GET  /mod/cycle
   POST /mod/cycle/reset
   GET  /mod/recovery
   POST /mod/recovery/rollback-last
   /mod/status + /mod/events include cycle/recovery state.
7) MockProvider repair-cycle scenario drives Flash A -> Flash B -> Pro diagnosis -> Flash PASS fully offline.
8) Smoke test expanded to 13 stages.

Apply:
  bash balance_mod_bundle_v0.4/apply_balance_mod_v0.4.sh ~/DeepSeek-Reasonix

Test:
  cd ~/DeepSeek-Reasonix
  PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh

Expected final line:
  BALANCE_MOD_V04_SMOKE_PASS

Important:
- Live DeepSeek Flash/Pro switching is still intentionally disconnected.
- API key remains untouched until the offline prototype passes.
