# Autonomous development flow

```text
public fork or repository branch
        |
        v
Pull Request -> next
        |
        v
trusted intake (never checks out contributor code)
        |
        v
trusted workflow_dispatch candidate gate
        |
        +--> preflight exact PR/base/head
        |
        +--> read-only candidate runner
        |      - current Reasonix: Go/backend/mobile/Swarm gates
        |      - future Reasonix: reasonix-evolution.json contract
        |
        v
fresh write-capable merge job
(no contributor checkout)
        |
        v
next
        |
        |  stability delay + repeated health checks
        v
trusted promotion validation
        |
        v
main stable snapshot
```

No human reviewer is required by this automation.

Unknown public contributors normally contribute through forks. People who already have repository write access may use branches in the original repository and open the same kind of pull request to `next`.

Community pull requests cannot automatically modify the trusted root automation. This is deliberate: allowing untrusted code to rewrite the mechanism that decides whether untrusted code is safe would make autonomous merging meaningless.
