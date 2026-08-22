# Reasonix Balance Mod v0.19 bundle

APK Backend Integration over the verified v0.18 offline RC.

This bundle is intentionally small: it bumps the Balance Mod marker to v0.19 and
adds an authenticated Android/APK backend integration gate. It does not duplicate
Reasonix Controller/agent/tool/queue/recovery logic and it does not use a real
provider or API key.

Apply:

    bash apply_balance_mod_v0.19.sh ~/DeepSeek-Reasonix

Then targeted -> quick -> full. Do not call v0.19 PASS until the full smoke prints
`BALANCE_MOD_V19_SMOKE_PASS` on the target ARM64 Debian environment.
