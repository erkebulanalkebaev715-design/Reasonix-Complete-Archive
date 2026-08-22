#!/usr/bin/env python3
from __future__ import annotations
import argparse, json, tempfile
from pathlib import Path


def seq(e):
    try:
        return int(e.get("sequence", -1))
    except Exception:
        return -1


def classify(payload):
    events = payload.get("events", []) if isinstance(payload, dict) else []
    events = [e for e in events if isinstance(e, dict)]
    starts = [e for e in events if str(e.get("type", "")).lower() == "live.turn.started"]
    if not starts:
        return "WAIT", "no-turn-start"
    start_seq = seq(max(starts, key=seq))
    window = [e for e in events if seq(e) >= start_seq]
    messages, dones = [], []
    for e in window:
        typ = str(e.get("type", "")).lower()
        data = e.get("data") if isinstance(e.get("data"), dict) else {}
        err = str(data.get("error", "")).strip()
        if data.get("cancelled") is True or data.get("canceled") is True:
            return "ERROR", "turn cancelled"
        if err:
            return "ERROR", err.replace("\n", " ")[:700]
        if typ == "live.chat.message":
            text = str(data.get("text", "")).strip()
            if text:
                messages.append((seq(e), text))
        elif typ == "live.turn.done":
            dones.append(e)
    if not dones:
        return "WAIT", "turn-running"
    done_seq = seq(max(dones, key=seq))
    valid = [m for m in messages if m[0] <= done_seq]
    if not valid:
        return "ERROR", "clean turn.done without non-empty live.chat.message"
    text = valid[-1][1]
    masked = text == "****" or bool(text) and set(text) <= {"*"}
    return "DONE", "masked" if masked else "visible"


def load(path):
    try:
        return json.loads(Path(path).read_text(encoding="utf-8"))
    except Exception:
        return None


def self_test():
    base = [
        {"type":"live.turn.started","sequence":2,"data":{}},
        {"type":"live.phase","sequence":5,"data":{"phase":"working"}},
    ]
    masked = {"events": base + [
        {"type":"live.chat.message","sequence":14,"data":{"text":"****"}},
        {"type":"live.turn.done","sequence":16,"data":{"cancelled":False,"outcome":""}},
    ]}
    visible = {"events": base + [
        {"type":"live.chat.message","sequence":14,"data":{"text":"BALANCE_V20_REAL_PROVIDER_OK"}},
        {"type":"live.turn.done","sequence":16,"data":{"cancelled":False}},
    ]}
    failed = {"events": base + [
        {"type":"live.turn.done","sequence":16,"data":{"cancelled":False,"error":"provider failed"}},
    ]}
    running = {"events": base}
    assert classify(masked) == ("DONE", "masked")
    assert classify(visible) == ("DONE", "visible")
    assert classify(failed) == ("ERROR", "provider failed")
    assert classify(running) == ("WAIT", "turn-running")
    print("V020_COMPLETION_CHECK_SELFTEST_PASS")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("path", nargs="?")
    ap.add_argument("--self-test", action="store_true")
    a = ap.parse_args()
    if a.self_test:
        self_test(); return
    if not a.path:
        raise SystemExit(2)
    payload = load(a.path)
    if payload is None:
        return
    state, detail = classify(payload)
    print(f"{state}\t{detail}")

if __name__ == "__main__":
    main()
