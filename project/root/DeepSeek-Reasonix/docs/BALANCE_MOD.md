# Balance Mod

Goal: maximize verified coding result per unit of model spend, token budget,
wall time, and Android device resources. The mod prefers deterministic local
work and DeepSeek Flash, and escalates only when host-observed evidence proves
the cheaper path is not progressing.

## Non-negotiable development rule

No real provider key is used during mod development. The first real DeepSeek
call happens only after the offline prototype and its mock scenarios pass.

## Reused Reasonix foundations

Balance Mod extends existing Reasonix seams instead of duplicating them:

- `control.Controller`: orchestration behind CLI, HTTP/SSE and desktop.
- native loop/progress guards and final-readiness completion gate.
- `checkpoint`: snapshot/rewind transaction engine.
- Context Engine: cache-stable prefix, compaction and bounded tool output.
- `billing`: fixed-point money and host-generated `CostQuote`.
- task budgets and project analysis/CodeGraph.

## Implemented layers

### v0.1 / v0.1.1 — APK bridge, KZT budget, local resource telemetry

`/mod/*` is a machine-readable surface for the future Android APK. The APK
consumes JSON/SSE and never parses terminal text.

Budget policy supports total KZT budget, final-verification reserve, Pro share,
hard stop and explicit FX rates. No hard-coded exchange rate is trusted.

### v0.2 — host quality telemetry

The mod mirrors native Reasonix anti-loop/progress/final-readiness evidence into
typed APK events. A model cannot make itself `DONE` by saying it is done.

### v0.3 — efficiency decision modules

- objective router: `Flash A -> Flash B -> Pro diagnosis -> Flash repair`
- verified-only failure cache with append-only CRC journal
- build-log reducer; complete raw log remains local
- patch governor based on `git diff --numstat`

The router is deterministic. It does not let model prose request Pro.

### v0.4 — unified repair/recovery cycle

`RepairCycle` now joins the v0.3 pieces:

1. receive host verification receipt
2. reject model-only completion (zero required checks never passes)
3. check patch scope
4. reduce build log for the next model call
5. consult proven-failure cache as a hint only
6. route cheap/expensive work through the objective ladder
7. rollback measurable regression or oversized repair through a host adapter
8. persist a failure fix only after external checks pass
9. finalize only after the verification receipt passes

Rollback does not implement a second backup system. The `reasonix serve` adapter
calls native `control.Rewind(..., RewindCode)`, so Reasonix's existing conflict,
coverage and transaction protections remain authoritative.


### v0.5 — provider-agnostic execution router

The previously abstract route decision can now be applied to concrete Reasonix
model refs through the native `serve.switchModel` controller rebuild path. The
mod does not create a second provider/session engine.

Execution slots are configurable and provider-agnostic:

- primary cheap executor
- alternative cheap executor (may be the same model with a different strategy)
- expensive diagnosis model
- cheap post-diagnosis repair executor

The host re-checks the KZT budget immediately before a switch. Pro is exposed as
`diagnosisOnly=true` in typed state; hard tool restriction for that phase is a
later permissions-layer binding, not falsely claimed here.

The execution router is disabled by default. It must be explicitly configured
by the APK/host with refs that already exist in Reasonix's provider catalog.
This prevents development builds from accidentally activating a paid provider.

A successful sequence is:

`Flash A -> Flash B -> Pro diagnosis -> Flash repair -> verified finalize`

Model switches emit content-free events such as `model.switch.completed`,
`model.escalated`, `model.returned_flash`, and `execution.blocked`.

## APK endpoints through v0.5

Existing:

- `GET /mod/status`
- `GET /mod/budget`
- `POST /mod/budget`
- `POST /mod/budget/reset`
- `GET /mod/resources`
- `GET /mod/quality`
- `GET /mod/router`
- `POST /mod/router/reset`
- `GET /mod/events`

Added in v0.4:

- `GET /mod/cycle`
- `POST /mod/cycle/reset`
- `GET /mod/recovery`
- `POST /mod/recovery/rollback-last`

Added in v0.5:

- `GET /mod/execution`
- `POST /mod/execution/config`
- `POST /mod/execution/reset`

The recovery endpoint is fail-closed: active turn, no checkpoint, unsafe
coverage/conflict, or any native rewind refusal means no rollback is claimed.

