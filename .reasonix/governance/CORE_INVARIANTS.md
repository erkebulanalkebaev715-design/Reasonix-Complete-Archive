# Reasonix Autonomous Governance — Core Invariants

This directory is the trusted control plane for unattended community development.

The automation protects behavior and repository integrity, not the historical 2026 implementation.

## Permanent repository invariants

1. `main` is the stable canonical snapshot branch.
2. `next` is the automatically integrated development branch.
3. `original-v3.0.1` remains a historical recovery point.
4. Untrusted contributor code is never executed in a job that has repository write permission.
5. Contributor code receives no repository secrets.
6. Root `.github/`, `automation/`, `.reasonix/governance/`, and `.gitmodules` are governance-controlled and are not auto-merged from public pull requests.
7. A pull request is validated against the current `next` merge result, not merely against an isolated fork head.
8. A pull request is merged only if its head SHA and base SHA are unchanged after validation.
9. Candidate validation is serialized to avoid accepting two independently-green changes whose combination was never tested.
10. `main` promotion is performed only by a trusted workflow after a stability delay and a fresh full validation of the exact `next` SHA.
11. A failed newly-promoted `main` may be automatically reverted once. Historical bad states remain recoverable through Git history/tags.
12. Architecture changes are allowed. Current file names, languages, process boundaries, ports, frameworks and internal packages are not permanent invariants.

## Behavioral evolution rule

The current Reasonix v3 architecture is validated directly while it exists.

A future architecture may replace it by supplying `reasonix-evolution.json`. The trusted validator requires a declared build/test contract and runs those commands in an unprivileged, read-only-token CI job.

For an architecture that claims compatibility with `balance-apk-v1`, the manifest must also provide a black-box contract probe. A protocol-major migration may intentionally replace that protocol when it includes a migration document and explicit major-evolution declaration.

The purpose is to prevent old implementation-specific checks from treating a legitimate redesign as a regression while still refusing an empty or untestable replacement.

## Trust boundary

A passing test is evidence, not mathematical proof that a contribution is harmless. The system therefore layers:

- immutable automation;
- exact-SHA validation;
- serialized integration;
- no secrets in candidate execution;
- `next` quarantine;
- delayed `main` promotion;
- repeated scheduled health validation;
- automatic single-step recovery.

This control plane itself is intentionally harder to change than Reasonix application architecture.
