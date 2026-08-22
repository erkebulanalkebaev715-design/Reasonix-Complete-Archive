#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-go}"
cd "$ROOT"

echo "[1/11] v0.12 orchestrator policy + APK contract"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^TestModAuto' -count=1

echo "[2/11] v0.13 execution-mode guard rollback"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^TestExecutionRouterModeGuardRunsBeforeSwitchAndRollsBackOnFailure$' -count=1

echo "[3/11] v0.13 hard Pro diagnosis + durable pending route"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^(TestModProDiagnosisHardDeniesMutatingTools|TestModPendingPowerRoutePersistsSanitizedAcrossRestart)$' -count=1

echo "[4/11] v0.14 frozen APK contract"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^TestModAPKContract' -count=1

echo "[5/11] v0.15 native deny-bypass mock scenario"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/provider/mock -run '^TestDenyBypassScenarioRequiresSchemaTrimAndNativeDenial$' -count=1

echo "[6/11] v0.15 offline stress harness syntax"
bash -n ./scripts/balance_mod_offline_stress.sh

echo "[7/11] v0.16 remaining-budget conversion"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/efficiency -run '^TestRemainingProviderBudgetDoesNotRegrantSpentMoney$' -count=1

echo "[8/11] v0.16 strict pre-call + single-agent budget policy"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/agent -run '^(TestStrictPreCallBudget|TestProviderRequestTokenUpperBound|TestStrictBudgetBlocks)' -count=1

echo "[9/11] v0.16 model-switch/title hard-budget regression"
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^(TestHardPreCallBudgetSurvivesNativeModelSwitch|TestHardBudgetSkipsCosmeticTitleProvider)$' -count=1

echo "[10/11] v0.16 process hard-budget harness syntax + serve compile"
bash -n ./scripts/balance_mod_precall_budget.sh
GOTOOLCHAIN=local "$GO_BIN" test ./internal/serve -run '^$' -count=1

echo "[11/11] CLI compile gate"
mkdir -p bin
GOTOOLCHAIN=local CGO_ENABLED=0 "$GO_BIN" build -o bin/reasonix ./cmd/reasonix

echo "BALANCE_MOD_V16_QUICK_PASS"
