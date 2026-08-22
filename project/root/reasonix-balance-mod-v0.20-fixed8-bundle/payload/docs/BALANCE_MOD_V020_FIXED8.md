# Balance Mod v0.20 Fixed8

Consolidated real-provider repair for the confirmed Fixed4 failure. Replaces the obsolete 66-way retry-reserve split with one paid attempt per hard-budget admission, disables hidden provider/body/reasoning retries under strict hard-budget mode, updates DeepSeek V4 Flash pricing to the official 2026-08-18 rate card, and minimizes only the isolated real-gate prompt surface. Production Reasonix streaming is otherwise unchanged.


Fixed8 additionally consolidates generated strict-budget regression tests: prior
versioned balance-mod tests are backed up and removed before package compilation
to prevent duplicate Go test declarations. Rollback restores them on failure.
