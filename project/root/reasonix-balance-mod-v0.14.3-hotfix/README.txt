Balance Mod v0.14.3 hotfix

Fixes only the v0.14 process-level offline prototype assertion.
GET /mod/budget returns:
  {"budget": {"budgetKzt": ..., "spentKzt": ...}, "taskCostGate": {...}}
The prototype incorrectly read spentKzt/budgetKzt from the top-level object.
Runtime/backend logic is unchanged.
