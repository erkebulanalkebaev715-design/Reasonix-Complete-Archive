# Balance Mod v0.20 — First Real DeepSeek API Gate

Baseline: verified v0.19. Two-phase by design. Install/targeted/quick/preflight never read/write/use a provider key and never make a provider call. The real gate is locked until exact explicit approval.

First real task: DeepSeek V4 Flash only, Pro disabled, max output 64, hard KZT budget default 10 and absolute maximum 25, explicit USD/KZT FX required, token-authenticated loopback serve, provider-reported usage reconciliation.

Cost reconciliation is a rate-card estimate from provider-reported usage, not an invoice or exact wallet debit.

PASS markers: BALANCE_MOD_V20_TARGETED_PASS, BALANCE_MOD_V20_QUICK_PASS, BALANCE_MOD_V20_PREFLIGHT_PASS, BALANCE_MOD_V20_REAL_GATE_PASS, and only then BALANCE_MOD_V20_SMOKE_PASS.
