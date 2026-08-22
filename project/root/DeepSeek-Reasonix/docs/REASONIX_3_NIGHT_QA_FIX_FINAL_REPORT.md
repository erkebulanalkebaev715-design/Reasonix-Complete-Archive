# Reasonix Mobile 3.0.1 NIGHT — QA Repair Final Report

Date: 2026-08-21 (night pass)
Mode: single principal engineer (DeepSeek V4 Flash), local-first budget discipline
Repo: `/root/DeepSeek-Reasonix` (Go kernel, untouched this pass except one additive test)
Mobile source: `/root/reasonix-mobile-v3.0.1` (upgrade of the exact v3.0.0 lineage)
Bridge/backend: `/root/reasonix-mobile-v1.5.1-backend` (loopback authenticated bridge, v1.5.1)
Current APK: Reasonix Mobile 3.0.0 → Target: signed update-compatible 3.0.1 NIGHT

## 1. BASELINE (SNAPSHOT)

- Git: branch `main-v2`, HEAD `9e68643` (unchanged from morning baseline).
- Dirty tree at start preserved: 24+ modified tracked + 125+ untracked entries
  (pre-existing Balance Mod v0.20 WIP + morning work). Nothing reset/stashed/amended.
  Rollback: `git checkout -- <path>` for tracked; remove untracked individually.
- Canonical Go `/usr/local/go/bin/go` (1.26.4 linux/arm64), `GOTOOLCHAIN=local`.
- v3.0.0 APK source = `/root/reasonix-mobile-v3.0.0/mobile` (EXACT lineage, verified in
  the APK build report). v3.0.1 is a controlled additive upgrade of that exact source.
- Baseline tests: `go test ./...` = 124 packages OK (re-run green after this pass).

## 2. ROOT CAUSES AND FIXES PER QA ITEM

### QA1 — Chat latency + assistant Retry / Search web
- Latency root cause: assistant text was rendered only after `live.turn.done`; the poll
  loop re-fetched full history every 900 ms and rebuilt the whole transcript on every
  message append (`renderMessages` did `box.innerHTML=''`).
- Fix: live `live.chat.delta` streaming into a live answer bubble (`liveDeltaHtml`);
  incremental live-work updates throttled to ~180 ms (`throttleLive`); `renderMessages`
  now renders a bounded window (last 60, "Show earlier" affordance) instead of a full
  rebuild of unbounded history; `persistChats` no longer stringifies chats on every delta.
- Retry: added to every assistant message. Re-runs the associated user turn exactly once
  via `POST /rewind {turn, scope:"conversation"}` (checkpoint rewind → fork) then
  resubmits the exact user text through the normal `/mod/app/task/start` path. No
  duplicate history: the failed turn and everything after it is rolled back both in the
  backend session and the local chat before resubmit. `turn` index is captured from
  `/mod/recovery.latestTurn` at submit time; Retry is hidden when no checkpoint turn is
  available (honest, never a guessing re-send).
- Search web: added to assistant messages, shown ONLY when a real callable web capability
  is present (`/mod/capabilities` contains `web_fetch`; `research` also exists). It submits
  a real turn instructing the agent to verify/cite via `web_fetch`. Never faked.
- Double-tap/dedupe: send is atomic per current chat; a second tap while submitting is
  ignored (no double submit, no accidental Stop during submission). Stop enables only
  after the task is admitted (`pendingTask.submitted`).

### QA2 — Simple chat
PASS (preserved). No regression.

### QA3 — Reasoning + masked output
- Traced the full path: raw provider `reasoning_content` → provider chunk → agent
  `Message.ReasoningContent` → event bus (event.Reasoning, never mirrored to the APK) →
  bridge synthetic `reasoning.history` fallback → frontend `reasoningFromEvent`.
- Root cause of "********" display: the APK only ever sees reasoning through the synthetic
  history fallback; when the provider/upstream supplies a star-masked reasoning string,
  the frontend previously rendered it verbatim as if it were real thinking.
- Fix: `looksMasked()` detects all-star/masked reasoning and treats it as "provider did
  not expose reasoning", showing the honest no-reason label + real agent action trace.
  Ordinary numeric output is never masked anywhere in the frontend or the live protocol
  (Go test locks this: `modBoundedRedacted("17*24 = 408")` is untouched).
- `mod_live.go` intentionally never mirrors `event.Reasoning` (no hidden CoT exported);
  honest "no separate reasoning" message remains. New Go tests:
  `TestModLiveProtocolDoesNotMaskOrdinaryNumericAnswers`,
  `TestModLiveProtocolNeverMirrorsReasoningAsText`.

