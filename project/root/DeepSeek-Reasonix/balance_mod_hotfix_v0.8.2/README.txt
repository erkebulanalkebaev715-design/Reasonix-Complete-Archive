Balance Mod v0.8.2 hotfix

Fixes the v0.8.1 compile error:
  unknown field Persistence in struct literal of type modStatusPayload

Cause:
v0.8.1 wired s.modPersistenceStatus() into modStatus(), but the typed
modStatusPayload struct itself was not extended with the matching JSON field.

This hotfix adds only:
  Persistence map[string]any `json:"persistence"`

It does not change the smoke logic, routing, API keys, provider behavior, or budget semantics.
