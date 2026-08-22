# Reasonix — Complete Frozen Project

Frozen: 2026-08-22

This repository is a complete historical snapshot of the Reasonix project.

## Included

- DeepSeek-Reasonix core
- Reasonix Controller / Agent
- Reasonix Serve
- Balance Mod development history
- historical Balance Mod patches / hotfixes / bundles
- mobile backend generations
- stable bridge backend
- Reasonix Android mobile projects
- Android build/tooling
- v2.0.7 / v3.0.0 / v3.0.1 generations
- night-build automation
- prompts and project handoff material
- runtime/project state retained on the device
- historical logs
- source archives and old bundles
- APK releases
- QA reports
- SHA records
- Git history bundles
- Git branch/tag/commit histories
- project timeline reconstructed from file mtimes

## Current frozen release

Reasonix Mobile 3.0.1 NIGHT

Package:
com.reasonix.mobile.installfix

Version:
3.0.1 / versionCode 29

Final APK:
Reasonix-Mobile-v3.0.1-NIGHT.apk

Final validated APK SHA-256:
2550c32dbccb2b36612977ead16aec5a4b7903dbb25ab4e846e8994cd7d339c2

Night result:
NIGHT V2 FINISH rc=0

## Architecture

Android APK / WebView UI
→ stable bridge 127.0.0.1:37914
→ Reasonix Mobile backend
→ authenticated Reasonix Serve
→ Reasonix Controller / Agent
→ Balance Mod
→ Tools / Skills / Swarm / Provider layer

## Archive layout

project/
  Preserved project files and historical device material.

git-history/
  Git bundles plus branch/tag/commit history.

meta/
  Original paths, project tree, timeline, size map and current state.