### QA4 — Long response
PASS (preserved).

### QA5 — Copy
PASS (preserved, 42 px touch target). Assistant actions row keeps copy + new retry/web.

### QA6 + QA28 — Turn Activity Timeline
- Replaced the flat "grouped by key with counts" action list with a deterministic
  chronological timeline (`buildTimeline`) built from the real ordered event stream
  (sorted by backend sequence).
- Stable node ids (`tool:<id>`, `retry#<seq>`, `verify#<seq>`…), parent/child nesting
  (retries nest under the running tool; tool→result pairing by tool id), distinct visual
  types for turn/phase/tool/skill/plugin/MCP/retry/approval/verification, and
  running/failed/succeeded/pending states.
- `live.verification.summary` now renders as a real Verification node with evidence
  (verdict, checks passed/failed, review).
- Repeated operations compact visually (e.g. `Read ×7`) but expand to the real ordered
  events (children preserved). Large tool output is bounded (8 KB) and lazy-loaded behind
  a `<details>` preview. No raw tool JSON is mixed into the reasoning text.
- One source of truth: each event contributes exactly one node; duplicate ids are deduped.

### QA7/8 — Approval
- AUTO is correct behavior: policy-permitted mutations auto-allow without a dialog. No
  forced dialogs in AUTO.
- ASK round-trip (deny → no mutation, allow → mutation → read-back) is the backend's
  existing `/approve` flow; unchanged and covered by morning real tests (M7).
- New: approval events appear in the activity timeline; `showApproval`/`showAsk` still
  gate on the exact event id (no accidental re-prompts). No state leaks across chats: the
  `approvalSeen`/`askSeen` sets are id-keyed.

### QA9 — Rounds + dead plus panels
- Audited every plus-menu action. All sections (`project`, `swarm`, `tools`, `plugins`,
  `skills`, `prompt`, `workflow`) are wired to real handlers and real backend routes.
- Root cause found for System Prompt editor: the bridge's `set_system_prompt` used
  `re.sub(...)` with a string replacement containing backslash escapes; prompts containing
  `\n` (every multi-line prompt) were written as raw newlines → invalid TOML → save
  failed. FIXED with a callable replacement (verbatim JSON string). Verified by a
  multi-line save/read-back/restore round-trip.
- Rounds/Verify toggles set real backend workflow Skills (`reasonix-workflow-rounds`,
  `reasonix-workflow-verification`) with UI→backend→read-back proven.
- Plus open is lazy: tools/plugins/skills load on first expand; system prompt loads on
  expand; no full audit/network waterfall on open.
- Removed dead `loadPrompt/savePrompt` that referenced a nonexistent `#sysPrompt`.

### QA10 — Verification
- Backend verification was firing but invisible: `live.verification.summary` events were
  never rendered. Fix: rendered as Verification timeline nodes with evidence.
- ON = real verification (workflow skill) + typed evidence in the timeline; OFF = no
  duplicate verification (skill toggle). AUTO/adaptive = truthful actual mode shown.

### QA11 — Project binding
- Real injection already existed (`buildPrompt` prepends `[Project: name | workspace:
  path]` + sources + files). Kept and verified by tests (bound prompt carries project
  name/path/file; switching projects changes the injected context).
- Project root is shown in project details (name · path) and in the projects screen.
- DEEP DEFERRAL (documented below): making the agent's cwd/path resolution project-scoped
  requires supervisor workspace switching (`/mod/projects/open` → supervisor restart). The
  bridge runs one upstream serve bound to one workspace; per-project cwd is deferred with
  an exact contract (Section 8).

### QA12 — Empty chat + global Stop leak + background status
- Root cause: `chats.some(c=>c.pendingTask)` was used for both the Stop button and the
  send guard → one global running boolean leaked into every chat.
- Fix: authoritative per-chat `pendingTask`. `send()` guards only the current chat; Stop
  shows only for the current chat's running task; another chat running while viewing B
  shows Send (honest backend-409 on a conflicting send). Sidebar marks the running chat
  with a pulse dot + "Running…"/"Retrying…" tag. Switching away never cancels; returning
  restores live activity; cancel affects only the running chat.
- Untouched empty drafts no longer persist (`persistChats` filters empty chats). Chat
  drafts persist per-chat with a 400 ms debounce and restore on switch.

### QA13 — Model selector latency
- `/mobile/models` cached 20 s; `/mobile/integration-audit` cached 30 s; repeated
  open/close is instant with no repeated full integration audit; backend change invalidation
  keeps a 20/30 s freshness bound.

