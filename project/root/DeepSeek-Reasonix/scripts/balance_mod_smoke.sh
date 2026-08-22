#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-go}"

cd "$ROOT"
echo "[1/52] Unit tests: Balance Mod core + APK bridge"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency ./internal/provider/mock ./internal/serve

echo "[2/52] Objective power router: Flash -> Flash -> Pro diagnose -> Flash"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^TestEscalatorFlashFlashProFlash$' -count=1

echo "[3/52] Failure cache: verified-only persistence + corruption recovery"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^TestFailureCache' -count=1

echo "[4/52] Context/patch economy: log reducer + patch governor"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^(TestReduceBuildLog|TestPatchGovernor)' -count=1

echo "[5/52] Unified repair cycle: verifier -> router -> cache"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^(TestVerificationReceiptRequiresHostEvidence|TestRepairCycleFlashFlashProFlashAndVerifiedCache)$' -count=1

echo "[6/52] Offline MockProvider drives Flash -> Flash -> Pro -> Flash repair cycle"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^TestRepairCycleDrivenByOfflineMockProvider$' -count=1

echo "[7/52] Recovery policy: regression/oversize rollback + fail-closed conflict"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^TestRepairCycleRollback' -count=1

echo "[8/52] APK cycle/recovery API integration"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^(TestModCycleAPIAndHostRepairWiring|TestModRecoveryAPIIsFailClosedWithoutCheckpoint)$' -count=1

echo "[9/52] Execution router: model slots + immediate KZT admission re-check"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^TestExecutionRouter' -count=1

echo "[10/52] Native Reasonix model-switch path: Flash B -> Pro -> Flash repair"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^TestModExecutionRouterUsesNativeServeModelSwitch$' -count=1

echo "[11/52] APK execution config/status contract"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^(TestModExecutionAPIIsStrictAndAPKVisible|TestModStatusExposesTypedAPKSurface)$' -count=1

echo "[12/52] Universal agent control: native per-tool allow/ask/deny + schema trim"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/control ./internal/tool -run '^(TestSessionToolDecisions|TestSessionHiddenTools)' -count=1

echo "[13/52] APK agent/tools API contract"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^TestModAgentToolsAPIUsesNativeRuntimePolicy$' -count=1

echo "[14/52] APK instructions + workspace file bridge"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^(TestModInstructionsAndWorkspaceFileAPI|TestModWorkspaceFileRejectsTraversal)$' -count=1

echo "[15/52] Provider-agnostic project profile"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^TestProjectProfile' -count=1

echo "[16/52] Capability Registry + Chat/Agent mode + Tool Packs"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^(TestModProjectChatModeHidesMutatingToolsAndRestoresAgentMode|TestModProjectToolPackRestrictsSurfaceAndManualDenySurvives|TestModEnvironmentAndProjectContract)$' -count=1

echo "[17/52] APK live protocol: actions/diff/results visible, hidden reasoning excluded"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^TestModLive' -count=1

echo "[18/52] Restart-safe KZT ledger persistence"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^TestBudgetPersistentState' -count=1

echo "[19/52] APK bootstrap + atomic apply + restart persistence"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^TestModApp' -count=1

echo "[20/52] APK project registry + supervisor handoff"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^TestModProjectRegistryAndSupervisorHandoff$' -count=1

echo "[21/52] Native task catalog + local rename (no provider listing call)"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^TestModTaskCatalogUsesNativeSessionsAndRename$' -count=1

echo "[22/52] APK durable task queue + per-turn budget metadata"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve ./internal/control -run '^(TestModQueue|TestParseInboxTaskBudget)' -count=1

echo "[23/52] Unified power engine: repair + router + budget + native execution"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^TestPowerEngine' -count=1

echo "[24/52] Native turn evidence -> pending power route (no mid-turn model race)"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^TestModPowerTurnTracker' -count=1

echo "[25/52] APK unified power endpoint + explicit idle-boundary apply"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^(TestModPowerAPIExposesUnifiedState|TestModPowerPendingRouteAppliesOnlyAtExplicitIdleBoundary)$' -count=1

echo "[26/52] Auto continuation policy: bounded lease + terminal route mapping"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^(TestModAutoOrchestratorLeaseAndLimit|TestModAutoContinuationPromptsCoverOnlyNonTerminalRoutes)$' -count=1

