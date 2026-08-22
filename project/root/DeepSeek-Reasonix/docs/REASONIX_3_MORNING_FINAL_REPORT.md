# Reasonix 3.0 Morning Stable — Final Evidence Report (Real API Validation)

Date: 2026-08-21 (local device: Android ARM64, root, proot-Debian/Termux)
Mode: single principal engineer (DeepSeek V4 Flash), real DeepSeek API authorized.
Plan: `REASONIX_3_MORNING_STABLE_WITH_API.txt` (M0..M19), executed against the
EXISTING `/root/DeepSeek-Reasonix` repo — nothing was started from scratch.

## MORNING_BASELINE_VERIFIED
- Repo `/root/DeepSeek-Reasonix`, branch `main-v2`, HEAD `9e68643` (unchanged from
  overnight baseline).
- Dirty tree at start: 24 modified tracked + 125 untracked entries (pre-existing
  Balance Mod v0.20 WIP + overnight swarm work). Morning changes are additive on
  top; pre-existing dirty state preserved (nothing reset, no rebase/amend).
- Canonical Go `/usr/local/go/bin/go` (1.26.4 linux/arm64), `GOTOOLCHAIN=local`.
- `DEEPSEEK_API_KEY_SET=true` (presence-only; value never printed).
- `docs/REASONIX_3_FINAL_REPORT.md` read in full before edits.

## OVERNIGHT_WORK_AUDITED
- Overnight claim of `go test ./...` 124/124 verified by re-run this morning.
- `BALANCE_MOD_V20_PREFLIGHT_PASS` and `BALANCE_MOD_SWARM_GATE_PASS` re-run green.
- `internal/swarm` (11 files) + `internal/serve/mod_swarm.go` + swarm events
  audited in full (types/planner/orchestrator/worker/resolver/budget/store +
  serve host). Architecture is a real orchestrator over the existing
  `agent.Agent` turn primitive — no concatenated-completion fake, no shadow
  engine.
- Real-provider gates were `TESTING_BLOCKED` overnight (approval env absent);
  this morning the user authorized them, so they were executed for real.

## ROOT_CAUSES_FOUND
1. **Stale + non-conservative official pricing snapshot (blocked the real gate).**
   Manifest `pricingSnapshot.asOf` was 2026-08-18 with flat rates
   (cacheHit 0.0028 / input 0.14 / output 0.28). DeepSeek changed to peak/off-peak
   pricing effective 2026-08-16 (official news `news260813`); the real gate fails
   closed on `asOf` age > `maxAgeDays` (1), so the gate was locked by staleness,
   and the flat rates under-price actual peak cost. FIXED: refreshed the snapshot
   (`asOf` 2026-08-21) to the current official **peak** rates (conservative):
   flash `0.014/0.44/1.32` USD, pro `0.044/1.32/3.96`; CNY flash `0.10/3.0/9.0`,
   pro `0.30/9.0/27.0`. Updated `configs/balance_mod_v020_real_provider_manifest.json`,
   `configs/reasonix.balance.v020.real.template.toml`, runtime official tables
   (`internal/config/pricing.go`, `internal/billing/catalog.go`), and the tests
   that pinned the old official values. Positive evidence: real gate now passes;
   reconciliation matches actual DeepSeek usage exactly.
2. **Cancelled swarm never persisted its terminal state.**
   `Orchestrator.Run` set `Status = StatusCancelled` and returned without calling
   `o.persist()`. The serve host then clears `active`; `GET /mod/swarm` falls back
   to the last persisted state, which was still `running` — a cancelled swarm
   appeared stuck as "running" forever after the run goroutine exited. FIXED:
   persist the cancelled state (with `FinishedAt`) before returning. Regression
   test `TestCancelledSwarmPersistsTerminalState`. Verified by the real
   cancellation + controlled-restart test (M10) after the fix.