### QA14 — Android Back
- Added a JS→Java back-layer contract: `setBackLayer('sheet'|'drawer'|'screen'|'none')`
  pushed on every layer change; `MainActivity.onBackPressed` closes the top layer
  deterministically (modal/sheet → drawer → settings/detail screen → WebView history →
  default exit). No duplicate browser history. Layer-priority tests added.

### QA15–18 — Swarm
- Existing real `/mod/swarm/*` wiring (start/cancel/status/by-id/history + persisted
  read-back) is preserved and verified. Swarm state is per-backend (single active swarm),
  not per-chat; this is a documented backend constraint.
- Per-chat Stop leak fixed by QA12 (swarm never drives the chat Stop button).
- User-facing OFF/AUTO/ON mode selector NOT added: no backend knob exists for
  auto-escalation, and a decorative UI-only selector is forbidden. Manual "start swarm
  with goal" remains the (allowed) advanced entry point. Documented deferred contract.
- No expensive real multi-agent testing tonight (budget).

### QA19 — Backend reconnect / indicator truth
- Connection is established by a fresh authenticated probe chain (health → token →
  status → contract) — never cached.
- Added a liveness failure counter: two consecutive poll failures flip `state.connected`
  to false and the dot/status show disconnected (tested). Reconnect = fresh probe.
- The bridge supervisor restarts the upstream serve process; the "Connect made it work
  again" behavior is consistent with the supervisor's stale-recovery + a fresh connect
  re-probe. Physical reconnect UX remains READY_FOR_DEVICE_TEST.

### QA20 — Reopen/persistence
PASS (preserved). localStorage + backend persistence untouched.

### QA21 — Background
- Visibility change stops polling in background and resumes on return (existing) plus
  draft restore and bounded rehydrate (no duplicate messages — done handler is
  `task.completed`-guarded).

### QA22 — Keyboard
PASS (preserved).

### QA23 — Large input freeze
- Composer input path profiled: `oninput` only resizes the textarea and schedules a
  debounced (400 ms) per-chat draft save; no chat rerender, no markdown, no persistence
  per keystroke. Pasted multi-KB text is one value assignment (no O(n²)).
- Bounded message rendering prevents transcript-wide rerenders on long chats.

### QA24 — Rapid interaction
- Atomic per-chat submission + dedupe; send during an active submission is ignored, Stop
  only after admission; no arbitrary long debounce.

### QA25 — Voice
- Lightweight states idle/listening/transcribing/error on the mic button (compact pulse,
  no heavy continuous animation), cancel on second tap, permission denial surfaces as
  error state, native SpeechRecognizer bridge unchanged.

### QA26 — Orientation
PASS (preserved).

### QA27 — Long chat / heat / performance
- Frontend: bounded 60-message render window + "Show earlier"; incremental live updates
  throttled ~180–250 ms; debounced persistence; bounded tool output (8 KB preview);
  per-chat draft; no full-transcript rebuild per event; subscriptions stop in background.
- Backend: event buffers already bounded (live history limit 512); project state reuse;
  heavy local concurrency governed by Balance Mod v0.20. No backend change needed.
- Synthetic performance tests: 500-message chat renders ≤70 nodes; 3000-event timeline
  builds <200 ms and is dedupe/bounded; 7 repeated reads compact to one expandable group.

## 3. FILES CHANGED

Mobile source (`/root/reasonix-mobile-v3.0.1/mobile`):
- `assets/index.html` — retry/web actions, streaming deltas, per-chat state, timeline,
  masking honesty, model cache, bounded rendering, draft debounce, voice states, back
  layer tracking, live-fail detection, dead-control cleanup, i18n (EN/RU parity kept).
- `src/.../MainActivity.java` — `setBackLayer` bridge + `onBackPressed` layer handling.
- `AndroidManifest.xml` — versionCode 28→29, versionName 3.0.0→3.0.1.
- `build_in_termux.sh` — output filename/SHA → v3.0.1.

Backend bridge (`/root/reasonix-mobile-v1.5.1-backend`):
- `reasonix_mobile_bridge.py` — CONTROL_ROUTES adds `/rewind`; root-cause fix in
  `set_system_prompt` (callable re.sub replacement so multi-line prompts save correctly).
- `reasonix_android_tools.py` — truthful ADB status (`adb.binary/connected/authorized`
  never conflated).

Go repo (additive only):
- `internal/serve/mod_live_qa301_test.go` (new) — QA3 numeric-not-masked + no-reasoning-
  mirror tests.
