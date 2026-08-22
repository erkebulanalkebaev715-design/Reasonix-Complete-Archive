# Reasonix Balance Mod v0.20 Fixed2

Fixed bundle for an already-verified v0.20 offline preflight.

Install:

```bash
bash reasonix-balance-mod-v0.20-fixed-bundle/apply_balance_mod_v0.20_fixed.sh ~/DeepSeek-Reasonix
```

Then run the final ONLINE smoke only:

```bash
cd ~/DeepSeek-Reasonix
PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_v020.sh
```

The online smoke submits exactly one real `deepseek-v4-flash` task and prints visible `[REAL 1/10] ... [REAL 10/10]` progress.
It does not repeat v0.18/v0.19 offline suites.

Required final output:

```text
BALANCE_MOD_V20_REAL_GATE_PASS
BALANCE_MOD_V20_SMOKE_PASS
```


Fixed2 closes the real-provider credential handoff bug: the online gate now uses the isolated Reasonix global `.env` credential source expected by current Reasonix rather than relying on inherited shell env at provider-call time.
