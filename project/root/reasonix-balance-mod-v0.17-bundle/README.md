# Reasonix Balance Mod v0.17 — Crash / Replay Hardening

Required baseline: **v0.16 FULL PASS**.

## What v0.17 closes

The native inbox previously had this crash window:

`SnapshotActivity PASS -> crash -> AckDequeue never committed`

On restart the durable inbox could not distinguish that case from a turn whose result had not crossed the durable transcript boundary, so fail-closed recovery made it `uncertain`.

v0.17 adds a native durable completion receipt between those two operations:

`SnapshotActivity -> atomic active-set completion receipt -> AckDequeue`

- crash before receipt: `uncertain`, paused, explicit retry only;
- crash after receipt: recovery finalizes without replay;
- active-set receipt is all-or-nothing;
- client idempotency receipts are preserved when recovery finalizes a completed orphan;
- completed items cannot be requeued with `RetryItem`;
- schema bumps from 2 to 3; old binaries fail closed on the newer manifest.

This does **not** guarantee universal exactly-once behavior for arbitrary external side effects. Tool/provider-specific side effects still need their own transaction/idempotency semantics.

No DeepSeek/provider API key is read, written, or needed.

## Install in Debian

```bash
cd ~
cp /sdcard/Download/reasonix-balance-mod-v0.17-bundle.tar.gz .
tar -xzf reasonix-balance-mod-v0.17-bundle.tar.gz
bash reasonix-balance-mod-v0.17-bundle/apply_balance_mod_v0.17.sh ~/DeepSeek-Reasonix
```

The installer performs patch apply/reverse-apply checks, gofmt and `git diff --check`. It intentionally does not claim a Go build/test.

## Test order

Targeted first:

```bash
cd ~/DeepSeek-Reasonix
PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_v017_targeted.sh
```

Expected final marker:

`BALANCE_MOD_V17_TARGETED_PASS`

Then quick regression:

```bash
PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_quick_v017.sh
```

Expected:

`BALANCE_MOD_V17_QUICK_PASS`

Then full regression:

```bash
PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_v017.sh
```

Expected:

`BALANCE_MOD_V17_SMOKE_PASS`
