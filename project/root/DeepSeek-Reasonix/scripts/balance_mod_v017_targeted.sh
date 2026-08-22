#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

: "${GOTOOLCHAIN:=local}"
export GOTOOLCHAIN

echo "[v0.17 1/2] durable completion receipt + crash recovery"
go test ./internal/sessioninbox -run '^TestV017' -count=1

echo "[v0.17 2/2] controller snapshot -> receipt -> ack ordering"
go test ./internal/control -run '^TestV017ControllerCommitsCompletionBeforeAck$' -count=1

echo "BALANCE_MOD_V17_TARGETED_PASS"