APK-visible cycle state contains counters, route decisions, fingerprints and
verification counts only. Raw build logs, source code and cached fix hints are
not copied onto the Balance Mod SSE stream.

## Offline mock provider

`kind = "mock"` performs no provider network call and needs no API key.
Scenarios:

- `smoke`: real tool loop (`read_file -> use_capability/grep -> PASS`)
- `repeat-failure`: repeats a missing read until Reasonix's native loop guard
  redirects it
- `repair-cycle`: deterministic four-step sequence used to drive the v0.4
  `Flash -> Flash -> Pro diagnosis -> Flash PASS` policy test

Each release advances the offline gate; the current v0.7 smoke script must end
with `BALANCE_MOD_V07_SMOKE_PASS` before any real provider test is accepted.

## Still intentionally disconnected

v0.5 still spends **zero real DeepSeek tokens during development**. Native model
switching is now wired, but execution routing starts disabled and the smoke tests
use only offline/fake controller refs. A real DeepSeek key remains out of scope
until the complete offline prototype passes.

The next phase should bind universal workspace/tool/instruction permissions to
these execution phases so the future APK can choose project files, tools,
system/project instructions and model policy without moving control logic into
the Android UI.

## v0.6 — Universal Agent Control / APK workspace contract

v0.6 turns the existing Reasonix controller into the single universal backend
surface the Android APK will drive. It deliberately reuses native Reasonix
capabilities instead of creating an APK-only agent implementation.

- `/mod/agent` — aggregate APK-safe control state.
- `/mod/agent/tools` — enumerate tools and install ephemeral `allow|ask|deny`
  overrides. The overrides feed the native Reasonix permission gate; explicit
  upstream/config denies remain authoritative. Denied tools are also removed
  from the provider schema to save prompt tokens, but indirect capability calls
  still hit the same native deny gate.
- `/mod/agent/skills` — enumerate skills and persist enable/disable choices via
  the existing Reasonix skill configuration path. A toggle takes effect on the
  next native controller rebuild.
- `/mod/instructions` — edit recognized `REASONIX.md` / `AGENTS.md` /
  `CLAUDE.md` instruction documents through Reasonix `MemoryControl.SaveDoc`.
  This avoids arbitrary-file writes and preserves cache-first semantics: the
  new text is queued to the next turn and folds into the cache-stable prefix on
  the next session/rebuild.
- `/mod/workspace` and `/mod/workspace/validate` — expose the active project
  root and validate a future APK-selected root. Workspace switching is a
  supervisor operation: stop/restart `reasonix serve` with the selected project
  as CWD, rather than calling process-wide `chdir` inside a live agent.
- `/mod/workspace/files` and `/mod/workspace/file` — symlink-aware, workspace-
  confined browsing/text preview for the APK. No write endpoint is added; agent
  writes continue through Reasonix tools, permission gates and checkpoints.

Runtime tool overrides survive Flash/Pro native model switches because `serve`
re-applies the APK/session policy after a controller rebuild. No provider API key
is needed by the v0.6 smoke tests.

`POST /mod/agent/reload` performs a same-model native controller rebuild. The
APK uses it after changing skills or when it wants edited standing instructions
folded into a fresh cache-stable prefix immediately; runtime tool overrides are
re-applied to the replacement controller automatically.

## v0.7 — Capability Registry, environment/project profile, APK live protocol

v0.7 makes the v0.6 universal-control surface easier to extend without moving
agent logic into Android UI code.

### Capability Registry and Tool Packs

`GET /mod/capabilities` exposes the native Reasonix tool contract plus derived
APK packs (`basic`, `files`, `verify`, `vcs`, `shell`, `developer`). Packs are
only a UI/policy projection over the existing native registry; they do not
create wrapper tools or a second executor.

The effective tool policy has three layers:

1. native Reasonix/config policy (authoritative; an existing deny cannot be
   weakened),
2. APK manual `allow|ask|deny` overrides from v0.6,
3. v0.7 project-mode/tool-pack restrictions.

`chat` mode hides mutating tools from the provider schema while keeping the same
controller, conversation and model session. `agent` mode restores the selected
pack/manual policy. `use_capability` may remain visible in chat/read packs, but
resolved targets still pass through the same native per-tool permission gate;
a denied writer cannot be reached through the proxy.

