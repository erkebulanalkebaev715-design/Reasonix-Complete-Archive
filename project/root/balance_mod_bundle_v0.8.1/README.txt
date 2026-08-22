Balance Mod v0.8.1

This is a corrected/rebased v0.8 patch for the exact v0.7 tree that passed the user's 24-stage smoke test.
The previous v0.8 package had one stale context hunk in internal/serve/mod_capabilities.go: it was generated without the v0.7 reasoningPolicy line. Because the installer used `git apply --check`, the failed install changed no files.

Apply:
  bash balance_mod_bundle_v0.8.1/apply_balance_mod_v0.8.1.sh ~/DeepSeek-Reasonix

Then:
  cd ~/DeepSeek-Reasonix
  PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh

Expected final marker:
  BALANCE_MOD_V08_SMOKE_PASS
