Balance Mod v0.15 — Offline Stress Gate

Base required:
  v0.14 + hotfixes v0.14.1, v0.14.2 and v0.14.3,
  with BALANCE_MOD_V14_SMOKE_PASS already observed.

What v0.15 adds:
- MockProvider deny-bypass scenario that deliberately targets write_file through
  use_capability after write_file is hidden from the provider schema.
- Process-level offline stress harness using an isolated REASONIX_HOME.
- Crash/restart persistence test for budget + tool policy.
- Paused durable queue + per-task KZT ceiling test.
- Fail-closed rollback test when no checkpoint exists.
- Malformed APK-state recovery test.
- Invalid budget mutation test.
- Provider-key-shaped state leak check.

Important limitation:
This does NOT claim arbitrary in-flight model work is exactly-once after process
kill. Recovered/uncertain work remains subject to Reasonix's reviewed recovery
policy. The v0.13 auto-continuation path has its separate idempotency guarantee.

No real provider key is needed.
