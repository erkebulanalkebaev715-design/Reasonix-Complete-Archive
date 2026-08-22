Balance Mod v0.4.1 hotfix

Root cause of v0.4 smoke failure:
Reasonix's existing csrfGuard requires application/json on every state-changing POST.
The new v0.4 tests for POST /mod/cycle/reset and POST /mod/recovery/rollback-last forgot that header, so the guard correctly returned HTTP 415 before the handlers ran.

This hotfix DOES NOT weaken or bypass csrfGuard.
It fixes the tests/contract to send application/json, matching the existing Reasonix frontend and future APK client.

Expected behavior after hotfix:
- /mod/cycle/reset -> 200 in the test
- /mod/recovery/rollback-last with no checkpoint -> reaches handler and returns 409 fail-closed
