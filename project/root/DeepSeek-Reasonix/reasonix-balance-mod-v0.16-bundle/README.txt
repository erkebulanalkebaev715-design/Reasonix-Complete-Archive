Balance Mod v0.16
Base: v0.15 PASS

Purpose: close the pre-call budget overshoot gap before any real provider test.
HardStop now uses a conservative pre-provider MaxTokens cap, remaining-budget
carryover across model/controller rebuilds, fail-closed missing FX/pricing, and
single-agent hard-budget mode for provider side paths not sharing one reservation
ledger yet.

No API key is required or touched.
