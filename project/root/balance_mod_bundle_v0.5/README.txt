Reasonix Balance Mod v0.5 — Provider-Agnostic Execution Router

Requires:
- Balance Mod v0.4 + v0.4.1 hotfix already applied
- previous full smoke ended with BALANCE_MOD_V04_SMOKE_PASS
- no DeepSeek API key is used or needed

v0.5 adds:
1) ExecutionRouter: turns the already-tested route decision into concrete model refs.
2) Four configurable slots:
   - Flash primary
   - Flash alternative
   - Pro diagnosis
   - Flash repair
3) Uses Reasonix's native serve.switchModel/controller rebuild path; no duplicate session/provider engine.
4) Immediate KZT CanSpend() re-check before model switching.
5) Pro phase is marked diagnosisOnly=true for the later permissions/tool-binding layer.
6) Routing is DISABLED by default. It must be explicitly configured; applying this patch cannot activate a paid provider by itself.
7) APK API:
   GET  /mod/execution
   POST /mod/execution/config
   POST /mod/execution/reset
   /mod/status and /mod/events include execution state.
8) Model refs are provider-agnostic and validated against the current Reasonix model/catalog/config.
9) Typed APK events include execution.updated, execution.blocked, model.switch.completed, model.escalated, model.returned_flash.
10) Smoke test expanded to 16 stages, including a native model-switch integration test with fake/offline controller refs.

Apply inside Debian:
  cd ~
  cp /sdcard/Download/reasonix-balance-mod-v0.5-bundle.tar.gz .
  tar -xzf reasonix-balance-mod-v0.5-bundle.tar.gz
  bash balance_mod_bundle_v0.5/apply_balance_mod_v0.5.sh ~/DeepSeek-Reasonix

Test:
  cd ~/DeepSeek-Reasonix
  PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh

Expected final line:
  BALANCE_MOD_V05_SMOKE_PASS

Important:
- No real DeepSeek API key is touched.
- No paid provider is automatically enabled.
- v0.5 wires native switching, but full automatic controller-side generation of RepairAttempt from live build/test/tool events is still a later stage.
- v0.6 should bind universal workspace/tool/instruction/permission controls for the future APK.
