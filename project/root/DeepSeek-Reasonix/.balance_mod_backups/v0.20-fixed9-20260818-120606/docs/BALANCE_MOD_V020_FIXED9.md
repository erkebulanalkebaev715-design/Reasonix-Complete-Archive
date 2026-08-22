# Balance Mod v0.20 Fixed9

Consolidated real-provider repair for the confirmed Fixed4 failure. Replaces the obsolete 66-way retry-reserve split with one paid attempt per hard-budget admission, disables hidden provider/body/reasoning retries under strict hard-budget mode, updates DeepSeek V4 Flash pricing to the official 2026-08-18 rate card, and minimizes only the isolated real-gate prompt surface. Production Reasonix streaming is otherwise unchanged.


Fixed9 additionally consolidates generated strict-budget regression tests: prior
versioned balance-mod tests are backed up and removed before package compilation
to prevent duplicate Go test declarations. Rollback restores them on failure.


## Fixed9 installer validation policy

Fixed9 keeps executable targeted regression tests for the strict budget/retry changes,
then compiles all tests in internal/provider, internal/agent and internal/serve with
`-run '^$'`. It deliberately does not execute the entire internal/agent runtime suite
during installation because that suite includes session lease/write-authority stress
probes whose behavior depends on the Android/PRoot filesystem/process environment.
The complete CLI is still built after the compile-only gate.
