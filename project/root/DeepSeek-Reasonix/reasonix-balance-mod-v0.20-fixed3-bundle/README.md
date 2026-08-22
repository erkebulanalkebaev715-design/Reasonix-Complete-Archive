# Reasonix Balance Mod v0.20 Fixed3 bundle

Use this bundle on the existing v0.19/v0.20 tree after the v0.20 offline targeted/quick/preflight gates have passed.

The installer dynamically constructs an exact patch against the current tree and performs:
- `git apply --check`
- exact apply (no fuzzy/force)
- shell/Python syntax checks
- `git diff --check`
- `git apply --reverse --check`

It does not read an API key and does not call a provider.

Final online test:
`PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke_v020.sh`

Expected success:
`BALANCE_MOD_V20_REAL_GATE_PASS`
then
`BALANCE_MOD_V20_SMOKE_PASS`
