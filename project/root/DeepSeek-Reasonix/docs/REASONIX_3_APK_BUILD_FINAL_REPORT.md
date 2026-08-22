# Reasonix Mobile 3.0 — APK Build / Integration / Verify — Final Report

Date: 2026-08-21
Mode: single principal engineer (DeepSeek V4 Flash), budget-constrained local build pass
Executed against the EXISTING `/root/DeepSeek-Reasonix` lineage. No second agent engine was
created inside the APK. The APK remains a thin mobile control plane over the real Reasonix runtime.

## SOURCE_LINEAGE
EXACT v2.0.7-RC mobile source found and reused. The real source bundle
`/sdcard/Download/reasonix-v2.0.7-rc-device-bundle.tar.gz` contains:
- `mobile/AndroidManifest.xml`
- `mobile/src/com/reasonix/mobile/installfix/MainActivity.java` (ordinary Activity + WebView + native voice bridge)
- `mobile/assets/index.html` (single-file ChatGPT-like frontend)
- `mobile/res/drawable/reasonix_icon.png`
- `mobile/build_in_termux.sh` (proven manual aapt2-era pipeline: ecj → d8 → aapt → zipalign → apksigner)
- `mobile/reasonix-signing.p12` (original signing keystore)
- `mobile/signing-cert.pem`
- `backend/` (reasonix_mobile_bridge.py, reasonix_mobile_backend.sh — v1.5.x loopback bridge)

Candidate classification:
- `/sdcard/Download/reasonix-v2.0.7-rc-device-bundle.tar.gz` → EXACT_SOURCE_LINEAGE
- `/root/reasonix-mobile-v1.0..v1.5.1-backend` → LIKELY_PREDECESSOR (bridge/backend only, not APK UI source)
- `/sdcard/Download/reasonix_mobile_v0_6_phone_only*` → LIKELY_PREDECESSOR (older Python phone prototype)
- other `.apk`/`.zip` starters → UNRELATED/NOT_VERIFIED

## SOURCE_CLASSIFICATION
EXACT_SOURCE_LINEAGE. No reconstruction was required. The v3.0.0 APK is a controlled,
additive upgrade of the exact v2.0.7 source with the Reasonix 3.0 Swarm UX integrated and the
version bumped. It is NOT the original untouched source (deliberately modified for 3.0), but it is
built directly from the real lineage with no guessing.

## V2_0_7_APK_VERIFIED
TRUE. `reasonix-apk-verify /sdcard/Download/Reasonix-Mobile-v2.0.7-RC.apk com.reasonix.mobile.installfix fdf18d0b9d5372d142caf4fe76598e761090db75b5c66165a41b0ce67c65e48c`
→ ZIP_OK, PACKAGE_OK, CERT_OK, ZIPALIGN_OK, v1/v2/v3 signatures true, EXIT 0.
- SHA-256 `4f6dae9ba7d2fd01b36c084459f0bbaa82379ef8611441dc5e7beb5000b1cb08`
- package `com.reasonix.mobile.installfix`, versionCode 27, versionName 2.0.7, MainActivity launchable.
Artifact evidence matches the handoff. No package/signing contradiction → no lineage migration needed.

## PACKAGE_LINEAGE
VERIFIED: `com.reasonix.mobile.installfix` preserved. v3.0.0 package is identical, so
v3.0.0 installs as an UPDATE over any installed v2.x of the same cert lineage.

## SIGNING_LINEAGE
SIGNING_LINEAGE_VERIFIED. The original keystore `reasonix-signing.p12` (PKCS12, alias `reasonix`)
is present in the source bundle. Keytool export SHA-256
`fdf18d0b9d5372d142caf4fe76598e761090db75b5c66165a41b0ce67c65e48c` matches:
- the handoff expected cert,
- the verified v2.0.7-RC APK signer cert,
- the newly built v3.0.0 APK signer cert.
No private key/password values are printed in this report (they remain in the local p12 / build
script used on-device only).

## TOOLCHAIN
`reasonix-toolchain-check` PASS on this host (R8_JAR missing but not needed by the proven path):
- JAVA/JAVAC/KEYTOOL OK, AAPT2/AAPT OK, APKSIGNER OK, ZIPALIGN OK, D8 OK (termux), GO OK, ADB OK (optional).
- Node.js 20 was installed during this run to enable a definitive JS syntax check of the WebView
  frontend (`node --check` PASS on final bundle; the original v2.0.7 bundle also PASS).
Toolchain notes / architecture compatibility:
- `/usr/bin/aapt` (Debian v0.2-debian) could NOT resolve `android:*` framework attributes; the
  termux `aapt` (Android 16.0.0_r4, ARM64) + the real SDK framework
  `platforms/android-36/android.jar` (18 MB, has `resources.arsc`) was required.