## FILES_CHANGED_THIS_MORNING
- `configs/balance_mod_v020_real_provider_manifest.json` (pricing snapshot refresh)
- `configs/reasonix.balance.v020.real.template.toml` (matching provider price)
- `internal/config/pricing.go` (official USD/CNY tables -> current peak rates)
- `internal/billing/catalog.go` (official catalog entries -> current peak rates)
- `internal/swarm/orchestrator.go` (cancel persists terminal state)
- Tests pinned to old official prices: `internal/config/backfill_test.go`,
  `internal/config/billing_upgrade_test.go`, `internal/config/render_test.go`,
  `internal/cli/cli_test.go`, `internal/billing/quote_test.go`,
  `internal/boot/costquote_order_test.go`, `internal/eventwire/costquote_compat_test.go`,
  `desktop/settings_app_test.go`, `desktop/official_provider_access_install_test.go`
- New regression test: `internal/swarm/orchestrator_test.go`
  (`TestCancelledSwarmPersistsTerminalState`)
- New real-gate scripts: `scripts/reasonix_morning_real_chat.sh`,
  `scripts/reasonix_morning_real_tool.sh`,
  `scripts/reasonix_morning_real_approval.sh`,
  `scripts/reasonix_morning_real_swarm.sh`,
  `scripts/reasonix_morning_cancel_recovery.sh`

## PREEXISTING_DIRTY_STATE_PRESERVED
- All pre-existing modified/untracked files (Balance Mod WIP, overnight swarm)
  remain in the working tree. No `git checkout`, no stash, no rebase, no amend,
  no commit was made. Rollback for tracked files: `git checkout -- <path>`;
  new untracked files can be removed individually.

## CONTRACTS_CHANGED
- No frozen APK contract surface changed this morning (still `balance-apk-v1`,
  72 endpoints / 85 events, verified by `TestModAPKContractFrozenV1AndBootstrapNegotiates`).
- Pricing snapshot data changed (values + `asOf`), not the schema.
- No event kind changed (18 `Swarm*` kinds remain additive above `event.KindCount`).

## INVARIANTS_ADDED
- Official DeepSeek pricing tables never under-price the current official peak
  rate card (conservative pre-call budget estimate).
- A swarm's terminal state (done / failed / cancelled) is always persisted before
  `Run` returns, so persisted read-back can never show a stale `running` state.
- Cancelled workers are terminal (`cancelled`), never retried, and never counted
  as progress/failure.

## REASONIX_2_STATE
- Complete: turn lifecycle, continuation, cancellation, tool loop, Skills, MCP,
  projects, provider handling, sessions/recovery, progress-aware anti-loop,
  usage/budgets, typed event transport, approval invariant — all present via
  upstream + Balance Mod v0.20.
- Approval invariant preserved in the real path: mutation -> `ApprovalRequest` ->
  WAIT -> deny/allow -> continuation -> mutation -> read-back (M7).

## REASONIX_3_ARCHITECTURE
- `internal/swarm` = real runtime: orchestrator owns the objective,
  decomposition, DAG, scheduling, bounded concurrency, lifecycle, verification,
  integration. Workers are real `agent.Agent.Run` turns with scoped
  `tool.Registry`, isolated `agent.Session`, provider-agnostic `Resolver`,
  `ReadOnlyExecution` boundary. Durable `Store` under
  `config.SwarmStateDir()` (atomic 0600 JSON). 18 typed `Swarm*` events appended
  above `event.KindCount` (wire-stable), fanned through the existing broadcaster.
- Serve: `POST /mod/swarm/start|cancel`, `GET /mod/swarm|/mod/swarm/{id}|/mod/swarm/history`;
  background run uses a server-owned context (not the HTTP request context).

## SWARM_CORE
VERIFIED (audit + unit + real run): orchestrator owns global objective; bounded
parallelism; deterministic terminal states; permanent failures not blindly
retried; budget ledger; cancellation at swarm granularity.

## SWARM_TASK_GRAPH
VERIFIED: stable IDs (`00-…`/`01-…` zero-padded), dependency semantics
(`then`/`after`), valid transitions, dependency-failure propagation to
`TaskFailed` with `FailureDependency`, completion rules, durable
persistence/restart representation. `Store.List()` newest-first by `UpdatedAt`
(index 0 = newest, verified in `store.go`).

