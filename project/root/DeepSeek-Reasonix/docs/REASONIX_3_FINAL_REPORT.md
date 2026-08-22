# Reasonix 3.0 Night Run — Final Evidence Report

Date: 2026-08-20 (local device: Android ARM64, root, proot-Debian/Termux)

## BASELINE_VERIFIED
- Repo: `/root/DeepSeek-Reasonix`, branch `main-v2`, HEAD `9e68643`
  (docs release v1.25.4). Dirty tree: 24 modified tracked + 125 untracked
  files at start (Balance Mod v0.20-fixed12 WIP).
- Go 1.26.4 `linux/arm64`, `GOTOOLCHAIN=local`; `go build ./...` exit 0.
- Single real provider configured: `custom-api-deepseek-com` (openai kind,
  `https://api.deepseek.com`, models `deepseek-v4-flash`/`deepseek-v4-pro`);
  global config adds anthropic-kind `deepseek-flash`/`deepseek-pro`.
- Balance Mod real-provider gate is **locked**: `BALANCE_V20_REAL_API_APPROVED`
  unset; `balance_mod_v020_real_gate.sh` fails closed
  `BALANCE_V20_REAL_GATE_LOCKED` (exit 20).
- No swarm code existed anywhere in the tree before this run.

## ROOT_CAUSES_FOUND
1. Strict pre-call budget guard did not clamp unbounded `MaxTokens` to the
   provider ceiling; a generous budget produced 495674 > official 384000
   (server would reject). FIXED (`internal/agent/run_budget.go`):
   unbounded -> ceiling, budget caps tighten, ceiling-minus-reserve clamp.
   Positive test `TestStrictPreCallBudgetCapsUnboundedOutputBeforeProviderIO`.
2. Two tests assumed `chmod 0` forces read/write failure; as root
   (CAP_DAC_OVERRIDE) it does not. FIXED deterministically:
   `internal/agent/reasoning_warn_state_test.go` (state path swapped for a
   directory -> EISDIR), `internal/config/edit_test.go` (target swapped for a
   directory -> rename EISDIR).
3. Android/PRoot filesystem rejects 255-byte filename components despite
   `NAME_MAX=255` (verified: max is 254). `TestSaveBoundsLongNames` and
   `TestStatsPathBoundedForLongNames` failed with ENAMETOOLONG. SYSTEMIC FIX:
   new `config.FilenameComponentMaxBytes = 254` used by `WorkspaceSlug`,
   memory `slug`, plugin `statsPath`.
4. Stale `scripts/balance_mod_precall_budget.sh` encoded the v0.20-fixed9
   obsolete 66-way retry-reserve split (budget 1000 KZT -> cap ~7M tokens).
   ALIGNED with one-paid-attempt policy (budget 15 KZT -> cap ~107K < mock
   128K baseline). `BALANCE_MOD_PRECALL_BUDGET_PASS`.
5. Background swarm must not bind to the HTTP request context (swarm would be
   cancelled when the handler returns). FIXED: server-owned context.
6. `internal/control` and `internal/acp` showed intermittent wall-clock test
   flakes under full-suite parallelism on this ARM64 device; both pass in
   isolation (noted, no code change).

## REASONIX_2_STATE
- Complete: turn lifecycle, continuation, cancellation, tool loop, Skills,
  MCP, projects, provider handling, sessions/recovery, progress-aware
  anti-loop, usage/budgets, typed event transport, approval invariant — all
  present via upstream + Balance Mod v0.20-fixed12.
- Offline release gate chain: `BALANCE_MOD_V16_SMOKE_PASS` ->
  `V17_SMOKE_PASS` -> `V18_SMOKE_PASS` -> `V19_SMOKE_PASS` ->
  `V20_TARGETED_PASS` -> `BALANCE_MOD_V20_PREFLIGHT_PASS` (EXIT 0).
- Full repo `go test ./...`: 124 packages ok, EXIT 0.
- Approval invariant preserved: mutations ride native `ApprovalRequest`/WAIT;
  pending approval is not failure/retry/no-progress (native controller
  behavior, verified by existing approval e2e tests).

## REASONIX_3_ARCHITECTURE
New `internal/swarm` package (real runtime, not concatenated completions):
- Orchestrator owns the global objective: decides usefulness/decomposition,
  deps, worker count, profiles, providers, concurrency, budgets, stop
  conditions, verification, result integration.
