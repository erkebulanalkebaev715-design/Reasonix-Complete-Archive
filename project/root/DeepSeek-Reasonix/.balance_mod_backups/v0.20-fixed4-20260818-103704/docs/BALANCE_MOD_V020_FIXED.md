# Balance Mod v0.20 Fixed3

This is a replacement real-provider harness for the already-preflighted v0.20 architecture.

Changes versus the earlier v0.20 online harnesses:

- online-only final smoke: does not rerun historical v0.18/v0.19 offline suites;
- visible `[REAL 1/12]` ... `[REAL 12/12]` progress;
- DeepSeek `/models` and `/user/balance` authenticated preflight before any generation;
- current Reasonix `doctor --json` must report `deepseek-v20` with `key_present=true`;
- isolated `HOME` and `REASONIX_HOME`, with provider secret staged in `<REASONIX_HOME>/.env`;
- exactly one submitted `deepseek-v4-flash` model task;
- prompt-echo-safe result marker (contiguous expected marker is not present in the prompt);
- immediate failure on typed live errors or provider/runtime errors seen in the backend log;
- 5-second heartbeat while waiting;
- success still requires both provider output marker and positive KZT ledger spend;
- Flash-only/Pro-forbidden verification;
- provider-reported token/cost reconciliation;
- final hard KZT budget assertion.

This does not claim universal exactly-once semantics for external side effects. It is the v0.20 real-provider integration gate.
