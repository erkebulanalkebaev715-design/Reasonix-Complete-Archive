Reasonix Balance Mod v0.20 Fixed8

Install directly over an existing balance-mod-v0.20 tree (including Fixed4 or Fixed5). The installer makes no provider/API call. It patches exact local copies, runs syntax checks, targeted tests, package tests and an ARM-compatible CLI build, and rolls back automatically if local validation fails.


## Fixed8 installer hardening

Fixed8 fixes the local compile failure where an older generated
`balance_strict_retry_fixed5_test.go` remained beside the new generated test
and both declared `TestBalanceStrictRetryLimitZeroStartsOneHTTPAttempt`. The
installer now backs up and removes only stale `balance_strict_retry_fixed*_test.go`
files from `internal/agent` and `internal/provider`, keeps only Fixed8 tests, and
restores the old files automatically if local validation fails. Production code
and unrelated tests are not matched by this cleanup. The CLI binary is replaced
atomically only after all local Go tests and the build succeed.
