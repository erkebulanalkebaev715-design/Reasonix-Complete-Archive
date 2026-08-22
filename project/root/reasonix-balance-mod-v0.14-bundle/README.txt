Reasonix Balance Mod v0.14 — APK v1 Contract Freeze + Offline Prototype Gate

Required baseline:
  Balance Mod v0.13 with BALANCE_MOD_V13_SMOKE_PASS.

What changes:
1) GET /mod/app/contract exposes the frozen balance-apk-v1 machine contract:
   endpoint names/methods/paths, typed event names, compatibility rules and
   backend safety guarantees.
2) /mod/app/bootstrap now exposes the contract revision + digest. The Android
   APK can verify backend compatibility before enabling mutating controls.
3) v1 compatibility rule is explicit: additive response fields/endpoints/events
   are allowed; removing/renaming a frozen endpoint or changing its method/
   meaning requires a protocol-major bump.
4) Hidden model reasoning remains outside the APK protocol. The UI receives
   visible chat, plans/phases, actions, diffs, results and verification events.
5) scripts/balance_mod_offline_prototype.sh starts the real reasonix serve
   process on localhost with the zero-cost MockProvider, negotiates the APK
   contract, applies a KZT budget/profile, starts a task through HTTP, waits for
   OFFLINE_MOCK_PASS through live history, verifies zero KZT spend, and switches
   the same backend to chat mode.
6) Common real-provider key environment variables are explicitly removed from
   the offline prototype child process.

Important:
  The patch/apply/rollback/shell syntax was checked in the build environment.
  Full Go 1.26 ARM64 compilation is still authoritative on your Debian phone.

Apply:
  bash reasonix-balance-mod-v0.14-bundle/apply_balance_mod_v0.14.sh ~/DeepSeek-Reasonix

Quick:
  cd ~/DeepSeek-Reasonix
  PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_quick.sh
Expected:
  BALANCE_MOD_V14_QUICK_PASS

Full regression + process-level offline prototype:
  PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh
Expected:
  BALANCE_MOD_V14_SMOKE_PASS
