# Reasonix Balance Mod v0.18 Fixed

This is the same v0.18 Full Offline Prototype / Release Candidate, rebased for the
actually-passed v0.17 tree. It does not introduce a new version or architecture.

The original v0.18 installer used a static zero-context hunk for
`internal/serve/mod_bridge.go`. That was too brittle against the real v0.17 working
tree and correctly failed instead of using fuzzy patching.

The fixed installer:

1. verifies the passed v0.17 crash/receipt markers;
2. refuses conflicting pre-existing v0.18 files;
3. generates a unified patch against the exact local `mod_bridge.go` plus missing RC files;
4. runs `git apply --check` before applying;
5. runs shell/config validation and `git diff --check`;
6. runs `git apply --reverse --check` after applying;
7. never uses a provider, network API, or API key.

It still performs no Go build or test. PASS must come from the ARM64 test sequence.

Install in Debian:

```bash
cd ~
cp /sdcard/Download/reasonix-balance-mod-v0.18-fixed-bundle.tar.gz .
tar -xzf reasonix-balance-mod-v0.18-fixed-bundle.tar.gz
bash reasonix-balance-mod-v0.18-fixed-bundle/apply_balance_mod_v0.18_fixed.sh ~/DeepSeek-Reasonix
```

Then run targeted, quick, and full in that order.
