# Reasonix

**Reasonix** is an Android-first AI orchestration project preserved together with its development history and opened for long-term community evolution.

This repository has two purposes:

1. preserve the complete historical Reasonix project and its original working state;
2. allow the project to continue evolving through community contributions, forks, pull requests, architectural rewrites, testing, and future automation.

---

## Current reference release

**Reasonix Mobile 3.0.1 NIGHT**

- Android package: `com.reasonix.mobile.installfix`
- Version: `3.0.1`
- versionCode: `29`
- Final APK: `Reasonix-Mobile-v3.0.1-NIGHT.apk`
- Final validated APK SHA-256: `2550c32dbccb2b36612977ead16aec5a4b7903dbb25ab4e846e8994cd7d339c2`
- Frozen historical reference tag: `original-v3.0.1`

The `original-v3.0.1` tag is the permanent reference point for the project state that existed when this repository was opened for public development.

It is a historical baseline, not a requirement that future versions keep the same implementation.

---

## Historical architecture

At the frozen `v3.0.1` reference point, the main flow was:

```text
Android APK / WebView UI
        ↓
stable bridge 127.0.0.1:37914
        ↓
Reasonix Mobile backend
        ↓
authenticated Reasonix Serve
        ↓
Reasonix Controller / Agent
        ↓
Balance Mod
        ↓
Tools / Skills / Swarm / Provider layer
```

This architecture documents the original implementation.

Future contributors are explicitly allowed to replace, merge, simplify, split, rewrite, or remove historical components if the resulting system is demonstrably better and preserves or improves the intended behavior of the project.

A future Reasonix does **not** need to keep:

- the same programming languages;
- the same internal process layout;
- the same ports;
- the same filenames;
- the same backend generation;
- the same bridge implementation;
- the same provider implementation;
- the same Android UI architecture;
- the same controller structure;
- the same Balance Mod implementation.

Compatibility should be judged primarily by useful behavior, reliability, migration quality, and measurable project capabilities rather than by resemblance to old internals.

---

## Repository branches

### `main`

The stable canonical Reasonix branch.

This is the branch that should represent the best known integrated version of the project.

### `next`

The development and integration branch.

New work can be integrated here before promotion to `main`.

### `original-v3.0.1`

A permanent historical tag pointing to the original frozen Reasonix state.

It exists so the original project can always be inspected or recovered even after years of community development.

---

## How to contribute

There are two valid contribution models.

### Fork-based development

Anyone may fork this repository, experiment independently, create branches in their fork, and open a pull request back to this repository.

Typical flow:

```text
Fork
  ↓
feature branch
  ↓
development
  ↓
tests
  ↓
Pull Request
  ↓
Reasonix repository
```

Forks are welcome even when they explore a radically different architecture.

### Direct repository branches

Contributors who have repository write access may create branches directly inside this repository and open pull requests from those branches.

Typical flow:

```text
Reasonix branch
  ↓
development
  ↓
tests
  ↓
Pull Request
  ↓
integration
```

The canonical project remains this repository even when development begins in external forks.

---

## What contributors may change

Almost every technical part of Reasonix may evolve.

Contributors may improve or replace:

- Android application architecture;
- WebView or native UI;
- backend architecture;
- bridge and transport layers;
- controller and agent runtime;
- model/provider routing;
- orchestration;
- tool execution;
- skills;
- plugins;
- memory;
- context management;
- project/session handling;
- concurrency;
- Swarm systems;
- budgeting;
- performance;
- reliability;
- offline behavior;
- networking;
- tests;
- build systems;
- packaging;
- developer tooling;
- security architecture;
- documentation;
- release process.

Large rewrites are allowed.

A rewrite should not be rejected simply because it removes historical components.

---

## Project evolution rule

Reasonix should protect **capabilities and behavioral invariants**, not obsolete implementation details.

Bad invariant:

```text
A specific historical bridge file must always exist.
```

Better invariant:

```text
The Android client must be able to submit work and receive the correct result through the current supported runtime path.
```

Bad invariant:

```text
Port 37914 must exist forever.
```

Better invariant:

```text
The supported client/backend connection must be deterministic, testable, reconnectable, and correctly authenticated where authentication is required.
```

Bad invariant:

```text
The old backend architecture must remain unchanged.
```

Better invariant:

```text
A replacement backend must preserve or improve the user-visible behavior it replaces, or provide a documented migration when compatibility is intentionally changed.
```