### Native Reasonix environment projection

`GET /mod/environment` does **not** introduce a second environment detector.
It reuses upstream `internal/environment` — the same native probe list,
overrides, deny-root checks, in-memory cache and persisted snapshot path that
`boot.Build` uses to create the model-facing cache-stable `## Environment`
section. This keeps APK state and the agent's actual environment knowledge on
one source of truth instead of letting two probe systems drift.

The APK projection adds only cheap workspace/project-marker metadata (`go.mod`,
Gradle/Android files, `package.json`, Python/Rust markers, `.git`). An explicit
`GET /mod/environment` refreshes through the native cached probe path; ordinary
`/mod/status` reads the server's cached projection and therefore does not launch
version commands on every UI refresh. No model/provider call is involved.

### Project profile

`GET/POST /mod/project` manages a provider-agnostic APK/session profile:

- display name,
- `chat|agent` mode,
- selected Tool Packs,
- live-detail level (`metadata|project`).

The profile is intentionally APK-owned/replayable in v0.7 rather than silently
writing another project config format. Workspace switching remains a supervisor
restart operation from v0.6. Budget, model routing, skills and standing
instructions remain their existing independent Reasonix/Balance Mod systems.

### Typed live project protocol

`GET /mod/events` remains the streaming SSE transport. v0.7 adds typed `live.*`
events derived from Reasonix's native event stream and `GET /mod/live/history`
for a bounded recent action history.

The APK can render:

- visible assistant chat text,
- turn/phase state,
- tool start/finish/progress,
- redacted/capped tool argument and result previews when `liveDetail=project`,
- file diff previews and +/- counts,
- approval requests,
- workspace mutations,
- retry state,
- verification/completion summaries.

High-frequency text/tool-progress deltas are streamed but not retained in the
bounded history ring; completed visible chat messages and action/result events
are retained. This prevents one long response from evicting the useful project
audit trail.

**Hidden model reasoning is deliberately excluded.** `event.Reasoning` and the
`Message.Reasoning` chain are never mirrored to the APK protocol. The UI gets
observable plans/phases/actions/results and visible answer text instead.
Credential-like strings crossing the APK live-detail boundary are defensively
redacted and previews are bounded.

### v0.7 APK endpoints

Added:

- `GET /mod/capabilities`
- `GET /mod/environment`
- `GET /mod/project`
- `POST /mod/project`
- `GET /mod/live/history`

The normal Reasonix `POST /submit` remains the chat/agent input endpoint. The
project mode controls available capabilities; it does not create a second chat
model or duplicate session state.

The v0.7 offline gate must end with `BALANCE_MOD_V07_SMOKE_PASS`. No real
provider key is required or used by these tests.


## v0.8 — APK control plane, restart-safe policy state, Debian supervisor

v0.8 prepares the already-universal Reasonix backend for an actual Android
application without moving agent logic into the APK.

### Single APK bootstrap/control surface

`GET /mod/app/bootstrap` returns one bounded machine snapshot containing the
active workspace, model, project profile, KZT budget, execution router,
quality/recovery/resource state, tools, skills, persistence status and the
stable endpoint map the Android UI needs at startup.

`POST /mod/app/apply` atomically validates and applies an idle-session policy
bundle: project profile, KZT budget, execution model slots, per-tool decisions
and approval mode. It validates the whole request before mutating state and
rolls back in-memory policy if a later host apply/persist step fails. Budget
spend is preserved by default when settings are re-applied; the APK must send
`resetBudgetSpend:true` explicitly to start a fresh budget ledger.

`POST /mod/app/task/start` and `/mod/app/task/stop` are direct aliases to the
native Reasonix `submit` / `cancel` handlers. They do not introduce a second
agent loop, queue, session or provider path.

### Restart-safe workspace policy

APK-owned policy is stored atomically under the Reasonix home, keyed by a hash
of the canonical workspace path. The state file contains no API keys, prompts,
source files or hidden model reasoning.

Persisted state includes:

- project profile / Chat-vs-Agent mode / Tool Packs,
- manual per-tool decisions and approval mode,
- execution-router model refs/config,
- KZT budget configuration,
- **already-accounted KZT and Pro spend**.

Preserving the spend ledger is a hard requirement: restarting the backend must
not silently grant a fresh budget. Writes use a temporary `0600` file, fsync,
and atomic rename. Invalid/corrupt/impossible ledgers are rejected fail-closed
and surfaced in APK bootstrap rather than weakening the budget.

Native Reasonix-owned state remains native: Skills and instruction documents
continue to use Reasonix's own config/memory persistence rather than being
copied into this APK state file.

### Debian-side Android supervisor bridge

`scripts/reasonix_android_backend.sh` is a small process wrapper intended to be
invoked later through Termux `RUN_COMMAND` -> `proot-distro login debian`.
It supports `start|stop|restart|status|token|log`, binds the Reasonix HTTP/SSE
backend to localhost, creates a local auth token, stores PID/log/workspace state
under the Reasonix home, and starts the **same** `reasonix serve` backend in the
selected workspace.

Workspace switching remains process-level by design:

`APK -> Termux RUN_COMMAND -> Debian supervisor restart -> reasonix serve(CWD)`

This avoids process-wide `chdir` races and keeps one controller/session bound to
one workspace.

### v0.8 APK endpoints

Added:

- `GET /mod/app/bootstrap`
- `POST /mod/app/apply`
- `POST /mod/app/task/start` (native submit alias)
- `POST /mod/app/task/stop` (native cancel alias)
- `GET /mod/app/persistence`
- `POST /mod/app/persistence/save`

The v0.8 offline gate must end with `BALANCE_MOD_V08_SMOKE_PASS`. No real
provider key is required or used.


## v0.9 — Project registry + native task catalog for the APK

v0.9 adds the minimum project/task management layer needed by a real Android
frontend without creating a second scheduler, session format or agent runtime.

### Global project registry

`GET /mod/projects` exposes a small registry stored under the Reasonix home.
Projects are identified by a deterministic hash of their canonical workspace
path, so registering the same workspace twice updates one record instead of
creating duplicates. The registry stores only project metadata (name/path and
local timestamps); it contains no API keys, prompts, source contents or hidden
reasoning.

Project registration validates that the target is an existing directory.
Registry reads remain tolerant when a previously registered directory later
moves/disappears: the entry is returned with `available:false` instead of
corrupting the whole registry.

`POST /mod/projects/open` deliberately does **not** call process-wide `chdir` or
build a second controller. It returns a typed supervisor handoff:

`{restartRequired, supervisor:{action:"restart", workspace:"..."}}`

The future APK passes that workspace to the Debian supervisor from v0.8, which
restarts the same `reasonix serve` process in the selected project. Per-project
budget/tool/profile state continues to come from v0.8's workspace-scoped state,
so switching projects also switches to the correct preserved KZT ledger.

Project removal only unregisters metadata; it never deletes the user's project
files.

### Native task/session catalog

`GET /mod/tasks` is an APK-friendly view over `agent.ListSessions`. It performs
no LLM title request and therefore cannot spend provider tokens merely to draw
the history screen. It returns local titles/previews, turn counts, recovery
state and the native session path needed by Reasonix's existing resume route.

Task lifecycle intentionally stays native:

- create: `POST /new`
- resume: `POST /resume`
- delete: `POST /delete-session`
- run/stop: existing v0.8 `/mod/app/task/start|stop`

v0.9 adds only `POST /mod/tasks/rename`, which uses Reasonix's native
`agent.RenameSession` metadata ledger after validating that the target remains
inside the current session directory.

### v0.9 APK endpoints

Added:

- `GET /mod/projects`
- `POST /mod/projects/register`
- `POST /mod/projects/remove`
- `POST /mod/projects/open`
- `GET /mod/tasks`
- `POST /mod/tasks/rename`

The v0.9 offline gate must end with `BALANCE_MOD_V09_SMOKE_PASS`. No real
provider key is required or used.


## v0.10 — Native durable task queue + reviewed recovery + per-turn budgets

v0.10 does **not** add a Balance-owned scheduler. Reasonix already ships a
transactional per-session instruction queue in `internal/sessioninbox`, with
FIFO ordering, idempotency, bounded capacity, crash recovery, pause/resume,
reordering, retry and atomic per-item blobs. The APK layer now exposes that same
queue through a stable `/mod/queue/*` facade.

### Queue and crash recovery

`GET /mod/queue` projects the native inbox as APK-safe metadata: item id,
position, state, bounded preview, capacity and recovery status. Listing does not
call a provider. Queue bodies stay in the native per-session inbox blobs and are
not copied into a second Balance database.

The native recovery contract is preserved: a process interruption that leaves
owned/running work ambiguous moves it to `uncertain` and pauses the inbox. The
APK must review the recovery state. `POST /mod/queue/recovery/retry` accepts only
explicit ids currently in `uncertain|blocked`; it never blindly retries every
item. Automatic resume is intentionally false.

The server `Broadcaster` now forwards Reasonix's optional `InboxChanged`
capability into typed `queue.updated` Balance events, so the APK can redraw the
queue from native state changes without parsing terminal output or polling each
turn.

### Per-turn task budget

A queued APK item may carry a `taskBudget`:

- `budgetKzt`
- `tokenLimit`
- `wallSeconds`

Token/time ceilings are stored as typed inbox metadata and injected into the
existing `agent.WithTaskBudget` context when that durable item is actually run.
A KZT ceiling is converted to the active provider's price-book currency using
the workspace FX map and then uses the same native Agent task-cost gate. The
workspace KZT Governor remains authoritative and may stop work earlier; a queued
task never reserves or grants additional workspace money.

KZT admission is fail-closed: when the active provider has no billing currency
or the APK has not supplied the required KZT FX rate, the task is rejected
rather than being queued with an unenforceable money limit. If the workspace
hard budget has less money remaining than the requested task ceiling, the
effective task ceiling is clamped to that remaining workspace amount.

The generic inbox budget keys are interpreted only when the queued turn starts,
so normal chat and other frontends keep their existing budget behavior.

### v0.10 APK endpoints

Added:

- `GET /mod/queue`
- `POST /mod/queue/items`
- `PATCH /mod/queue/items/{id}`
- `DELETE /mod/queue/items/{id}`
- `POST /mod/queue/move`
- `POST /mod/queue/pause`
- `POST /mod/queue/resume`
- `POST /mod/queue/items/{id}/retry`
- `GET /mod/queue/recovery`
- `POST /mod/queue/recovery/retry`

The v0.10 offline gate must end with `BALANCE_MOD_V10_SMOKE_PASS`. No real
provider key is required or used.

## v0.11 — Unified power/economy engine + native turn evidence bridge

v0.11 removes the last duplicated host orchestration path between the Balance
repair policy and concrete model execution. `efficiency.PowerEngine` now owns
one deterministic sequence:

`host verification -> repair cycle -> objective route -> KZT admission -> native model switch`

It still never calls a model provider directly. Concrete model changes continue
to use Reasonix's existing `serve.switchModel` rebuild path, and a Pro route is
still diagnosis-only before returning execution to Flash.

### Native turn evidence

The serve frontend now derives a bounded, content-free turn observation from the
existing typed Reasonix event stream. It consumes mutation counts, shell
verification PASS/FAIL receipts, CompletionSummary and TurnDone readiness state.
A model's prose is not accepted as verification.

A mutating turn with no host verification is deliberately converted to a failed
verification receipt instead of being treated as complete. Failure identity is
stored as a truncated SHA-256 fingerprint; raw tool errors remain private host
state and are never exposed through `/mod/power`.

### No mid-turn model-switch race

TurnDone is emitted at a controller boundary where runtime teardown may still be
finishing. v0.11 therefore does **not** rebuild the model from inside that event
callback. The observed repair attempt becomes a pending route and is applied
only at an explicit idle boundary with:

- `GET /mod/power`
- `POST /mod/power/reset`
- `POST /mod/power/apply-pending`

This keeps Reasonix's invariant that `switchModel` cannot run while active work
or background jobs own the controller. A pending route remains pending if
budget/model admission fails, so the APK can adjust policy and retry rather than
silently consuming another strategy step.

The v0.11 offline gate must end with `BALANCE_MOD_V11_SMOKE_PASS`. No real
provider key is required or used.

## v0.12 — idle-boundary automatic continuation

Balance Mod can now advance a verified repair route without an APK button between
attempts. The orchestrator is host-owned: it waits for Reasonix's TurnDone
finishing boundary, temporarily pauses the native durable session inbox, applies
the pending PowerEngine route, places one bounded continuation at the front of
the same inbox, then resumes native dispatch.

