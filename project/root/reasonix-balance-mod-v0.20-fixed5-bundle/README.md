# Reasonix Balance Mod v0.20 Fixed5

Fixed5 replaces Fixed4 after the first real provider submission was blocked by the old v0.16 66-way retry reservation. It keeps the v0.20 architecture and changes strict hard-budget execution to one paid provider attempt at a time: hidden provider retries and body/reasoning replays are disabled while strict hard budget is active, so every later paid attempt must return through host admission and the durable ledger.

Installer dynamically patches the actual installed v0.20 tree, performs exact apply/reverse/diff checks, runs targeted local Go tests, and never performs an API call. The final online smoke still performs one DeepSeek Flash task.