- Structured task graph (`Task`, `TaskStatus`, `FailureClass`, `EvidenceKind`).
- Generic agent profiles (not name-dependent).
- Workers are real `agent.Agent.Run` turns with scoped `tool.Registry`,
  isolated `agent.Session`, provider-agnostic `Resolver`, native
  `ReadOnlyExecution` for read-only profiles.
- Structured shared state (findings/artifacts/evidence/test results).
- Bounded parallelism (MaxWorkers, provider slots, budget ledger).
- Mutation ownership: read-only workers enforced; write scopes via
  `WriteRoots` disjointness (documented; full enforcement wired for the
  read-only boundary).
- Result integration (structured, not prose concatenation) + verification.
- Failure taxonomy: temporary/permanent/approval_wait/tool_missing/schema_
  error/provider_error/budget_stop/timeout/no_progress/dependency_failure/
  merge_conflict/cancelled. Permanent never blindly retried.
- Cancellation at swarm granularity (worker/task via context).
- Recovery: durable `Store` under `config.SwarmStateDir()`, atomic 0600 JSON.
- Events: 18 `Swarm*` Kinds appended above `event.KindCount` (wire-stable);
  `eventwire` + desktop `types.ts` updated; reasoning not advertised to APK.

## SWARM_IMPLEMENTED
- `internal/swarm/`: types.go, profile.go, planner.go (deterministic
  planner: trivial -> 1 worker; `;`/newline segments -> parallel; "then/after"
  -> dependency), orchestrator.go (Run/Cancel/Snapshot/schedule/runTask),
  worker.go (agent.Agent runtime + evidence collector + failure classifier),
  resolver.go (config-backed, provider-agnostic), budget.go, store.go.
- Serve: `internal/serve/mod_swarm.go` — `POST /mod/swarm/start|cancel`,
  `GET /mod/swarm|/mod/swarm/{id}|/mod/swarm/history`; structured state
  persisted; swarm events fan into the existing broadcaster.
- APK contract: `balance-apk-v1` now 72 endpoints / 85 events (additive).

## FILES_CHANGED (this run)
- Fixed: `internal/agent/run_budget.go`,
  `internal/agent/reasoning_warn_state_test.go`,
  `internal/config/edit_test.go`, `internal/config/paths.go`,
  `internal/memory/store.go`, `internal/plugin/stats.go`,
  `internal/plugin/stats_test.go`, `scripts/balance_mod_precall_budget.sh`.
- New: `internal/swarm/` (11 files), `internal/serve/mod_swarm.go`,
  `internal/serve/mod_swarm_test.go`, `scripts/balance_mod_swarm_gate.sh`,
  `docs/REASONIX_3_SWARM.md`.
- Extended: `internal/event/event.go`, `internal/eventwire/wire.go`,
  `internal/serve/serve.go`, `internal/serve/mod_contract.go`,
  `internal/serve/mod_contract_test.go`,
  `desktop/frontend/src/lib/types.ts`.

## CONTRACTS_CHANGED
- `balance-apk-v1` frozen surface extended additively (72 endpoints / 85
  events); contract digest test updated to the new inventory.
- 18 new `Swarm*` event kinds appended above `event.KindCount` (existing kinds
  wire-stable). `swarm_*` wire names + `swarm` payload added to eventwire and
  desktop types.
- New `config.FilenameComponentMaxBytes = 254` (Android/FUSE-safe).

## INVARIANTS_ADDED
- Strict pre-call cap is never above the provider ceiling and never exactly at
  it (completion-margin reserve).
- A state read failure never overwrites existing warn incidents; a failed
  symlink-target write never replaces the symlink or mutates the target.
- Derived filename components never exceed 254 bytes.
- Swarm: worker "done" requires host evidence (provider + readback minimum;
  runtime when tools ran); permanent failures not retried; hidden reasoning
  never exported to the APK contract; swarm run is not bound to an HTTP
  request context.

## LOCAL_TESTS
- `go test ./...`: 124 packages ok, EXIT 0.
- Swarm unit tests: trivial/parallel/dependency/cancel/budget/failure/events/
  persister/store/resolver. Serve swarm e2e (offline mock): pass.
- `go vet ./...` exit 0; `gofmt -l` clean on touched Go files.

