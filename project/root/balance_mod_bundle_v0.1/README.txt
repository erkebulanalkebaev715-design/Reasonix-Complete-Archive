Reasonix Balance Mod v0.1 bundle
Base commit: 9e68643823943f05d13ab6a4578b7a629d490b07

Contains:
- reasonix-balance-mod-v0.1.patch
- apply_balance_mod_v0.1.sh

Apply inside Debian:
  bash apply_balance_mod_v0.1.sh ~/DeepSeek-Reasonix

Then test without any API key:
  cd ~/DeepSeek-Reasonix
  GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh
