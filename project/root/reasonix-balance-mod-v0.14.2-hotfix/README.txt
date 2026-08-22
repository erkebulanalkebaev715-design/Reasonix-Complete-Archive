Balance Mod v0.14.2 hotfix

Fixes only scripts/balance_mod_smoke.sh.
After step 37 the script is still inside mktemp workspace. Step 38 executes `go test ./internal/serve`, so Go cannot find go.mod. The fix changes cwd back to $ROOT before step 38.
No Reasonix runtime, API contract, agent, budget, provider, or APK logic is modified.