Safety rules:

- direct HTTP task submission is gated only during the short model-transition
  lease; user work can still be added durably to the queue;
- an already user-paused queue is never overridden;
- KZT hard-stop work prepares a native per-turn cost budget before model
  switching, and fails closed if FX/pricing cannot enforce it;
- terminal/budget/no-progress routes never enqueue another turn;
- automatic continuations are bounded (default 4, maximum 12);
- the pending route remains available for manual APK review whenever automation
  is refused;
- no hidden reasoning is exported or used as a completion signal.

APK endpoints: `GET /mod/orchestrator`, `POST /mod/orchestrator/config`,
`POST /mod/orchestrator/stop`, and `POST /mod/orchestrator/resume`.

## v0.13 — durable route recovery + hard Pro diagnosis sandbox

The two known v0.12 safety gaps are closed before the APK protocol is frozen.
A host-derived pending power route is persisted in sanitized form before an
automatic model transition. Raw logs, fix hints, prompts, source and hidden
reasoning are never written into that state. Continuations use a stable native
session-inbox idempotency key, so a restart after durable enqueue replays the
same receipt instead of creating a second model turn. Restored pending routes
require an explicit APK resume after backend restart.

`pro_diagnosis` is now a native permission/schema boundary, not merely a prompt
instruction: every mutating tool is denied and removed from the provider schema
while read-only tools remain available. The `use_capability` proxy can stay
visible because its resolved target still passes through the same native deny
policy. A failed model switch rolls the temporary execution-policy overlay back.


## v0.14 — APK v1 contract freeze + offline prototype gate

The Android-facing API now exposes `GET /mod/app/contract`, a deterministic
`balance-apk-v1` manifest containing the supported endpoint/method surface,
typed event names, compatibility rules and backend safety guarantees. The
contract digest is also returned by `/mod/app/bootstrap`, so an APK can detect
that it is speaking to the expected backend before enabling mutating controls.

Within protocol major v1, additive JSON fields/endpoints/events are allowed and
clients must ignore unknown response fields. Removing/renaming a frozen endpoint
or changing its method/meaning requires a protocol-major bump rather than a
silent APK break. Mutating requests remain JSON/CSRF guarded. Hidden reasoning
is explicitly outside the protocol; the app consumes visible plans, actions,
diffs, results and verification events.

`scripts/balance_mod_offline_prototype.sh` is the first process-level offline
prototype gate. It starts the real `reasonix serve` binary on localhost with the
zero-cost MockProvider, negotiates the v1 contract, atomically applies an APK
profile/budget, starts a task through the APK alias, waits for the real live-event
path to surface `OFFLINE_MOCK_PASS`, verifies zero KZT spend, and switches the
same backend session into chat mode. It unsets common provider key environment
variables for the child process and requires no external provider/network call.

The v0.14 full gate ends with `BALANCE_MOD_V14_SMOKE_PASS`.


## v0.15 — offline stress gate before any real provider

v0.15 deliberately adds very little production surface. The goal is to attack
existing assumptions before a real API key is ever accepted by the workflow.
The zero-cost MockProvider gains a `deny-bypass` scenario that first verifies a
denied writer is absent from the provider schema and then deliberately attempts
the same writer through `use_capability`. Success requires the resolved target
to be rejected by Reasonix's native permission gate; merely hiding a tool from
the schema is not considered sufficient.

`scripts/balance_mod_offline_stress.sh` runs the real localhost serve process in
an isolated `REASONIX_HOME` with all common provider-key environment variables
unset. It verifies, in one process-level scenario:

- frozen APK v1 safety guarantees are still negotiated;
- `write_file=deny` survives a capability-proxy bypass attempt and no file is
  created;
- a queued task requesting more KZT than the workspace budget is clipped to the
  workspace ceiling before admission;
- rollback without an available checkpoint fails closed;
- an abrupt backend kill followed by explicit native session resume preserves
  workspace budget, tool policy and the paused durable inbox item;
- malformed APK persistence is reported as an error without crashing the
  backend, and a valid saved state can subsequently be restored;
- an invalid budget mutation is rejected without changing the prior budget;
- Balance Mod state contains no provider-key-shaped data.