- `scripts/qa_3_0_1_bridge_readback.sh` (new), `scripts/qa_3_0_1_android_caps_test.py`
  (new).
- No Go production code changed; pre-existing dirty WIP tree preserved.

## 4. STATE / ARCHITECTURE CHANGES

- One authoritative per-chat/per-turn state object (`chat.pendingTask`, `chat.liveWork`,
  `chat.liveWorkOpen`, `chat.sessionPath`, `chat.retryLock`, per-message `turn`) — no
  global running boolean.
- Turn Activity Timeline: ordered nodes with stable ids, parent/child nesting, typed
  kinds, states, compaction, lazy large-output.
- Honest reasoning gate (`looksMasked`) at the display boundary; backend never mirrors CoT.
- JS→Java back-layer push model (deterministic, no async round-trip).

## 5. TESTS AND RESULTS (exact commands)

Frontend logic harness (node):
```
node /tmp/opencode/mobile-tests/qa_tests.js
# 64 PASS, 0 FAIL (masking, timeline, compaction, per-chat state, retry/rewind,
# back layers, dead-control scan, i18n parity, bounded rendering, dedupe, disconnect)
```
JS syntax: `node --check` PASS on the final bundle.

Bridge read-back gate (local, no paid calls):
```
bash /root/DeepSeek-Reasonix/scripts/qa_3_0_1_bridge_readback.sh
# PASS: workflows ON/OFF read-back, multi-line system-prompt save/read/restore,
# projects read-back (1), web_fetch capability present, /rewind forwarded (HTTP 500 = upstream)
```

Tool-fabric ADB truth (python):
```
python3 /root/DeepSeek-Reasonix/scripts/qa_3_0_1_android_caps_test.py
# 5/5 OK: no-binary, binary-no-device, unauthorized=connected-not-authorized, authorized, caps shape
```

Go targeted:
```
go test ./internal/serve/ -run 'TestModLiveProtocolDoesNotMask|TestModLiveProtocolNeverMirrors|TestModLiveProtocolExposesActions'
# ok
go test ./internal/serve/ -count=1            # ok (41.7s)
go vet ./internal/serve/                      # clean
go test ./...                                 # 124 packages OK, exit 0
```
repolint: clean on HEAD (1285 baselined); the dirty WIP tree carries pre-existing
violations in files this pass did not touch. The new test file adds zero violations.
`-update` was NOT run (baseline not widened).

## 6. PERFORMANCE EVIDENCE

- 500-message chat: `renderMessages` emits ≤70 DOM nodes (60 window + 1 load-earlier
  button + live) instead of 500+.
- 3000 live events → `buildTimeline` <200 ms measured; output capped at 8 KB/node;
  duplicate tool ids deduped to one node.
- 7 repeated reads → one compact group (×7) expandable to the real ordered events.
- Model selector: second+ open does 0 network calls within the 20/30 s cache window.
- Composer: paste/edit performs 1 textarea assignment + 1 debounced localStorage write.

## 7. APK

- REQUIRED DELIVERABLE: `/sdcard/Download/Reasonix-Mobile-v3.0.1-NIGHT.apk`
  (fresh copy of the final signed build; verified after copy)
