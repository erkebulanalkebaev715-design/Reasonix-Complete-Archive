Balance Mod v0.10 — Native Durable Task Queue + Reviewed Recovery + Per-Turn Budgets

Expected baseline:
- Balance Mod v0.9
- BALANCE_MOD_V09_SMOKE_PASS already passed

What v0.10 adds:
- APK facade over Reasonix's existing durable session inbox (no second scheduler/database)
- queue list/add/update/delete/reorder/pause/resume/retry API
- typed queue.updated events for the future APK
- crash-recovery view with explicit human-reviewed retry of uncertain/blocked items
- no blind automatic retry after ambiguous crash state
- optional per-queued-turn limits: KZT, tokens, wall time
- KZT task limit clamps to remaining workspace hard budget
- KZT admission fails closed if provider currency/FX cannot be enforced
- task budget is injected only when that durable queued turn actually runs
- v0.9 session catalog remains task history; v0.10 queue is pending/running/recovery work

APK endpoints added:
GET    /mod/queue
POST   /mod/queue/items
PATCH  /mod/queue/items/{id}
DELETE /mod/queue/items/{id}
POST   /mod/queue/move
POST   /mod/queue/pause
POST   /mod/queue/resume
POST   /mod/queue/items/{id}/retry
GET    /mod/queue/recovery
POST   /mod/queue/recovery/retry

Offline gate:
PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh

Expected final marker:
BALANCE_MOD_V10_SMOKE_PASS

No API key is needed or used by the v0.10 smoke tests.
