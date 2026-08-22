# Reasonix Android Build Skill

Use for all future Reasonix Mobile APK work.

- Continue the existing Reasonix project; never create a random replacement app.
- Locate and reuse the real mobile lineage that produced the latest known-good APK.
- APK is UI/control-plane only; Reasonix remains the runtime.
- Preserve package/update/signing lineage unless evidence proves a migration is necessary.
- Preserve ordinary Android Activity + WebView; never regress to NativeActivity or handwritten DEX hacks.
- Stable localhost bridge/backend contracts must be read from current source, not guessed.
- Thinking != Activity. Approvals/tools/Skills/MCP/projects/Swarm must map to real backend primitives.
- No dead/decorative controls and no fake success.
- Build PASS != install PASS != launch PASS != feature PASS.
- Physical behavior remains READY_FOR_DEVICE_TEST until manually observed.
- For every defect: SYMPTOM -> REPRODUCE -> ROOT CAUSE -> INVARIANT -> FIX -> REGRESSION TEST -> RELEASE GATE.

Persistent commands:
- reasonix-toolchain-check
- reasonix-apk-verify APK [package] [cert_sha256]
- reasonix-apk-copy APK [name.apk]
- reasonix-backend-probe

Visual contract: minimal black/dark-gray/gray/white UI, tiny green status only; no neon or decorative gradients; composer `+ | message | mic | send`; sidebar New Chat/Search/Projects/history; Swarm collapsed by default and expandable.
