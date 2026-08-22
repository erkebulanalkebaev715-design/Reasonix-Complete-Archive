# Balance Mod v0.17 — Crash / replay hardening

Baseline: verified v0.16 Hard Pre-Call Budget System.

v0.17 closes one specific native session-inbox crash window:

1. Controller finishes the turn.
2. `SnapshotActivity()` durably commits transcript/activity state.
3. v0.17 atomically writes a completion receipt for the entire active inbox set.
4. Only then does normal `AckDequeue` remove each inbox item.

Recovery semantics:

- crash before the completion receipt -> in-flight work becomes `uncertain`, inbox pauses, explicit retry is required;
- crash after the completion receipt but before dequeue -> recovery finalizes the item without replaying the turn;
- a completion receipt is independent of the optional client idempotency key;
- existing client idempotency aliases are converted to normal acknowledged receipts when a completed orphan is finalized;
- a completed item cannot be requeued through `RetryItem` even if an acknowledgement failure temporarily made it `uncertain`.

This is deliberately **not** a claim of universal exactly-once execution for arbitrary external side effects. External tools/providers need their own transactional or idempotency guarantees for that. v0.17 hardens Reasonix's durable inbox-turn replay boundary only.

On-disk inbox schema is bumped from 2 to 3. Existing schema-2 manifests migrate forward. Older binaries continue to fail closed on an unknown newer schema.

No provider/API key is needed or used by the v0.17 tests.
