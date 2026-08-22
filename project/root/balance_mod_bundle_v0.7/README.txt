Balance Mod v0.7 — Universal APK protocol layer

Incremental baseline:
  Balance Mod v0.6 + v0.6.1 provider-visibility hotfix, with V06 smoke PASS.

What v0.7 adds:
  - Dynamic Capability Registry projected from Reasonix's native tool registry.
  - Native JSON schemas exposed to the APK for dynamic tool UI.
  - Tool Packs: basic/files/verify/vcs/shell/developer.
  - Chat mode and Agent mode on the SAME controller/session/model.
  - Provider-agnostic Project Profile (name, mode, packs, live detail).
  - APK environment endpoint that REUSES upstream internal/environment probes,
    cache and persisted snapshots; no duplicate environment engine.
  - Cheap project markers for Go/Gradle/Android/Node/Python/Rust/Git.
  - Typed live.* event protocol and bounded history for chat/actions/diffs/results.
  - Project-detail previews are bounded and credential-redacted.
  - metadata live mode omits project payloads.
  - Hidden model reasoning is never exported to APK live events.

Offline rule:
  This release needs no DeepSeek API key and the smoke suite must not use one.

Install inside Debian:
  cd ~
  cp /sdcard/Download/reasonix-balance-mod-v0.7-bundle.tar.gz .
  tar -xzf reasonix-balance-mod-v0.7-bundle.tar.gz
  bash balance_mod_bundle_v0.7/apply_balance_mod_v0.7.sh ~/DeepSeek-Reasonix

Test:
  cd ~/DeepSeek-Reasonix
  PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local ./scripts/balance_mod_smoke.sh

Expected final marker:
  BALANCE_MOD_V07_SMOKE_PASS
