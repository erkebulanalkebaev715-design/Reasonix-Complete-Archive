#!/usr/bin/env python3
"""Fail-closed probe for the frozen balance-apk-v1 contract.

The v0.14 contract is intentionally consumed structurally rather than by relying
on one private JSON field layout. The probe searches the complete JSON tree for
registered /mod routes, the contract id, event-type collections, and the mobile
surface vocabulary required by the v0.19 APK backend boundary.
"""
from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from typing import Any, Iterable

if len(sys.argv) != 3:
    raise SystemExit("usage: balance_mod_v019_contract_probe.py CONTRACT_JSON MANIFEST_JSON")

contract = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
manifest = json.loads(Path(sys.argv[2]).read_text(encoding="utf-8"))

strings: list[str] = []
containers: list[tuple[str, Any]] = []

def walk(node: Any, path: str = "$") -> None:
    if isinstance(node, str):
        strings.append(node)
        return
    if isinstance(node, dict):
        for k, v in node.items():
            containers.append((str(k).lower(), v))
            walk(v, f"{path}.{k}")
        return
    if isinstance(node, list):
        for i, v in enumerate(node):
            walk(v, f"{path}[{i}]")

walk(contract)

if manifest["apkContract"] not in strings:
    raise SystemExit(f"V019_CONTRACT_FAIL: {manifest['apkContract']!r} not found")

routes = sorted({s for s in strings if s.startswith("/mod/")})
missing = [p for p in manifest["requiredEndpoints"] if p not in routes]
if missing:
    raise SystemExit(f"V019_CONTRACT_FAIL: required endpoint(s) missing: {missing}; found={routes}")

# Prefer an explicit endpoint/routes collection when the contract has one; fall
# back to unique /mod paths. This keeps the check resilient to harmless field
# renames while retaining the frozen minimum.
endpoint_count = len(routes)
for key, value in containers:
    if ("endpoint" in key or "route" in key) and isinstance(value, list):
        endpoint_count = max(endpoint_count, len(value))

# Find explicit event/eventTypes arrays. As a fallback, collect event-looking
# dotted strings. Route strings are already excluded by the regex.
event_re = re.compile(r"^[a-z][a-z0-9_-]*(?:\.[a-z0-9_-]+)+$")
event_strings = {s for s in strings if event_re.match(s)}
event_count = len(event_strings)
for key, value in containers:
    if "event" in key and isinstance(value, list):
        event_count = max(event_count, len(value))

min_endpoints = int(manifest["contractMinimums"]["endpoints"])
min_events = int(manifest["contractMinimums"]["eventTypes"])
if endpoint_count < min_endpoints:
    raise SystemExit(
        f"V019_CONTRACT_FAIL: endpoint inventory {endpoint_count} < frozen minimum {min_endpoints}"
    )
if event_count < min_events:
    raise SystemExit(
        f"V019_CONTRACT_FAIL: event inventory {event_count} < frozen minimum {min_events}"
    )

corpus = "\n".join(strings).lower()
requirements: dict[str, Iterable[str]] = {
    "project": ("project",),
    "chat": ("chat",),
    "agent": ("agent",),
    "file": ("file",),
    "tool": ("tool",),
    "permission": ("permission",),
    "instruction": ("instruction",),
    "skill": ("skill",),
    "budget": ("budget",),
    "live": ("live",),
    "task": ("task",),
    "queue": ("queue", "inbox"),
    "recovery-or-checkpoint": ("recovery", "recover", "checkpoint", "rollback"),
}
for name in manifest["requiredSurfaces"]:
    needles = tuple(requirements[name])
    if not any(n in corpus for n in needles):
        raise SystemExit(f"V019_CONTRACT_FAIL: APK surface {name!r} absent from frozen contract")

print(f"V019_CONTRACT_ID={manifest['apkContract']}")
print(f"V019_CONTRACT_ENDPOINTS={endpoint_count}")
print(f"V019_CONTRACT_EVENT_TYPES={event_count}")
print("V019_CONTRACT_SURFACES=PASS")
