Balance Mod v0.11
=================
Base required: the user's known-good Balance Mod v0.10 tree.

Adds:
- one PowerEngine path for verification -> repair -> route -> KZT admission -> native model execution
- native Reasonix turn-evidence collector (mutations, verification receipts, CompletionSummary, TurnDone)
- unverified mutations fail closed instead of becoming DONE
- pending route application at an explicit idle boundary (avoids Reasonix model-switch/runtime race)
- GET /mod/power
- POST /mod/power/reset
- POST /mod/power/apply-pending
- APK bootstrap/status + SSE power state

No DeepSeek/API key is needed for installation or the smoke test.
Expected final smoke marker: BALANCE_MOD_V11_SMOKE_PASS
