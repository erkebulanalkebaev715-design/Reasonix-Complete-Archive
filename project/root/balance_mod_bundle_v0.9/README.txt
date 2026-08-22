Balance Mod v0.9 — Project Registry + Native Task Catalog

Expected baseline:
- Balance Mod v0.8.x
- v0.8 smoke already passed
- v0.8.2 persistence compile hotfix present

Adds:
- global APK project registry under Reasonix home
- deterministic project identity from canonical workspace
- safe register/remove/open-supervisor-handoff endpoints
- missing project folders are shown unavailable rather than corrupting registry
- project removal never deletes project files
- per-project budget remains the existing workspace-scoped v0.8 ledger
- APK task list built from native agent.ListSessions with no provider title call
- native task rename via agent.RenameSession with session-dir confinement
- bootstrap endpoint map for projects/tasks/native lifecycle
- v0.9 smoke stages

No API key is needed or used by the v0.9 tests.