## REGRESSION_TESTS
- `scripts/balance_mod_v020_preflight.sh` -> `BALANCE_MOD_V20_PREFLIGHT_PASS`
  (covers V16..V20 cumulative gates, offline prototype/stress/precall/crash).
- `scripts/balance_mod_swarm_gate.sh` -> `BALANCE_MOD_SWARM_GATE_PASS`
  (real binary + real HTTP + mock provider + 2 parallel workers, 0 KZT spend).

## FIRST_REAL_CHAT_TEST
TESTING_BLOCKED — real provider approval env
`BALANCE_V20_REAL_API_APPROVED` is unset; gate fails closed.

## REAL_TOOL_TEST
TESTING_BLOCKED (same gate).

## REAL_APPROVAL_TEST
TESTING_BLOCKED (same gate). Offline approval/queue/deny-bypass gates pass.

## REAL_SWARM_TEST
TESTING_BLOCKED (same gate). Offline 2-worker swarm e2e (serve endpoint,
real binary, mock provider) passes.

## DEEPSEEK_RUNTIME
CONFIG VERIFIED (deepseek-v4-flash/pro configured). MODEL READINESS
NOT_VERIFIED on device (no approved call). REAL ROUTING NOT_VERIFIED.

## KIMI_RUNTIME
TESTING_BLOCKED — no Kimi/Moonshot credentials or config present.

## REAL_ROUTING
NOT_VERIFIED — locked behind explicit approval.

## BUDGET_USAGE
0 real provider tokens spent this run. All gates used the zero-cost
MockProvider or local-only work.

## APK
No Android source changed; APK not rebuilt (backend is additive). Desktop
`types.ts` updated for the new event kinds. `bin/reasonix` rebuilt
(SHA256 `588fe958032bc136ba41f95d310c0d06decd44e4b17de7825cadf11c6995e9e5`).

## PASS
- Full offline release gate chain (V16..V20 preflight).
- `go test ./...` (124 pkgs).
- Deterministic swarm core tests + serve swarm e2e + process swarm gate.
- Real-gate locked-by-default behavior.
- Root-cause fixes 1-5 above.

## NOT_VERIFIED
- Real DeepSeek routing/usage/readback (approval locked).
- Kimi routing.
- Real swarm integration with a real provider.
- repolint baseline: the tree exceeds the committed baseline due to the
  pre-existing Balance Mod WIP; the new `internal/swarm` package itself has no
  findings. golangci-lint not installed on-device (run in CI).

## TESTING_BLOCKED
- Real chat / tool / approval / swarm / DeepSeek / Kimi (explicit approval
  env absent; by design the gate stays locked).

## READY_FOR_DEVICE_TEST
- APK: no new feature UI was added this run; the swarm surface is backend-only
  and ready for an APK/device pass once approval is granted.
- Set `BALANCE_V20_REAL_API_APPROVED=YES_I_EXPLICITLY_APPROVE_DEEPSEEK_API`,
  `BALANCE_V20_USD_KZT=<rate>`, `BALANCE_V20_BUDGET_KZT<=25` to unlock the
  real gates.

## ARTIFACTS
- `bin/reasonix` (fresh build).
- `internal/swarm/` source + tests.
- `scripts/balance_mod_swarm_gate.sh`, `docs/REASONIX_3_SWARM.md`.
- Gate logs: `/tmp/v020_preflight2.log`, `/tmp/full_test_suite5.log`,
  `/root/reasonix-night-20260820-170419.log`.

## SHA256
- `bin/reasonix` = `588fe958032bc136ba41f95d310c0d06decd44e4b17de7825cadf11c6995e9e5`
- Source bundle (`git status` tree, not committed): created on request.

## ROLLBACK
- Working tree changes are uncommitted; `git checkout -- <tracked>` +
  `rm` of untracked new files reverts to HEAD `9e68643`.
- Balance Mod backups preserved under `.balance_mod_backups/` and
  `reasonix-balance-mod-v0.20-fixed12-bundle/`.

## NEXT_SINGLE_ACTION
Grant the real-provider approval env, then run REAL TEST 1
("Привет. Коротко ответь: Reasonix работает.") through the real Reasonix
path, then the FIRST REAL SWARM TEST (1 orchestrator + 2 workers) and the
Kimi heterogeneous test if credentials appear.
