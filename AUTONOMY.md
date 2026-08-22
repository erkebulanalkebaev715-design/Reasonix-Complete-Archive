# Reasonix autonomous community development

Reasonix uses `next` as an automatically integrated community branch and `main` as the delayed stable snapshot branch.

Public contributors can fork the repository and open pull requests to `next`. Contributors who already have write access can use branches in the original repository and open the same pull requests.

The trusted root workflows validate the exact PR merge result without giving contributor code write credentials or repository secrets. Passing contributions are merged into `next`. After a three-day stability window, the exact `next` tree is fully revalidated and may be promoted automatically to `main`.

A newly promoted `main` that later fails the full scheduled gate may be automatically reverted once.

Architecture is allowed to evolve. The validator directly understands the historical v3 tree while it exists; a future complete redesign uses `reasonix-evolution.json` instead of being rejected solely because old Go/mobile/backend paths disappeared.

See `.reasonix/governance/CORE_INVARIANTS.md` for the trust boundary and permanent automation invariants.