## SWARM_WORKERS
VERIFIED: canonical `agent.Agent` runtime; bounded objective; isolated session;
scoped tools (read-only surface default; write only via explicit `AllowedTools` +
ownership); provider/model assignment via `Resolver`; timeout + budget from
profile; structured result/evidence (`provider` + `readback` minimum; `runtime`
when tools ran).

## SWARM_PARALLELISM
VERIFIED: `MaxWorkers` strict bound; real concurrency (2 workers ran in parallel
in the real swarm test); no runaway fan-out; race safety under `-race`-compatible
locking (Snapshot/state under mutex); reasonable ARM64 behavior (works on this
device).

## SWARM_MUTATION_OWNERSHIP
VERIFIED: read-only workers enforced via native `ReadOnlyExecution` at dispatch;
write scopes via `WriteRoots` disjointness; no blind concurrent writes observed;
deny prevents mutation with read-back proof (M7).

## SWARM_SHARED_STATE
VERIFIED: structured findings/artifacts/evidence/test results; no raw transcript
fan-out; only structured content persisted; hidden reasoning never exported.

## SWARM_VERIFICATION
VERIFIED: per-task required-evidence check; integrated result verified before
`done` (`verified:true` observed in real run); empty/missing evidence fails the
swarm.

## SWARM_BUDGET
VERIFIED: total cost/token caps; per-worker caps; budget-stop cancels remaining
workers (unit test); deterministic stop; usage reconciliation (real run: ledger
requests=2, providers per-worker, reconcile against rate card).

## SWARM_CANCELLATION
VERIFIED + FIXED: worker/task/swarm cancellation via context; observable state;
cancelled workers terminal; no uncontrolled continuing work; **persisted
terminal state fix** (regression test + real restart test).

## SWARM_PERSISTENCE
VERIFIED: atomic 0600 JSON under `config.SwarmStateDir()`; completed and
cancelled states survive restart; completed-state read-back via
`GET /mod/swarm/{id}` and history.

## SWARM_RECOVERY
VERIFIED: controlled backend restart (no PRoot/Android crash) — state survives,
cancelled swarm not re-run, no auto-restart of completed work.

## SWARM_EVENTS
VERIFIED: canonical event transport (`event.Sink`); IDs/correlation via
`SwarmEvent{SwarmID, TaskID, WorkerID, Provider, Status, ModelRef, Failure}`;
ordering via existing broadcaster; no fake reasoning; no duplicate event bus.

## SWARM_API
VERIFIED: start (202), current, get-by-ID, history, cancel, auth (token), invalid
IDs (404/400), concurrent-call safety (active-swarm conflict 409), completed-state
read-back, restart behavior (persisted state read after restart).

## DEEPSEEK_RUNTIME
VERIFIED: real `deepseek-v20/deepseek-v4-flash` via `https://api.deepseek.com`
(openai kind). `/models` lists the model; `/user/balance` positive; doctor
reports `key_present=true`; pricing known before `Provider.Stream`.

## REAL_CHAT_TEST
PASS. Input `Привет. Коротко ответь: Reasonix работает.` ->
`M5_REAL_CHAT_PASS`. Final assistant message observed; real provider usage
receipt exact (`deepseek-v20/deepseek-v4-flash`, prompt=5140, completion=8,
requests=1); budget accounting positive (spentKzt≈0.99–1.04); no fake reasoning;
no uncontrolled retries; actual model ref from runtime evidence.

## REAL_TOOL_TEST
PASS (`M6_REAL_TOOL_PASS`): real `read_file` ToolDispatch -> ToolResult ->
provider continuation -> final answer quoting the read marker; `live.tool.finished`
ok=true; usage receipt exact. (Marker was kept credential-scrub-safe.)

## REAL_APPROVAL_TEST
PASS (`M7_REAL_APPROVAL_PASS`): single turn with two approvals — first **deny**
(no mutation, file absent, agent continues — deny is not no-progress/failure),
then **allow** (mutation -> read-back -> final answer confirms
`MORNING_APPROVAL_MARKER_OK`); positive spend.