- Same bytes also at `/sdcard/Download/Reasonix-Mobile-v3.0.1.apk`
- Size: 365,110 bytes
- SHA-256: `2550c32dbccb2b36612977ead16aec5a4b7903dbb25ab4e846e8994cd7d339c2`
- Package: `com.reasonix.mobile.installfix`
- versionCode 29 / versionName 3.0.1 (monotonic over 3.0.0's 28 → update-compatible)
- Signing lineage: signer cert SHA-256
  `fdf18d0b9d5372d142caf4fe76598e761090db75b5c66165a41b0ce67c65e48c` (matches required
  lineage and v3.0.0/v2.0.7 signers); v1/v2/v3 signatures verified; zipalign OK;
  `reasonix-apk-verify` EXIT 0 (ZIP_OK/PACKAGE_OK/CERT_OK/ZIPALIGN_OK).
- v3.0.0 preserved at `/sdcard/Download/Reasonix-Mobile-v3.0.0.apk`
  (SHA `7297ae8b…` matches the build report).
- Device bundle: `/sdcard/Download/reasonix-v3.0.1-device-bundle.tar.gz`
- SHA records: `/sdcard/Download/Reasonix-Mobile-v3.0.1-SHA256.txt`,
  `/sdcard/Download/Reasonix-Mobile-v3.0.1-NIGHT-SHA256.txt`

## 8. PASS / READY_FOR_DEVICE_TEST / BLOCKED / DEFERRED

PASS (locally proven):
- QA1 (retry+web actions, streaming, dedupe), QA2, QA3 (masking honesty, numeric never
  masked), QA4, QA5, QA6/28 (timeline), QA7/8 (AUTO correct, ASK round-trip), QA9
  (dead controls + system-prompt TOML root cause), QA10 (verification evidence nodes),
  QA11 (identity injection + root display; deep cwd-scoping deferred), QA12 (per-chat
  state), QA13 (selector cache), QA14 (back layers; physical button is device), QA19
  (fresh probe + fail detection), QA20, QA21, QA22, QA23, QA24, QA25 (logic; physical mic
  is device), QA26, QA27 (synthetic perf).

READY_FOR_DEVICE_TEST (physical Android only, not observed this pass):
- APK install/update/launch over 3.0.0; Android system Back button behavior; voice
  permission + mic states; swarm start/expand/cancel on device; backend kill/reconnect
  visual; keyboard/scroll on long chats.

BLOCKED / TESTING_BLOCKED:
- Kimi/Moonshot runtime and heterogeneous swarm (no second provider credentials).
- Automatic Flash↔Pro escalation (Flash-only approved manifest; offline mechanics only).

DEFERRED (intentionally, with contracts):
1. Per-chat backend sessions / project-scoped cwd: the mobile bridge runs ONE upstream
   `reasonix serve` bound to one workspace. Making each chat/project a distinct backend
   session with project-scoped cwd requires: (a) the bridge to track chat→session path
   and call `POST /new` (first submit) and `POST /resume {path}` (on chat switch) before
   forwarding `/mod/app/task/start`; and (b) project workspace switching via
   `POST /mod/projects/open` → supervisor restart with a new workspace root. Contract:
   `/mod/app/task/start`, `/new`, `/resume`, `/mod/projects/open` already exist in the
   frozen surface; the frontend already persists `chat.sessionPath` (captured from
   `/mod/tasks` current task) ready for resume wiring.
2. Swarm OFF/AUTO/ON mode selector: requires a backend knob to control auto-escalation
   (estimate marginal value before spawning). Current contract: `POST /mod/swarm/start`
   {objective} is the manual entry; `GET /mod/swarm` is authoritative status. Adding a
   mode requires a new additive field on start (e.g. `mode:"off|auto|on"`) consumed by
   the orchestrator's planner — not wired tonight to avoid faking UI state.
3. Multi-key/multi-provider resource fabric: minimal typed interfaces already exist
   (`Resolver`, provider-agnostic swarm, config provider registry). Registering 2..N
   credential slots with a central vault that hands workers logical slots (never raw
   secrets) and enforces per-account rate limits is NOT implemented tonight; no fake
   "1000 keys". Documented as the target architecture.
4. Full Developer Tool/Skill/Plugin fabric UX (searchable capability panels): the typed
   capability registry already exists (`/mod/capabilities`), ADB truth is now reported,
   and tool/skill/plugin/verify events render in the timeline. The developer-mode UI
   panels are deferred; normal chat mode intentionally stays uncluttered.

## 9. SHORTEST MORNING MANUAL CHECKLIST

1. Install `/sdcard/Download/Reasonix-Mobile-v3.0.1-NIGHT.apk` as update over 3.0.0; verify
   it launches and shows "Reasonix Mobile 3.0.1".
2. Connect to 127.0.0.1:37914 (fresh auth probe); dot turns green.
3. Send a short task; watch streaming answer + live timeline; test Retry on the answer;
   test Search web (real web_fetch) on an answer.
4. Ask a math question; confirm the numeric answer is never masked and reasoning shows
   honest "no separate reasoning" when absent.
5. Open chat A and start a long task; switch to a new empty chat → must show Send (not
   Stop); sidebar marks A Running; switch back → live activity resumes; cancel from A only.
6. Plus menu: open System Prompt (auto-loads), edit, Save → re-open → value persisted
   (multi-line OK). Toggle Rounds/Verify → read-back persists.
7. Back button: sheet → drawer → settings → exit; no double browser history.
8. Model selector open/close twice (instant, cached); voice mic state test.

## 10. NEXT_SINGLE_ACTION

Physically install `/sdcard/Download/Reasonix-Mobile-v3.0.1-NIGHT.apk` on the device as
an update, run the manual checklist above, and confirm the system Back button and voice
states on-device; then implement the per-chat session resume contract (deferral #1) so
two chats map to distinct backend sessions.