echo "[27/52] Auto continuation safety: direct-submit gate + fail-closed without execution routing"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^(TestModAutoSubmitGateRejectsDirectHTTPDuringTransition|TestModAutoScheduleFailsClosedWithoutExecutionRouting)$' -count=1

echo "[28/52] APK orchestrator config/stop contract"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^TestModAutoAPIConfigAndStop$' -count=1

echo "[29/52] v0.13 execution-mode guard rollback semantics"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^TestExecutionRouterModeGuardRunsBeforeSwitchAndRollsBackOnFailure$' -count=1

echo "[30/52] v0.13 Pro diagnosis is physically read-only"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^TestModProDiagnosisHardDeniesMutatingTools$' -count=1

echo "[31/52] v0.13 pending route survives restart without raw log/prompt leakage"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^TestModPendingPowerRoutePersistsSanitizedAcrossRestart$' -count=1

echo "[32/52] Native session-inbox crash recovery regression"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/control -run '^TestInboxSnapshotRecoversUnownedInFlightItem$' -count=1

echo "[33/52] Debian Android backend supervisor syntax"
bash -n ./scripts/reasonix_android_backend.sh

echo "[34/52] Native Reasonix completion gate regression test"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/agent -run '^TestFinalReadinessBlocksUntilProjectCheckRunsAfterWriter$' -count=1

echo "[35/52] Build CLI only (skip example plugin)"
mkdir -p bin
GOTOOLCHAIN=local CGO_ENABLED=0 "$GO_BIN" build -o bin/reasonix ./cmd/reasonix

TMP="$(mktemp -d)"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT
printf 'print("TEST OK")\n' > "$TMP/hello.py"
cat > "$TMP/reasonix.toml" <<'TOML'
default_model = "balance-mock"

[[providers]]
name = "balance-mock"
kind = "mock"
model = "smoke"
base_url = "http://127.0.0.1"
context_window = 1000000
price = { cache_hit = 0, input = 0, output = 0, currency = "KZT" }

[environment]
offline = true

[sandbox]
bash = "off"
TOML

echo "[36/52] Offline normal tool-loop"
cd "$TMP"
OUT="$("$ROOT/bin/reasonix" run --model balance-mock --max-steps 6 "Run the offline Balance Mod smoke scenario." 2>&1 || true)"
printf '%s\n' "$OUT"
if ! grep -q 'OFFLINE_MOCK_PASS' <<<"$OUT"; then
  echo "BALANCE_MOD_SMOKE_FAIL: normal mock provider did not reach final pass" >&2
  exit 1
fi

cat > "$TMP/reasonix.toml" <<'TOML'
default_model = "balance-mock-loop"

[[providers]]
name = "balance-mock-loop"
kind = "mock"
model = "repeat-failure"
base_url = "http://127.0.0.1"
context_window = 1000000
price = { cache_hit = 0, input = 0, output = 0, currency = "KZT" }

[environment]
offline = true

[sandbox]
bash = "off"
TOML

echo "[37/52] Offline anti-loop intervention"
LOOP_OUT="$("$ROOT/bin/reasonix" run --model balance-mock-loop --max-steps 8 "Investigate the requested file. If a host guard detects repetition, obey it and stop retrying the same failed action." 2>&1 || true)"
printf '%s\n' "$LOOP_OUT"
if ! grep -q 'OFFLINE_LOOP_GUARD_PASS' <<<"$LOOP_OUT"; then
  echo "BALANCE_MOD_SMOKE_FAIL: native loop guard did not redirect the repeat-failure mock" >&2
  exit 1
fi

# Balance Mod v0.14.2: return from temp workspace before repo-relative tests.
cd "$ROOT"
echo "[38/52] v0.14 frozen APK v1 contract + bootstrap negotiation"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^TestModAPKContract' -count=1

echo "[39/52] v0.14 full localhost HTTP offline prototype"
PATH="$(dirname "$(command -v "$GO_BIN")"):$PATH" GOTOOLCHAIN=local ./scripts/balance_mod_offline_prototype.sh

echo "[40/52] Universal agent-control contract summary"
echo "  - workspace root + confined file browser: PASS"
echo "  - per-tool allow/ask/deny via native gate: PASS"
echo "  - instruction editor via native memory docs: PASS"
echo "  - skill management surface: compiled in core serve tests"

