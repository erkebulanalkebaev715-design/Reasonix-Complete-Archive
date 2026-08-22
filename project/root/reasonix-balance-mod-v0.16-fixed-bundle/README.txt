Balance Mod v0.16 — corrected bundle
Base: v0.15 PASS

Purpose:
Close the last hard-budget overshoot gap before the first real provider test.

The hard KZT budget is now enforced before Provider.Stream, not only after a
model round. The agent conservatively reserves the retry envelope, estimates
prompt cost locally, clamps MaxTokens to what remains affordable, and fails
closed when pricing/FX is unknown or the request cannot fit.

Hard-budget mode is deliberately single-agent for now: planner/title semantic
router/delegated provider side paths are disabled or made deterministic until a
shared atomic reservation ledger exists. Model rebuilds carry over the remaining
allowance rather than re-granting the original budget.

This corrected bundle also fixes the stale NewCoordinator constructor call in
the v0.16 test file before any smoke test is run.

No API key is required or touched.
Expected quick final line: BALANCE_MOD_V16_QUICK_PASS
Expected full final line:  BALANCE_MOD_V16_SMOKE_PASS