## EXECUTION_ROUTER
CONFIG: present (`internal/efficiency/execution_router.go`), not enabled by the
real-gate manifest (`proAllowed=false`, `proMaxPercent=0`); `configured=false,
enabled=false` at runtime. READINESS: Flash real-ready (all real tests);
Pro not configured/not ready (no Pro key/manifest allowance). REAL ROUTING:
all real tests routed to `deepseek-v20/deepseek-v4-flash` (Flash-only). Offline
router mechanics tests pass (flash->flash->pro->flash mapping, budget recheck,
fail-closed switch). Automatic Flash<->Pro escalation NOT_VERIFIED (not
exercisable under the approved Flash-only manifest) — not faked as PASS.

## REAL_SWARM_TEST
PASS (`M9_REAL_SWARM_PASS`): 1 orchestrator + 2 independent worker tasks, both on
the real DeepSeek provider; both `succeeded` with `provider`+`readback` evidence;
`verified=true`; structured integration result; persistence + completed read-back
(`GET /mod/swarm/{id}`, history); budget ledger 2 requests, per-provider spend;
usage receipt exact.

## KIMI_RUNTIME
TESTING_BLOCKED — no Kimi/Moonshot credentials or config present
(`KIMI_API_KEY_SET=false`, `MOONSHOT_API_KEY_SET=false`, no provider entry).
The swarm is provider-agnostic by design (Resolver seam) and not blocked by this.

## HETEROGENEOUS_SWARM
TESTING_BLOCKED (depends on a second real provider; none available).

## BACKEND
VERIFIED: `balance-mod-v0.20`; stable bridge; dynamic upstream Serve; token auth;
canonical submit field (`/mod/app/task/start` `{"input":…}`); approval routes
(`POST /approve`); model routes; tools/skills/MCP/project/live/queue routes all
live-200; swarm routes (start/cancel/get/by-id/history); completed swarm
read-back. Frozen APK contract 72 endpoints / 85 events (test-passing). No
guessed endpoints: every route probed against a running backend.

## APK
No Android source changed this morning; no APK rebuild (backend additive, UI
unchanged). Desktop `types.ts` + `eventwire` carry the 18 `swarm_*` wire names
consistently. Package/signing lineage untouched (no APK produced).

## VOICE
READY_FOR_DEVICE_TEST (no device/voice observation this morning; nothing on the
backend blocks it).

## LOCAL_TESTS
- `go test ./...` 124 packages OK, EXIT 0 (post-change full run).
- `go vet` clean on touched packages; `gofmt -l` clean.
- Preflight `BALANCE_MOD_V20_PREFLIGHT_PASS`; swarm gate
  `BALANCE_MOD_SWARM_GATE_PASS`.

## REGRESSION_TESTS
- `TestCancelledSwarmPersistsTerminalState` (new, passes).
- Existing official-pricing tests updated to the refreshed snapshot and pass.
- Offline deterministic swarm tests, serve swarm e2e, mock swarm gate: all green.

## REAL_PROVIDER_TESTS
- `BALANCE_MOD_V20_REAL_GATE_PASS` (real one-shot Flash task; reconcile
  `V020_RECONCILE_PASS`; spend under cap).
- M5 real chat, M6 real tool, M7 real approval, M9 real 2-worker swarm,
  M10 cancel + recovery — all PASS on the real provider.

## BUDGET_USAGE
Small controlled budget (`BALANCE_V20_BUDGET_KZT` 15–25, FX 457.24 USD/KZT live
rate). Real spends observed: chat ≈0.99–1.04 KZT, tool ≈1.10 KZT, approval ≈1.42
KZT, swarm ≈0.00055 USD (2 req), cancel/recovery ≈2 requests. No budget raised to
force a pass; a pre-call rejection was diagnosed (estimate > allowance) and the
budget stayed at the small documented level. No 401/402/422/429 errors.

