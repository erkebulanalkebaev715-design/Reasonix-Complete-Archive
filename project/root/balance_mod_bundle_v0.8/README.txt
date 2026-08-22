Balance Mod v0.8 — APK Control Plane + Restart-safe Policy + Android Supervisor

BASELINE:
  Balance Mod v0.7 with BALANCE_MOD_V07_SMOKE_PASS.

ADDED:
  - GET /mod/app/bootstrap
  - POST /mod/app/apply (validated batch settings)
  - POST /mod/app/task/start -> native Reasonix submit
  - POST /mod/app/task/stop -> native Reasonix cancel
  - workspace-scoped atomic persistence of APK policy
  - KZT + Pro already-spent ledger survives backend restarts
  - project/tool/approval/execution policy survives backend restarts
  - Debian-side scripts/reasonix_android_backend.sh supervisor
  - future APK can start/stop/restart same Reasonix backend through Termux/PRoot

IMPORTANT:
  No DeepSeek/API key is needed or used by v0.8 development/smoke tests.
  Skills and AGENTS.md/REASONIX.md remain persisted by native Reasonix, not duplicated.
  Hidden model chain-of-thought is not exported to APK.

INSTALL IN DEBIAN:
  cd ~
  cp /sdcard/Download/reasonix-balance-mod-v0.8-bundle.tar.gz .
  tar -xzf reasonix-balance-mod-v0.8-bundle.tar.gz
  bash balance_mod_bundle_v0.8/apply_balance_mod_v0.8.sh ~/DeepSeek-Reasonix

TEST:
  cd ~/DeepSeek-Reasonix
  PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh

EXPECTED FINAL LINE:
  BALANCE_MOD_V08_SMOKE_PASS
