# Balance Mod v0.20 Fixed6

Fixed6 hardens the first real DeepSeek gate after Fixed5 could remain inside provider streaming while the post-round ledger stayed at zero.

- strict hard-budget OpenAI/DeepSeek requests use one non-stream ChatCompletions HTTP request; ordinary Reasonix remains streaming;
- strict request has a 45 second provider deadline;
- hidden HTTP, body-stream and reasoning-replay retries remain disabled;
- DeepSeek V4 Flash thinking is disabled for the tiny gate;
- gate config disables tools/environment and uses a minimal system prompt;
- rate-card snapshot uses the current official flat USD rates captured 2026-08-18: cache hit 0.0028, cache miss input 0.14, output 0.28 per 1M tokens;
- provider-reported usage is reconciled against the hard KZT ledger.

The installer performs no provider/network call. The version is PASS only after the ARM64 real gate prints both final PASS markers.
