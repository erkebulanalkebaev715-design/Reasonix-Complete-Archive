Balance Mod v0.6.1 hotfix

Fixes smoke stage [12/20]. Registry.ProviderVisible() ignored the runtime-only
sessionHidden deny list whenever the baseline providerVisible allowlist was nil.
The provider schema path already used the correct combined predicate. This
hotfix makes ProviderVisible() use that same predicate so API reporting, tests,
and schema trimming agree.

No API key/network is used.