- Android SDK build-tools/35.0.0 `zipalign` is x86-64 → architecture-incompatible on this ARM64
  device and was NOT used. The Debian ARM64 `/usr/bin/zipalign` (same binary `reasonix-apk-verify`
  uses) was used instead. `apksigner` is a Java wrapper (architecture-independent).

## BUILD_SYSTEM
REUSED the exact proven lineage path (`build_in_termux.sh`): ecj → d8 (min-api 23) → aapt
package (framework `-I`) → dex zip → zipalign 4 → apksigner (original p12). No new Android build
system was introduced. Output filename changed to `Reasonix-Mobile-v3.0.0.apk`.

## FILES_CHANGED
Mobile source (under `/root/reasonix-mobile-v3.0.0/mobile`, outside the Go repo; no Go changed):
- `AndroidManifest.xml` — versionCode 27→28, versionName 2.0.7→3.0.0 (minSdk 23, targetSdk 35 kept).
- `assets/index.html` — added the real Reasonix 3.0 Swarm UX (start/current/completed/history/cancel,
  collapsed-by-default expandable card, live drawer indicator, swarm poll), RU/EN strings (+40 keys
  per language, 225/225 parity), version label → "Reasonix Mobile 3.0.0", backend target note.
- `build_in_termux.sh` — output + SHA filenames → v3.0.0.
- `src/.../MainActivity.java` — unchanged (voice already native).
No changes to `/root/DeepSeek-Reasonix` backend code or the mobile bridge (additive-compatible).

## MOBILE_BACKEND_CONTRACT
Resolved live from current source and a real local run (bridge `reasonix-mobile-v1.5.1` on
127.0.0.1:37914 → upstream `reasonix serve` balance-mod-v0.20):
- health/status: `/mobile/health`, `/mod/status` (modVersion balance-mod-v0.20, modelRef correct).
- auth: `X-Reasonix-Mobile-Token` + `POST /auth/token` (204 in the app flow).
- task submit: `POST /mod/app/task/start` `{"input":…}` (canonical field `input`, route = `s.submit`).
- approval: `POST /approve` (`{id, allow, session, persist}`), `POST /tool-approval-mode`.
- plan: `POST /plan`; new/resume/fork/compact/summarize surfaced through Serve's session routes
  (mobile UI exposes new chat + project binding; fork/compact remain backend capabilities).
- projects: `/mobile/projects` GET/POST, `/mobile/projects/delete`.
- tools: `/mobile/tools` GET/POST/delete/toggle (`.servers` shape).
- Skills: `/mobile/skills` GET/POST/delete/toggle/detail (`.skills` shape).
- MCP/plugins: same `/mobile/tools` list filtered non-stdio.
- model/provider selector: `/mobile/models`, `/mobile/model`, `/mobile/provider`.
- integration audit: `/mobile/integration-audit` (ok=true, coreGuard.evidenceAware=true).
- events: `/mod/live/history` (200); Reasoning via `reasoning_content` transport.
- budget/progress: `/mod/status` budget + `live.*` events.
- Swarm (3.0): `POST /mod/swarm/start` `{"objective":…}` (202), `POST /mod/swarm/cancel`, `GET /mod/swarm`,
  `GET /mod/swarm/{id}`, `GET /mod/swarm/history` — all reachable through the bridge's `/mod/*` forward.
Frozen APK contract unchanged: `balance-apk-v1` revision 1, 72 endpoints / 85 events
(verified live via `/mod/app/contract`).

## UI_IMPLEMENTATION
Kept v2.0.7 structure: minimal dark chat, composer `+ | message | mic | send`, sidebar
New Chat / Search / Projects / history, expandable Thinking box, Codex-like Activity cards from real
events, compact Allow/Deny approvals with backend round-trip, model selector with real readiness,
tiny green/busy backend dot, plus-menu (Projects / Tools / Plugins / Skills / System prompt /
Rounds / Verify / AI breaker). Added the Swarm section as a first-class plus-menu card.

## THINKING
Real provider-visible reasoning only: the reason box renders `reasoning_content`/`<thinking>` from
live events or, when the provider exposes none, an explicit "no separate reasoning was exposed"
message + real agent action trace. Activity is never faked as model reasoning.

## ACTIVITY
Codex-like real timeline: read/edit/tool/search/command/test/approval cards grouped from real
`live.*` events; repeated identical actions grouped with a count; sources chips from real URLs.

## APPROVAL
Real backend round-trip: `approval_request` event → sheet (subject/diff preview) →
`POST /approve` allow(once/chat/always)/deny → continuation. Mutation only after approval; deny is
not treated as failure. No repeated mutation before approval.