This distinction is important. Automated protection must not accidentally freeze Reasonix in its 2026 architecture.

---

## Architectural rewrites

A pull request may replace a major subsystem or the entire architecture.

A legitimate architecture migration should provide enough evidence to establish that the new implementation is an improvement or intentional evolution.

Useful evidence may include:

- successful builds;
- black-box behavior tests;
- integration tests;
- regression tests;
- benchmarks;
- reliability measurements;
- migration documentation;
- compatibility results;
- removed limitations;
- newly supported capabilities;
- reduced complexity;
- improved performance;
- improved maintainability.

A rewrite does not need to preserve obsolete internal structures solely to satisfy old tests.

When an old test describes an implementation detail rather than an intended behavior, the test itself may be updated or replaced together with the architecture change.

---

## Historical validation record

The archived `Reasonix Mobile 3.0.1 NIGHT` release recorded:

- mandatory APK/report validation passed;
- `NIGHT V2 FINISH rc=0 initial_rc=0`;
- frontend validation: `64 PASS / 0 FAIL`;
- Android device tests: `5/5`;
- stable bridge readback gate passed;
- Go full test suite reported `124 packages OK`, exit `0`.

These results describe the historical reference build only.

They must not be interpreted as proof that every archived feature was fully completed or that future implementations must use the same architecture.

---

## Known historical nuance

Some historical work was intentionally deferred or only partially proven.

For example, a route may have passed transport/forwarding validation without proving every higher-level semantic path end-to-end.

Future contributors should distinguish:

- proven behavior;
- partially proven behavior;
- historical intent;
- unfinished functionality;
- future proposals.

Do not convert an old claim into a permanent project invariant unless the repository evidence supports it.

---

## Autonomous development direction

The long-term goal is for Reasonix to be able to continue evolving even when the original author is not actively supervising the repository.

The repository is intended to progressively support automated:

- pull-request validation;
- build checks;
- regression detection;
- behavioral invariant tests;
- architecture-aware validation;
- untrusted-contributor isolation;
- integration testing;
- dependency maintenance;
- merge gating;
- promotion from `next` to `main`;
- rollback of verified critical regressions;
- release generation;
- project health checks.

This document describes the intended governance direction. Automation should only be considered active when the corresponding workflows and repository rules actually exist and pass.

---

## Autonomous gate design principles

Future automation should follow these rules.

### 1. Do not trust contributor code by default

Code from forks and untrusted pull requests should run without privileged repository write access and without unnecessary secrets.

### 2. Separate testing from privileged actions

A workflow that executes contributor code should not also have broad authority to modify protected branches.

Privileged promotion or merge automation should consume trusted validation results rather than blindly executing untrusted code with write permissions.

### 3. Validate behavior, not filenames

Tests should verify project capabilities through supported interfaces whenever practical.

### 4. Allow tests to evolve

If a legitimate architecture change invalidates an implementation-specific historical test, the contributor may replace that test with a stronger behavioral test.

### 5. Protect the gate itself

Changes to the repository's core governance, CI, release, or protection logic should receive stricter validation than ordinary application changes.

### 6. Prefer reversible integration

Large changes should be easy to identify, isolate, revert, or bisect.

### 7. Keep historical recovery points

Stable releases and important architectural milestones should remain recoverable through tags or releases.

---

## Stable behavior vs historical implementation

When reviewing a change, ask:

> Does the change preserve or improve what Reasonix is supposed to accomplish?

Do **not** ask only:

> Does the new code look like the old code?

Examples of things that may legitimately change:

```text
WebView → native UI
Python service → Go/Rust/Kotlin service
local HTTP bridge → another transport
single provider → multi-provider fabric
single agent → multi-agent runtime
legacy session model → new persistent project runtime
old test harness → architecture-neutral integration harness
```

These changes may be valid even if most historical files disappear.

---

## Regression policy

A regression is a loss of intended supported behavior, reliability, correctness, or project capability that is not an intentional documented compatibility change.

The following alone are **not** regressions:

- deleting obsolete files;
- renaming components;
- changing programming language;
- replacing a framework;
- replacing a port;
- replacing internal APIs;
- moving directories;
- consolidating services;
- changing architecture;
- deleting tests that are replaced by stronger equivalent tests.

A compatibility-breaking change may be accepted when it is intentional, documented, tested, and justified by the new architecture.

---

## Pull request expectations