echo "[41/52] Universal APK protocol summary"
echo "  - dynamic Capability Registry + Tool Packs: PASS"
echo "  - Chat mode denies mutating tools without creating a second agent: PASS"
echo "  - APK environment endpoint reuses Reasonix native probes/cache + project markers: PASS"
echo "  - project profile is provider-agnostic and APK-replayable: PASS"
echo "  - live event history exposes actions/diffs/results but not hidden reasoning: PASS"

echo "[42/52] APK supervisor/control-plane summary"
echo "  - bootstrap + atomic settings apply: PASS"
echo "  - task start/stop aliases reuse native submit/cancel: PASS"
echo "  - workspace-scoped policy survives backend restart: PASS"
echo "  - KZT/Pro spend ledger survives restart: PASS"
echo "  - Debian Android supervisor wrapper syntax: PASS"

echo "[43/52] APK v1 protocol freeze summary"
echo "  - machine-readable endpoint/event manifest: PASS"
echo "  - contract digest exposed in bootstrap: PASS"
echo "  - breaking endpoint semantics require protocol-major bump: PASS"
echo "  - localhost HTTP -> mock tool-loop -> live history: PASS"

echo "[44/52] v0.15 native deny-bypass mock policy regression"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/provider/mock -run '^TestDenyBypassScenarioRequiresSchemaTrimAndNativeDenial$' -count=1

echo "[45/52] v0.15 offline stress harness syntax"
bash -n ./scripts/balance_mod_offline_stress.sh

echo "[46/52] v0.15 full localhost crash/restart/policy stress gate"
PATH="$(dirname "$(command -v "$GO_BIN")"):$PATH" GOTOOLCHAIN=local ./scripts/balance_mod_offline_stress.sh

echo "[47/52] v0.15 cumulative offline baseline complete"
echo "  - all v0.1-v0.15 gates above: PASS"

cd "$ROOT"
echo "[48/52] v0.16 remaining-global-budget + strict pre-call unit gates"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^TestRemainingProviderBudgetDoesNotRegrantSpentMoney$' -count=1
GOTOOLCHAIN=local "$GO_BIN" test ./internal/agent -run '^(TestStrictPreCallBudget|TestProviderRequestTokenUpperBound|TestStrictBudgetBlocks)' -count=1

echo "[49/52] v0.16 hard budget survives model rebuild + skips cosmetic title provider"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^(TestHardPreCallBudgetSurvivesNativeModelSwitch|TestHardBudgetSkipsCosmeticTitleProvider)$' -count=1

echo "[50/52] v0.16 process hard-budget harness syntax"
bash -n ./scripts/balance_mod_precall_budget.sh

echo "[51/52] v0.16 real localhost pre-call budget cap (offline mock, no keys)"
PATH="$(dirname "$(command -v "$GO_BIN")"):$PATH" GOTOOLCHAIN=local ./scripts/balance_mod_precall_budget.sh

echo "[52/52] PASS — no provider API key/network was used"
echo "  - v0.1 KZT budget/resource telemetry: PASS"
echo "  - v0.2 native anti-loop/completion telemetry: PASS"
echo "  - v0.3 router/cache/log/patch modules: PASS"
echo "  - v0.4 unified repair cycle + native rollback adapter: PASS"
echo "  - v0.5 provider-agnostic execution router + native model switching: PASS"
echo "  - v0.6 universal APK agent control + workspace bridge: PASS"
echo "  - v0.7 capability/environment/project/live APK protocol: PASS"
echo "  - v0.8 APK control plane + restart-safe persistence + supervisor: PASS"
echo "  - v0.9 project registry + native task catalog: PASS"
echo "  - v0.10 native durable task queue + reviewed recovery + per-turn budgets: PASS"
echo "  - v0.11 unified power engine + native turn-evidence bridge: PASS"
echo "  - v0.12 idle-boundary auto continuation + durable inbox handoff: PASS"
echo "  - v0.13 durable pending route recovery + hard Pro diagnosis read-only gate: PASS"
echo "  - v0.14 APK v1 contract freeze + localhost offline prototype: PASS"
echo "  - v0.15 native deny bypass + crash/restart + queue/state stress gate: PASS"
echo "  - v0.16 hard pre-call provider cap + retry reserve: PASS"
echo "BALANCE_MOD_V16_SMOKE_PASS"
