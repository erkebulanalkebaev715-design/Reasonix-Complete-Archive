# Reasonix Balance Mod v0.20 Fixed10

Harness-only hotfix over the validated Fixed9 hard-budget core.

The previous real gate incorrectly required the exact provider marker to appear in `/mod/live/history`. A successful real DeepSeek turn can be represented there with masked message text (for example `"****"`) even though `live.turn.done` is clean and the KZT ledger contains positive provider spend. Fixed10 accepts the current turn only when all three objective signals are present: a non-empty `live.chat.message`, a clean `live.turn.done`, and positive ledger spend. The exact marker is still searched in live/SSE telemetry but is informational rather than mandatory.

Fixed10 also detects generic terminal `data.error`/cancellation and preserves a sanitized failure snapshot under `.balance_mod_diagnostics/v020-fixed10-last`. It does not copy `.env`, cookies, auth token files, or the private curl config.

The installer performs no DeepSeek model call and does not change the Fixed9 production Go core.

Local validation before packaging:
- harness shell syntax: PASS
- reconciler self-test: PASS
- completion checker self-test: PASS
- exact user live-event fixture (`chat.message text="****"` + clean `turn.done`): `DONE masked`
- install over Fixed9-like tree: PASS
- second idempotent install: PASS
- no DeepSeek/provider call made by installer validation