A useful pull request should make its intent understandable.

For ordinary changes, contributors should preferably include:

- what changed;
- why it changed;
- what behavior is affected;
- how it was tested.

For major architectural changes, contributors should preferably include:

- what old subsystem is being replaced;
- what new subsystem replaces it;
- what remains compatible;
- what intentionally changes;
- migration implications;
- validation evidence;
- new or updated tests.

The project should favor demonstrated improvements over preservation of obsolete implementation details.

---

## Security and reliability changes

Security and reliability improvements are welcome, but they should not become permanent architecture locks.

A security control should protect a property such as:

- authorization;
- integrity;
- isolation;
- safe execution;
- correct boundaries;
- recovery;
- reproducibility.

It should not unnecessarily require one historical implementation forever.

If a new architecture provides a stronger mechanism for the same property, the project should be able to migrate to it.

---

## Tests

The long-term testing strategy should include multiple layers where practical:

```text
unit tests
    ↓
component tests
    ↓
contract tests
    ↓
integration tests
    ↓
black-box behavior tests
    ↓
build/package validation
    ↓
regression checks
```

No single test layer should define the entire architecture.

Black-box and behavioral tests are especially valuable for allowing internal implementations to evolve safely.

---

## Compatibility

Compatibility is valuable but not absolute.

Changes should preserve compatibility when doing so is reasonable.

When compatibility prevents a substantially better architecture, a deliberate migration is allowed if it is:

- documented;
- testable;
- recoverable where practical;
- explicit rather than accidental.

---

## Archive contents

This repository preserves a broad historical snapshot of Reasonix development material, including:

- DeepSeek-Reasonix core;
- Reasonix Controller / Agent;
- Reasonix Serve;
- Balance Mod history;
- Balance Mod patches, hotfixes, and bundles;
- historical mobile backend generations;
- stable bridge backend;
- Android mobile projects;
- Android build/tooling material;
- v2.0.7 / v3.0.0 / v3.0.1 generations;
- night-build automation;
- prompts;
- project handoff material;
- retained runtime/project state;
- historical logs;
- source archives;
- old bundles;
- APK releases;
- QA reports;
- checksum records;
- Git history bundles;
- branch/tag/commit histories;
- reconstructed project timeline.

---

## Repository layout

### `project/`

Preserved project files and historical device material.

### `git-history/`

Git bundles and preserved branch/tag/commit history.

### `meta/`

Archive metadata such as original paths, project tree, timeline, size information, checksums, and frozen-state records.

As Reasonix evolves, new canonical source layout may be introduced. The historical archive does not force future code to remain inside the same directory structure.

---

## Releases and checkpoints

Important stable states should be tagged or released.

Historical tags must remain available even when the architecture changes completely.

This allows future users and contributors to answer:

- what did Reasonix originally look like?
- when was a subsystem replaced?
- which release introduced an architectural migration?
- when did a regression appear?
- which version is known to build?

---

## Forks and independent evolution

Forks are part of the project's ecosystem.

Developers are encouraged to experiment with ideas that may be too large or disruptive for immediate integration.

Successful fork innovations can return through pull requests.

A fork may also continue independently.

The existence of forks does not change which repository is canonical: accepted changes become part of this repository when merged here.

---

## Long-term project philosophy

Reasonix should remain open to becoming something significantly better than its original implementation.

The repository should preserve history without turning history into a cage.

Future contributors should be able to:

- repair the existing system;
- simplify it;
- optimize it;
- replace weak subsystems;
- redesign architecture;
- introduce new model providers;
- introduce new agent systems;
- improve Android integration;
- improve autonomy;
- improve testing;
- remove obsolete code;
- create entirely new generations of Reasonix.

The standard should be evidence of correctness and improvement, not loyalty to the old implementation.

---

## Historical recovery

To inspect the original frozen state:

```bash
git checkout original-v3.0.1
```

To return to active stable development:

```bash
git checkout main
```

To inspect the integration branch:

```bash
git checkout next
```

---

## Project status

The repository began as a complete frozen archive of Reasonix and is being prepared for community-driven evolution.

The historical source and artifacts are preserved.

The governance and automation layer should evolve independently from the historical implementation and must remain capable of accepting legitimate future architectures.

---

## Final principle

> Preserve the history. Protect the behavior. Allow the architecture to evolve.

Reasonix should be difficult to accidentally break, but it must never become impossible to improve.
