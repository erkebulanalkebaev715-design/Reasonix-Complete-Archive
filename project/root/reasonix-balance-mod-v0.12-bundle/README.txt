Balance Mod v0.12
=================
Base required: the user's known-good Balance Mod v0.11 tree (V11 smoke PASS).

Adds:
- automatic continuation at Reasonix's real TurnDone/idle boundary
- bounded continuation lease (default 4, max 12)
- native durable session-inbox handoff for every continuation
- queue pause/prioritize/resume transaction so older queued user work cannot run on the temporary repair model tier
- direct HTTP/APK submit gate only during the short transition lease
- KZT fail-closed preparation of a native per-turn cost budget before model switching
- terminal/budget/no-progress routes never auto-enqueue another turn
- pending routes remain available for manual review if automation is blocked
- APK endpoints: GET /mod/orchestrator; POST /mod/orchestrator/config|stop|resume
- orchestrator state in /mod/status, /mod/power and /mod/app/bootstrap
- persisted orchestrator configuration in the existing workspace APK-state file
- quick smoke script plus full 38-stage regression smoke

Important limitations kept explicit:
- v0.12 automates serve/APK mode; other frontends are not forced through this lease.
- a process crash in the tiny interval after TurnDone but before the continuation is durably enqueued can still lose the in-memory pending route. Once enqueued, Reasonix's native inbox is durable. A later release should persist that pre-enqueue route journal if crash-proof coverage of that exact window is required.
- Pro 'diagnosisOnly' remains a routing semantic; v0.12 does not yet add a separate temporary read-only tool overlay for Pro. That should be enforced before real paid Auto mode is enabled.

No DeepSeek/API key is needed for installation or tests.
Expected quick marker: BALANCE_MOD_V12_QUICK_PASS
Expected full marker: BALANCE_MOD_V12_SMOKE_PASS