The crash test intentionally keeps the inbox paused. v0.15 does **not** claim
that arbitrary in-flight model work is exactly-once across process death; the
stronger guarantee remains the reviewed/uncertain native inbox recovery policy
and the v0.13 idempotent auto-continuation path.

The v0.15 quick gate ends with `BALANCE_MOD_V15_QUICK_PASS`; the full gate ends
with `BALANCE_MOD_V15_SMOKE_PASS`. No real provider key is required or used.

## v0.16 — hard pre-call provider budget admission

v0.15 proved that the offline control plane survives permission bypass attempts,
queue recovery, state corruption and process restart. One important money-safety
gap remained before a real provider could be enabled: Reasonix's native task
cost budget and the Balance KZT ledger historically observe actual spend after a
model round. That stops a runaway after the round, but a single very large
completion (or bounded retry sequence) could still overshoot a small hard budget.

v0.16 closes that gap at the provider-I/O boundary. When the APK configures a
hard KZT budget and the active model has known pricing plus a KZT FX rate,
`applyModTaskCostBudget` now enables a native Agent pre-call guard in addition to
the existing post-round ledger. Before `Provider.Stream` is entered, the Agent:

- computes a conservative prompt-token upper bound from the complete prepared
  provider request bytes plus framing headroom;
- reserves the remaining cost/token envelope across the full bounded sampling
  retry × provider-header retry window;
- charges input at the worst known input/cache rate for admission purposes;
- derives a maximum affordable output-token ceiling and clamps `MaxTokens`
  before network I/O;
- fails closed before the provider call when pricing is unavailable, the prompt
  cannot fit the retry-reserved share, or no completion tokens remain.

The guard is host-controlled and disabled by default for ordinary Reasonix
sessions. Balance enables it only when the KZT hard task-cost gate is actually
resolved. The APK-visible `taskCostGate.preCall` bit reports whether that
pre-network boundary is active; merely having a KZT ledger is not presented as
hard pre-call enforcement.

Compaction summaries are routed through the same guarded provider-call seam, so
an automatic compaction cannot bypass the hard budget. No tokenizer service,
extra model call or API key is required for admission; the byte-bound estimate is
local and intentionally conservative for Android.

`scripts/balance_mod_precall_budget.sh` starts the real localhost serve binary
with a non-zero-priced offline MockProvider, applies a KZT hard budget, confirms
`taskCostGate.preCall=true`, starts a task through the frozen APK API, and
requires the mock to observe a provider request whose large baseline output
budget was physically reduced before `Stream` executed. Provider credential
environment variables are unset for the child process.

The v0.16 full gate must end with `BALANCE_MOD_V16_SMOKE_PASS`. No real provider
or API key is used by this version.

### v0.16 hard-budget side-path policy

A pre-call cap on the executor alone is not enough to call the workspace budget
"hard": Reasonix can also spend through planner, semantic capability routing,
session-title generation, and delegated sub-agents. v0.16 therefore makes hard
budget mode deliberately single-agent until those secondary providers share one
atomic reservation ledger:

- two-model Coordinator planning is forced to executor-only while strict
  pre-call admission is active;
- semantic capability routing falls back to deterministic routing (no extra
  classifier model call);
- cosmetic session-title generation is skipped and uses the normal preview;
- provider-spawning delegation tools (`task`, `read_only_task`,
  `parallel_tasks`, `fleet`, subagent skill/research/review entry points) are
  blocked at execution, including proxy-resolved targets. Local tools and
  `read_skill` remain available;
- a native model rebuild re-applies the strict gate and uses the **remaining**
  KZT-derived provider allowance, not the original total, so switching
  Flash→Pro→Flash cannot re-grant already spent money;
- a hard budget with missing billing currency/FX fails closed. Ordinary task
  admission is rejected rather than silently dropping to post-round-only
  accounting;
- queued token/wall overrides may tighten the hard cost limit but cannot erase
  it;
- the current supported worst-case cache-write input-equivalent multiplier is
  reserved conservatively for prompt admission.

This is intentionally stricter than ordinary Reasonix. It sacrifices optional
secondary-model power under `hardStop=true` rather than making a budget promise
that the code cannot enforce. A future shared reservation ledger can re-enable
secondary agents without weakening that guarantee.