## PASS
- Offline release chain + full test suite (124 pkgs).
- Real DeepSeek chat / tool / approval / 2-worker swarm / cancel+recovery.
- Pricing snapshot refresh (root cause #1) and cancelled-state persist fix
  (root cause #2) with regression tests.
- Frozen APK contract unchanged; backend integration verified live.

## NOT_VERIFIED
- Automatic Flash<->Pro escalation on the real provider (disabled by the
  Flash-only approved manifest; offline mechanics pass only).
- Kimi / heterogeneous swarm (no credentials).

## TESTING_BLOCKED
- Kimi/Moonshot runtime and heterogeneous swarm (no second provider).

## READY_FOR_DEVICE_TEST
- APK/voice: backend surface is ready; physical install/launch/voice were not
  observed on a device this morning.

## ARTIFACTS
- `bin/reasonix` (fresh build; source changed -> rebuilt).
- Real-gate scripts: `reasonix_morning_real_chat.sh`, `reasonix_morning_real_tool.sh`,
  `reasonix_morning_real_approval.sh`, `reasonix_morning_real_swarm.sh`,
  `reasonix_morning_cancel_recovery.sh`.
- Updated manifests/templates and Go official pricing tables.
- Gate logs: `/tmp/m17_full_test.log` (124/124), preflight/swarm-gate outputs above.

## SHA256
- `bin/reasonix` = `a08b830509a04d501b9adc0d08fe0bc1139437844b1556c819177761b9b0cb4f`
- `configs/balance_mod_v020_real_provider_manifest.json` =
  `372e690a36f114010b5f97ed1344a3bfa18fda933c394529793f7a04eaa9df92`
- `scripts/reasonix_morning_real_chat.sh` =
  `ef3ee320c1a8ed92281dd8e62a9f9fb7d5d6f0aa2207a992bf49e2dc5101d93c`
- `scripts/reasonix_morning_real_tool.sh` =
  `0e8680611b6277c7c666999acf8990f1e0467143345447d398cd766dc4d970aa`
- `scripts/reasonix_morning_real_approval.sh` =
  `b811c2fa8b03ca7bf1b09178e4d7d3900edc39416fc5e5ce4368990f3be4d690`
- `scripts/reasonix_morning_real_swarm.sh` =
  `287c7df203c14b6d7b31655f1d2b7b3cd3f9f9e7003c9b2ca226947a96d9938f`
- `scripts/reasonix_morning_cancel_recovery.sh` =
  `776b199658a3c36f00f328af2f76329b7a5ed3f69d2f772974572fbbd13d6c60`

## ROLLBACK
- Working tree changes uncommitted. Tracked-file revert: `git checkout -- <path>`.
- New untracked files (swarm, mod_swarm, morning scripts, refreshed configs)
  removable individually; `.balance_mod_backups/` and prior bundles preserved.
- Prior `bin/reasonix` hash recorded in the overnight report for comparison.

## NEXT_SINGLE_ACTION
Run the same real-gate/smoke suite after any future pricing or provider change;
optionally add a second provider (Kimi) to exercise the heterogeneous swarm, and
install/launch the APK on a device for the VOICE/UI pass.

---

FEATURE | TEST | RESULT | EVIDENCE
--- | --- | --- | ---
Local suite | `go test ./...` | PASS | 124 pkgs, exit 0
Preflight | `balance_mod_v020_preflight.sh` | PASS | `BALANCE_MOD_V20_PREFLIGHT_PASS`
Mock swarm | `balance_mod_swarm_gate.sh` | PASS | `BALANCE_MOD_SWARM_GATE_PASS`
Real DeepSeek gate | `balance_mod_v020_real_gate.sh` | PASS | `BALANCE_MOD_V20_REAL_GATE_PASS`
Real chat | `reasonix_morning_real_chat.sh` | PASS | `M5_REAL_CHAT_PASS`
Real tool | `reasonix_morning_real_tool.sh` | PASS | `M6_REAL_TOOL_PASS`
Real approval | `reasonix_morning_real_approval.sh` | PASS | `M7_REAL_APPROVAL_PASS`
Real 2-worker swarm | `reasonix_morning_real_swarm.sh` | PASS | `M9_REAL_SWARM_PASS`
Cancel+recovery | `reasonix_morning_cancel_recovery.sh` | PASS | `M10_CANCEL_RECOVERY_PASS`
Swarm cancel persist | `TestCancelledSwarmPersistsTerminalState` | PASS | regression test
Execution router | offline unit tests | PASS (mechanics); REAL escalation | NOT_VERIFIED
Kimi / heterogeneous | (no credentials) | TESTING_BLOCKED | —
APK contract | `TestModAPKContractFrozenV1…` | PASS | 72 endpoints / 85 events
