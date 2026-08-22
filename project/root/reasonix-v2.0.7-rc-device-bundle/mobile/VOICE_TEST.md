# Voice acceptance gate

This is NOT PASS until run on the physical Android device.

1. Install/update the built APK.
2. Open Reasonix Mobile and connect backend.
3. Tap microphone.
4. If Android asks for microphone permission, allow it.
5. Speak a short Russian phrase.
6. Expected: listening state appears, partial/final recognition returns, final text is inserted into composer and is NOT auto-sent.
7. Tap mic during active recognition: expected cancel, no permanent listening state.
8. Deny microphone permission: expected a clear permission error, no crash.
9. Re-enable permission and repeat: expected recognition can start again.

Only after these physical checks may voice be marked PASS.
10. Start recognition and remain silent / make the recognizer fail to terminate naturally: expected terminal error within the bounded session (native watchdog ~35s), composer restored, mic usable again.
11. Trigger recognizer-unavailable / no-match / busy where possible: expected visible error and no crash or endless listening.
