# Balance Mod v0.20 Fixed5 — one-paid-attempt hard-budget policy

Fixed5 closes the real Fixed4 blocker:

`estimated input 0.011052 >= retry-reserved share 0.000328 USD`

The old v0.16 guard multiplied two retry layers (`maxSamplingAttempts * (provider.MaxRetries+1)` = 66). In strict hard-budget mode Fixed5 disables hidden automatic retries and admits only the current provider attempt against the current remaining hard allowance. Any later paid attempt must return to host/controller admission after ledger reconciliation.

Changes:
- provider connection/header retry limit forced to 0 under strict hard budget;
- body-stream replay disabled under strict hard budget;
- missing-reasoning fallback/replay disabled under strict hard budget;
- pre-call token/cost cap uses current remaining allowance, not a 66-way split;
- targeted tests prove one HTTP attempt and a fitting real-sized prompt;
- v0.20 online gate runs these tests before the one real DeepSeek task.