## PROJECTS
Real backend-persisted projects via `/mobile/projects` (id/name/workspace/sources/files/chatIds),
create/edit/delete/bind, project→chat association, sources/files attach.

## TOOLS
Real registry state from `/mobile/tools` (`.servers`), stdio vs remote filtered honestly, real
add/edit/delete/toggle that mutates `.mcp.json` and reloads Reasonix.

## SKILLS
Real `/mobile/skills` (`.skills`) list/detail/toggle/add/edit/delete; built-in pack install; Rounds
and Verify map to real backend-managed workflow Skills (`/mobile/workflows`), not prompt injection.

## MCP
Real installed/configured state via `/mobile/tools` (remote = MCP/plugins). No fake catalog state.

## SWARM_UI
NEW in 3.0. Real first-class but minimal-by-default:
- Collapsed card shows objective + profile chips with live marks (`Architect ✓`, `Coder …`, `Verify …`).
- Expandable detail shows swarm ID, status, verified flag, budget (requests/tokens/cost per provider),
  findings, result, and per-task rows (id, profile, status, provider/model, worker id, attempts, scope,
  evidence, failure).
- Real controls: start (`POST /mod/swarm/start`), cancel (`POST /mod/swarm/cancel`, shown only while
  running/planned), view current/last (`GET /mod/swarm`), view completed by id (`GET /mod/swarm/{id}`),
  history (`GET /mod/swarm/history`).
- Completed swarms remain readable after completion and after refresh/reopen (backend persists under
  `config.SwarmStateDir()`; the live probe read back a `done`/verified swarm with 2 tasks).
- Drawer activity shows a minimal `◈ objective · Swarm running` indicator while active.
- No fake badge: every control maps to a real `/mod/swarm/*` route.
Validated by node-based smoke test against a realistic backend SwarmState JSON (running + completed).

## VOICE
Real native Android SpeechRecognizer bridge (MainActivity `ReasonixNative.startVoice/cancelVoice/
voiceAvailable`, onStart/onPartial/onFinal/onError/onCancel, ~35s watchdog, RECORD_AUDIO permission +
runtime request, no Web Speech fake). Unchanged from v2.0.7. Physical microphone behavior remains
READY_FOR_DEVICE_TEST (not physically observed this pass).

## DEAD_UI_AUDIT
Static scan on the final frontend bundle: TODO=0, FIXME=0, `href="#"`=0, `window.prompt`=0,
`alert(`=0, `confirm(`=0. Web Speech fallback references=0; native `ReasonixNative` bridge ref=1.
Task-start protocol scan: `{"input":…}` path=1, no `prompt`-field regression. Every new swarm control
has a real handler → real `/mod/swarm/*` route → real state → read-back render (smoke-tested).

## BACKEND_INTEGRATION
Proven live on 127.0.0.1 without ADB and without paid inference:
- bridge `/mobile/health` 200; `/auth/token` 204 (app flow); `/mod/status` 200 (balance-mod-v0.20).
- `/mod/app/contract` 200 = balance-apk-v1, 72 endpoints / 85 events (frozen, unchanged).
- `/mod/live/history` 200; `/mobile/integration-audit` ok=true.
- swarm read-only: `/mod/swarm` 200 (done/verified, persisted), `/mod/swarm/history` 200,
  `/mod/swarm/{id}` read-back 200.
Mobile-only changes cannot affect Go/provider behavior; no Go tests were rerun (morning suite
124/124 already green; no backend code changed this pass). No paid provider call was repeated.

## APK_BUILD
Built locally at `/root/reasonix-mobile-v3.0.0/mobile/build/Reasonix-Mobile-v3.0.0.apk` using the
proven ecj→d8→aapt→zipalign→apksigner path with the original p12 keystore. Build produced
`VOICE_BUILD_READY`-equivalent outputs; the copy of the build script is on-device-compatible.

## APK_VERIFY
`reasonix-apk-verify /sdcard/Download/Reasonix-Mobile-v3.0.0.apk com.reasonix.mobile.installfix fdf18d0b9d5372d142caf4fe76598e761090db75b5c66165a41b0ce67c65e48c` → EXIT 0:
ZIP_OK, PACKAGE_OK, CERT_OK, ZIPALIGN_OK, v1/v2/v3 signatures valid, AndroidManifest/classes.dex/
resources.arsc PRESENT, cert SHA-256 lineage match.

## PACKAGE
`com.reasonix.mobile.installfix`

