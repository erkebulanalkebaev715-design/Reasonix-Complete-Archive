# Balance Mod v0.19 — APK Backend Integration

Baseline: verified v0.18 Full Offline Prototype / Release Candidate.

v0.19 does **not** create a second agent/backend or an APK-specific copy of
Reasonix logic. It freezes and stress-checks the boundary an Android client will
use: the existing `reasonix serve`, `balance-apk-v1`, native Controller, native
permissions/tools/queue/recovery, typed HTTP/SSE events, and the existing Android
supervisor script.

## New v0.19 gate

The targeted harness starts a real ARM64-capable `reasonix serve` with the native
mock provider and **token authentication** on `127.0.0.1:0`. It requires:

- private `--token-file` and `--pid-file`/`--port-file` supervisor handoff;
- no provider API-key variables in the offline backend process;
- no serve token in the Reasonix process command line;
- unauthenticated `/mod/*` API access denied;
- `/auth/token` cookie bootstrap works for an app-style client;
- frozen `balance-apk-v1` contract is present with at least the v0.14 inventory
  (67 endpoints / 68 event types) and all required mobile surfaces;
- authenticated `/mod/events` remains typed SSE;
- authenticated hard KZT budget -> `/mod/app/task/start` -> native tool loop ->
  `/mod/live/history` exposes `OFFLINE_MOCK_PASS`;
- existing `scripts/reasonix_android_backend.sh` still exposes the agreed
  `start|stop|restart|status|token|log` supervisor surface;
- v0.18 full regression passes first in the full v0.19 smoke.

PASS does not mean an Android APK UI has been visually implemented or tested. It
means the backend boundary required by that APK is authenticated, machine-readable,
and regression-tested on the target machine. No real provider/API key is used.
