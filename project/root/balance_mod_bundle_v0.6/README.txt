Balance Mod v0.6 — Universal Agent Control / APK foundation

Requires: v0.5 already applied and BALANCE_MOD_V05_SMOKE_PASS.
No DeepSeek/API key is required by this version or its smoke tests.

New backend surface:
  GET  /mod/agent
  POST /mod/agent/reload      same-model native controller rebuild
  GET  /mod/agent/tools
  POST /mod/agent/tools       per-tool allow|ask|deny + ask|auto|yolo
  GET  /mod/agent/skills
  POST /mod/agent/skills      native Reasonix skill enable/disable persistence
  GET  /mod/instructions
  POST /mod/instructions      recognized REASONIX.md/AGENTS.md/CLAUDE.md only
  GET  /mod/workspace
  POST /mod/workspace/validate
  GET  /mod/workspace/files
  GET  /mod/workspace/file

Design rules:
- same Reasonix controller/agent powers CLI and future APK;
- denied tools use the native permission gate and are removed from provider
  schema to save tokens; indirect use_capability calls cannot bypass the deny;
- tool overrides survive Flash/Pro controller rebuilds;
- instruction edits go through native MemoryControl rather than arbitrary writes;
- file browsing is read-only, symlink-aware, workspace-confined;
- changing the project root is an APK supervisor restart of reasonix serve, not
  process-wide chdir inside a live agent;
- no hidden model reasoning is exposed. APK will receive plans/actions/tool
  events/diffs/results through the existing event surfaces.

Expected final smoke line:
  BALANCE_MOD_V06_SMOKE_PASS
