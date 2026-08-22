Balance Mod v0.14.1 hotfix

Fixes one test/contract-name mismatch in v0.14:
- implementation exports requestRules.mutatingRequestsContentType
- the new test accidentally looked for postRequestsContentType

No runtime policy, endpoint behavior, API key, provider, budget, or agent logic is changed.
