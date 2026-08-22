Reasonix Balance Mod v0.13 — Durable Safety Gate

Baseline required:
  Balance Mod v0.12 with BALANCE_MOD_V12_SMOKE_PASS.

What changes:
1) Pending repair routes are persisted before automatic switching.
   Persisted records contain only fingerprints/counters/sanitized numstat;
   raw build logs, fix hints, prompts, source and hidden reasoning are excluded.
2) Automatic continuation uses a stable native session-inbox idempotency key.
   A crash/restart cannot blindly create another identical continuation turn;
   completed idempotent receipts are recognized as already consumed.
3) Restored pending routes require explicit APK/orchestrator resume after restart.
   Backend startup does not silently spend API budget.
4) Pro diagnosis is a hard native read-only boundary:
   mutating tools are denied and removed from provider schema. use_capability
   remains safe because its resolved target still passes the native deny gate.
5) If model switching fails, the temporary execution-policy overlay is rolled back.

No API key is needed for either smoke test.

Apply:
  bash reasonix-balance-mod-v0.13-bundle/apply_balance_mod_v0.13.sh ~/DeepSeek-Reasonix

Quick test:
  cd ~/DeepSeek-Reasonix
  PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_quick.sh

Expected:
  BALANCE_MOD_V13_QUICK_PASS

Full regression:
  PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh

Expected:
  BALANCE_MOD_V13_SMOKE_PASS
