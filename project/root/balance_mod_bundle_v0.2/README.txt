Balance Mod v0.2 — offline reliability layer

Requires: Balance Mod v0.1 + v0.1.1 hotfix already applied.
No DeepSeek API key is used.

Adds:
- APK-facing quality state: GET /mod/quality and quality in GET /mod/status
- Typed SSE signals: loop.detected, progress.stalled, verifier.evidence_required,
  verifier.passed, verifier.failed, completion.blocked, completion.allowed,
  completion.summary, phase.changed, task.failed
- Reuses Reasonix native ProgressTracker / loop guard / final-readiness gate
  instead of duplicating them
- Repeat-failure MockProvider scenario now proves the native loop guard can
  redirect the provider-visible history
- Expanded 7-stage offline smoke test, including the native final-readiness gate

Run:
  bash balance_mod_bundle_v0.2/apply_balance_mod_v0.2.sh ~/DeepSeek-Reasonix
  cd ~/DeepSeek-Reasonix
  PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh

Expected final line:
  BALANCE_MOD_V02_SMOKE_PASS
