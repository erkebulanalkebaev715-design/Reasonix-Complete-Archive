# Reasonix Balance Mod v0.20 Fixed12

Fixed12 is a compatibility + reconciliation hotfix over the validated Fixed9 hard-budget core and Fixed10 semantic completion gate.

The user tree is based on an older Reasonix `emitTurnUsage` shape: it returns no value and emits the Usage event inline. A newer upstream shape returns a CostQuote through a local event variable. Fixed11 matched only the newer formatting and therefore stopped safely before changing the tree.

Fixed12 removes that formatting dependency. Its installer locates `Agent.emitTurnUsage` structurally, verifies that the function contains the native sink emission, and inserts the env-gated exact Usage receipt hook in the correct place for both legacy and newer forms. The hook is a no-op unless `BALANCE_V20_USAGE_RECEIPT_PATH` is explicitly set by the bounded real gate.

The installer performs no DeepSeek/provider call. It runs receipt unit tests, compile-only gates for touched packages, a full CLI build, `git diff --check`, and rollback on failure.

The real gate then reconciles the exact provider token usage against the KZT ledger and requires Flash-only, one-request, non-estimated usage.