## VERSION_CODE
28 (monotonic over v2.0.7's 27 → update-compatible install)

## VERSION_NAME
3.0.0

## CERT_SHA256
`fdf18d0b9d5372d142caf4fe76598e761090db75b5c66165a41b0ce67c65e48c` (original lineage)

## APK_SHA256
`7297ae8b2861a7560b3d43aac5ac0a807fdd6d6800645e2aa740b796e89a09cc`

## DOWNLOADS_PATH
`/sdcard/Download/Reasonix-Mobile-v3.0.0.apk` (SHA record:
`/sdcard/Download/Reasonix-Mobile-v3.0.0-SHA256.txt`)
Release source bundle: `/sdcard/Download/reasonix-v3.0.0-device-bundle.tar.gz`

## PASS
- v2.0.7-RC APK lineage verified (package, cert, SHA match handoff).
- Source lineage: EXACT v2.0.7 source found; no reconstruction.
- Signing lineage verified with the original p12 keystore.
- Reasonix 3.0 Swarm UX integrated with real `/mod/swarm/*` routes; smoke-tested (running + completed + read-back).
- Frozen APK contract unchanged (72 endpoints / 85 events).
- APK built, locally verified (all gates), copied to Downloads; old APKs preserved.

## NOT_VERIFIED
- Physical APK install/update/launch on a device (no ADB/UI automation this pass).
- Physical voice behavior (SpeechRecognizer + RECORD_AUDIO) on a device.
- Physical swarm UX interactions on a device.
- Automatic Flash↔Pro escalation and Kimi/heterogeneous swarm remain NOT_VERIFIED/TESTING_BLOCKED
  (unchanged from the morning report; no second provider available).

## TESTING_BLOCKED
- Kimi/Moonshot runtime and heterogeneous swarm (no second provider credentials).

## READY_FOR_DEVICE_TEST
- `/sdcard/Download/Reasonix-Mobile-v3.0.0.apk` install/update/launch.
- Backend indicator, real chat, Thinking expand/collapse, Activity timeline, tool read,
  approval deny/allow, project create/open, swarm start/expand/cancel/completed read-back,
  close/reopen persistence, voice permission + mic, back/keyboard/scroll,
  backend disconnect/reconnect/error state.

## ROLLBACK
- v2.0.7-RC preserved: `/sdcard/Download/Reasonix-Mobile-v2.0.7-RC.apk`
  (SHA `4f6dae9b…b1cb08`). Revert = install v2.0.7-RC over v3.0.0 (same cert lineage; update-compatible).
- Prior mobile bundles (v1.0..v1.5.1 backend) and all old APKs in `/sdcard/Download` untouched.
- Go repo unchanged this pass (no `git` mutations); morning rollback notes still apply there.

## NEXT_SINGLE_ACTION
Physically install `/sdcard/Download/Reasonix-Mobile-v3.0.0.apk` as an update over the current
v2.x app, launch it, and run the manual checklist (install→voice→swarm) from section 17 of the
master prompt; verify backend bridge on 127.0.0.1:37914 is the v1.5.x loopback supervisor.

---

FEATURE | TEST | RESULT | EVIDENCE
--- | --- | --- | ---
v2.0.7-RC lineage | `reasonix-apk-verify` | PASS | package/cert/SHA match handoff
Signing lineage | keytool + apksigner cert compare | PASS | p12 cert == fdf18d0b… == v2.0.7 == v3.0.0
Toolchain | `reasonix-toolchain-check` | PASS | all required tools present (R8 n/a)
Source classification | bundle audit | EXACT_SOURCE_LINEAGE | v2.0.7 device bundle
Frontend JS syntax | `node --check` (node 20) | PASS | final bundle + original both PASS
RU/EN key parity | script | PASS | 225/225, no unknown I() keys
Swarm UI logic | node smoke test | PASS | running card, detail, start/stop enablement, completed read-back, history
Dead-UI scan | static grep | PASS | TODO=0 FIXME=0 href#=0 alert=0 prompt=0 confirm=0
Backend contract (live) | loopback bridge probe | PASS | contract 72/85; auth 204; status 200; audit ok
Swarm read-back (live) | `GET /mod/swarm`, `{id}`, history | PASS | done/verified swarm with 2 tasks
APK build | ecj→d8→aapt→zipalign→apksigner | PASS | signed with original keystore
APK verify | `reasonix-apk-verify` (Downloads copy) | PASS | EXIT 0, all gates green
Copy to Downloads | `reasonix-apk-copy` | PASS | `/sdcard/Download/Reasonix-Mobile-v3.0.0.apk`
Device bundle | tar.gz release artifact | PASS | `/sdcard/Download/reasonix-v3.0.0-device-bundle.tar.gz`
Physical install/launch/UI/voice/swarm on device | manual | READY_FOR_DEVICE_TEST | not physically observed
