# Balance Mod v0.18 — Full Offline Prototype / Release Candidate

Baseline: verified v0.17 Crash / Replay Hardening.

v0.18 intentionally adds no new routing, queue, verifier, recovery, or budget architecture.
Those mechanisms already exist. The purpose of this version is to freeze a canonical
configuration and prove the complete offline product boundary before APK UI work begins.

## RC boundary

The v0.18 targeted harness starts the real `reasonix serve` binary on localhost using only
the native mock provider. It removes common provider API-key environment variables,
applies the hard KZT budget through the existing HTTP bridge, starts a task through the
same `/mod/app/task/start` route intended for Android, waits for the real Reasonix tool
loop to expose `OFFLINE_MOCK_PASS` in live history, and verifies the budget remains below
its hard limit.

The wider RC gate then reruns:

- v0.14 localhost offline prototype;
- v0.15 offline stress harness;
- v0.16 hard pre-call process budget harness;
- v0.17 crash/replay targeted tests.

The full v0.18 smoke runs the complete v0.17 regression before the RC gate.

## What PASS means

`BALANCE_MOD_V18_SMOKE_PASS` means the existing Balance/Reasonix stack has passed the
configured offline release-candidate gate on that machine. It does not prove any real
provider behavior, price table, network failure mode, or Android APK UI behavior.

No real provider/API key is used by v0.18.
