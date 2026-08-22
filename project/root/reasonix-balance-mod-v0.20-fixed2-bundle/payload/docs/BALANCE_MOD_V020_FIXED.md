# Balance Mod v0.20 Fixed — online final gate

This fixed bundle does not change the v0.20 production controller/budget design.
It fixes the real-provider test harness and final wrapper after the offline preflight has passed.

Key changes:
- visible `[REAL x/10]` progress, including a 5-second heartbeat while waiting;
- no historical v0.18/v0.19 full regression is re-run by the final online smoke;
- exactly one real `deepseek-v4-flash` task is submitted;
- success requires both the response marker and positive KZT ledger spend, preventing prompt-marker false positives;
- isolated HOME/REASONIX_HOME for a clean real-provider gate;
- per-request curl timeouts, 120-second provider deadline by default;
- redacted backend log on failure;
- token value is never printed;
- Flash-only/Pro-deny, provider-usage reconciliation, and hard budget assertion remain mandatory.

Run after the previously-passed offline preflight:

```bash
PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_v020.sh
```

Required final lines:

```text
BALANCE_MOD_V20_REAL_GATE_PASS
BALANCE_MOD_V20_SMOKE_PASS
```


## Fixed2 credential-runtime correction
Reasonix current provider credential resolution reads provider API keys from the global `<REASONIX_HOME>/.env`; inherited shell environment variables are not provider-key runtime fallbacks. The real gate now stages the explicitly supplied `DEEPSEEK_API_KEY` into the isolated temporary `REASONIX_HOME/.env` with mode 0600 before starting `reasonix serve`. The whole temporary runtime is deleted on exit. Failure output now includes redacted task/live/budget diagnostics.
