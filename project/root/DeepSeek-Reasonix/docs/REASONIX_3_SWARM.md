# Reasonix 3.0 — Swarm Core

Design grounded in the actual Reasonix codebase (branch `main-v2`, Balance Mod
v0.20 baseline). The swarm is a real core runtime inside Reasonix, not a
frontend simulation or concatenated completions.

## Reused Reasonix primitives (no second agent environment)

Every worker turn is an ordinary `agent.Agent.Run(ctx, input)` — the same real
Reasonix turn primitive that `control.Controller` drives behind the CLI, HTTP/SSE
and desktop. The swarm only selects the pieces a worker turn needs:

- provider: resolved per worker from the configured catalog (`config.Load` /
  `config.ResolveModel` / `provider.Resolver`), so DeepSeek, Kimi/Moonshot and
  any future configured provider work identically.
- tools: a scoped `tool.Registry` built from the real registry
  (`SetSessionHiddenTools` / `SetProviderVisibleTools` / `Schemas()`), filtered
  to the worker profile's allowed tool names. Same registry type the whole app
  uses; no wrapper tool executor.
- session: one isolated `agent.Session` per worker (bounded context only).
- budget: `agent.Options.TaskBudget` + `agent.WithTaskBudget` context + pricing,
  so the native task-cost gate and the Balance strict pre-call guard apply to
  workers unchanged.
- events: `event.Sink` fanned into the shared swarm sink; new swarm `Kind`s are
  appended above `event.KindCount` (wire-stable per documented policy) and the
  serve layer projects them into the existing `live.*` APK protocol.
- approval: worker mutations that need approval ride the existing
  `ApprovalRequest` path (`control.Approvals`), not a new gate.
- persistence: swarm state lives under the Reasonix home using the same
  atomic-0600-write discipline as the Balance APK state (`fileutil.AtomicWriteFile`).

## Architecture

```
user objective
      │
┌─────▼────────────────────────────────────────────┐
│ internal/swarm.Orchestrator                       │
│  owns global objective; decides usefulness,       │
│  decomposition, deps, worker count, profiles,     │
│  providers/models, concurrency, budgets, stop,    │
│  verification, result integration                 │
└─────┬────────────────────────────────────────────┘
      │ Plan (task graph)                          │
┌─────▼────────────────────────────────────────────┐
│ Scheduler (bounded parallelism)                  │
│  runnable = deps satisfied ∧ budget ∧ limits ∧   │
│  cancellation open                                │
│  concurrency cap, provider cap, KZT cap          │
└─────┬──────────┬──────────┬──────────┬───────────┘
      │          │          │          │
 worker.1     worker.2   worker.3   ...        each: scoped tool.Registry,
      │          │          │          │        isolated agent.Session,
      │          │          │          │        resolved provider/model,
      │          │          │          │        TaskBudget, WriteRoots,
      │          │          │          │        shared event.Sink
      └──────────┴──────────┴──────────┘
              evidence / findings / artifacts (shared state)
                      │
┌─────────────────────▼────────────────────────────┐
│ Integrator + Verifier                            │
│  structured combination (not prose concat),      │
│  conflict surfacing, task-specific evidence,     │
│  integrated-output verification                  │
└─────────────────────┬────────────────────────────┘
                      │
                 final result / SwarmDone / SwarmFailed
```

## Task graph

Explicit `Task` state (authoritative, not transcript):

- id, objective, status, dependencies, worker assignment, profile,
  model/provider, scope (owned write paths), evidence requirements, result,
  failure, retry count, timestamps.

## Agent profiles

Generic; the architecture does not depend on exact names. Each profile may set:
instructions, allowed tools, provider/model preference, context window, budget,
timeout, evidence requirements, owned-write scope. Example profiles shipped in
`.reasonix/skills` already exist (debugger, verifier, project-architect,
web-researcher); a swarm run may assign any profile string and it resolves at
schedule time.

## Failure classes

temporary, permanent, approval_wait, tool_missing, schema_error, provider_error,
budget_stop, timeout, no_progress, dependency_failure, merge_conflict, cancelled.
Permanent failures are never blindly retried.

## Mutation ownership

Workers get disjoint owned write scopes (`agent.Options.WriteRoots` /
`WriteWorkspaceRoot`). The orchestrator derives each worker's scope from the
plan; overlapping scope is expressed as a dependency edge instead of a parallel
edge, so no two workers can mutate the same file concurrently. Read-only workers
get an empty owned scope.

## Budget / cancel / recovery

- total swarm budget (KZT, via `efficiency.Governor` when enabled, plus native
  task cost), worker budget, request/token limits, provider/model constraints,
  concurrency limits.
- cancellation at worker / task / swarm granularity via context cancellation and
  durable state; cancelling a worker never corrupts project/session state.
- swarm state is persisted incrementally; on restart the orchestrator resumes
  unowned/completed tasks and does not re-pay for completed evidence.

## Determinism and cost control

Default offline: all swarm tests run against the zero-cost MockProvider
(`internal/provider/mock`). Real provider/swarm tests are separately gated
exactly like the Balance v0.20 real gate.

## Serve / APK surface

Registered under the frozen `balance-apk-v1` protocol (additive within
protocol-major v1):

- `POST /mod/swarm/start` — start a bounded swarm run (background).
- `POST /mod/swarm/cancel` — cancel the active swarm.
- `GET /mod/swarm` — active swarm state (or the most recent persisted one).
- `GET /mod/swarm/{id}` — persisted structured state by id.
- `GET /mod/swarm/history` — recent persisted swarm states.

Typed `swarm.*` events stream through the existing SSE broadcaster
(`swarm_*` wire kinds). Hidden reasoning is never exported to the APK
protocol, so the contract does not advertise a reasoning event.
