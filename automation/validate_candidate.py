#!/usr/bin/env python3
from __future__ import annotations

import argparse
import html
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.request

CURRENT_CORE = Path("project/root/DeepSeek-Reasonix")
CURRENT_BACKEND = Path("project/root/reasonix-mobile-v1.5.1-backend")
CURRENT_MOBILE = Path("project/root/reasonix-mobile-v3.0.1/mobile")
EVOLUTION_MANIFEST = Path("reasonix-evolution.json")

SOURCE_SUFFIXES = {
    ".go", ".py", ".java", ".kt", ".kts", ".rs", ".c", ".cc", ".cpp", ".h",
    ".hpp", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".swift", ".dart",
    ".rb", ".php", ".cs", ".scala", ".lua", ".zig", ".ex", ".exs", ".sh"
}

def die(msg: str) -> None:
    print(f"AUTONOMY_FAIL: {msg}", file=sys.stderr)
    raise SystemExit(1)

def safe_env() -> dict[str, str]:
    env = {}
    for k, v in os.environ.items():
        ku = k.upper()
        if ku in {
            "GITHUB_TOKEN", "GH_TOKEN", "ACTIONS_RUNTIME_TOKEN",
            "ACTIONS_ID_TOKEN_REQUEST_TOKEN", "ACTIONS_CACHE_URL",
            "ACTIONS_RESULTS_URL", "GITHUB_OUTPUT", "GITHUB_ENV",
            "GITHUB_PATH", "GITHUB_STEP_SUMMARY"
        }:
            continue
        if any(x in ku for x in ("API_KEY", "SECRET", "PASSWORD", "PRIVATE_KEY")):
            continue
        env[k] = v
    env["CI"] = "true"
    env["GIT_TERMINAL_PROMPT"] = "0"
    return env

def run(cmd, cwd: Path, timeout: int = 1200, env_extra=None) -> None:
    cmd = [str(x) for x in cmd]
    print("+", " ".join(cmd))
    env = safe_env()
    if env_extra:
        env.update({str(k): str(v) for k, v in env_extra.items()})
    try:
        cp = subprocess.run(cmd, cwd=cwd, env=env, timeout=timeout)
    except subprocess.TimeoutExpired:
        die(f"timeout after {timeout}s: {' '.join(cmd)}")
    if cp.returncode != 0:
        die(f"command failed rc={cp.returncode}: {' '.join(cmd)}")

def structural_guard(candidate: Path) -> None:
    if not candidate.is_dir():
        die("candidate directory missing")

    giant = []
    source_count = 0
    nonempty_count = 0
    for p in candidate.rglob("*"):
        if ".git" in p.parts:
            continue
        if not p.is_file():
            continue
        try:
            size = p.stat().st_size
        except OSError:
            continue
        if size:
            nonempty_count += 1
        if p.suffix.lower() in SOURCE_SUFFIXES and size > 0:
            source_count += 1
        if size > 95 * 1024 * 1024:
            giant.append((str(p.relative_to(candidate)), size))

    if giant:
        die("files over 95 MiB are not accepted by the autonomous gate: " +
            ", ".join(f"{p}={n}" for p, n in giant[:10]))
    if nonempty_count < 5:
        die("candidate is effectively empty")
    print(f"STRUCTURE nonempty_files={nonempty_count} source_files={source_count}")

def check_html_js(index_html: Path) -> None:
    if not index_html.is_file():
        return
    node = shutil.which("node")
    if not node:
        print("SKIP inline JS syntax: node unavailable")
        return
    text = index_html.read_text(encoding="utf-8", errors="replace")
    scripts = re.findall(r"<script(?:\s[^>]*)?>(.*?)</script>", text, flags=re.I | re.S)
    if not scripts:
        print("SKIP inline JS syntax: no script blocks")
        return
    with tempfile.NamedTemporaryFile("w", suffix=".js", delete=False, encoding="utf-8") as f:
        for block in scripts:
            f.write(html.unescape(block))
            f.write("\n")
        temp = Path(f.name)
    try:
        run([node, "--check", str(temp)], index_html.parent, timeout=60)
    finally:
        temp.unlink(missing_ok=True)

def current_architecture(candidate: Path, full: bool) -> None:
    core = candidate / CURRENT_CORE
    backend = candidate / CURRENT_BACKEND
    mobile = candidate / CURRENT_MOBILE

    if not (core / "go.mod").is_file():
        die("current architecture selected but go.mod is missing")

    go = shutil.which("go")
    if not go:
        die("Go toolchain is unavailable")

    run([go, "version"], core, timeout=30)
    run([go, "vet", "./..."], core, timeout=1200,
        env_extra={"REASONIX_RELEASE_CACHE_GUARD": "1"})
    run([go, "build", "./..."], core, timeout=1200,
        env_extra={"REASONIX_RELEASE_CACHE_GUARD": "1"})
    run([go, "test", "-p=1", "-timeout=12m", "./..."], core, timeout=1800,
        env_extra={"REASONIX_RELEASE_CACHE_GUARD": "1", "GOMAXPROCS": "1"})

    if full:
        race_targets = [
            "./internal/agent/...", "./internal/plugin/...", "./internal/jobs/...",
            "./internal/proc/...", "./internal/sandbox/...", "./internal/filelock/...",
            "./internal/eventwire/...", "./internal/remote/...", "./internal/extension/...",
            "./internal/boot/...", "./internal/control/...", "./internal/tool/..."
        ]
        run([go, "test", "-race", "-timeout=15m", *race_targets],
            core, timeout=2400, env_extra={"REASONIX_RELEASE_CACHE_GUARD": "1"})

    if backend.is_dir():
        shell_files = sorted(backend.glob("*.sh"))
        py_files = sorted(backend.glob("*.py"))
        for p in shell_files:
            run(["bash", "-n", str(p)], candidate, timeout=30)
        if py_files:
            run([sys.executable, "-m", "py_compile", *map(str, py_files)],
                candidate, timeout=120)

    if mobile.is_dir():
        required = [
            mobile / "AndroidManifest.xml",
            mobile / "src/com/reasonix/mobile/installfix/MainActivity.java",
            mobile / "assets/index.html",
            mobile / "build_in_termux.sh",
        ]
        missing = [str(p.relative_to(candidate)) for p in required if not p.is_file()]
        if missing:
            die("mobile lineage files missing: " + ", ".join(missing))
        run(["bash", "-n", str(mobile / "build_in_termux.sh")], candidate, timeout=30)
        check_html_js(mobile / "assets/index.html")

    swarm_gate = core / "scripts/balance_mod_swarm_gate.sh"
    if swarm_gate.is_file():
        run(["bash", str(swarm_gate)], core, timeout=900,
            env_extra={"GOTOOLCHAIN": "local"})

    print("CURRENT_ARCHITECTURE_PASS")

def validate_cmd_list(name: str, value):
    if not isinstance(value, list) or not value:
        die(f"evolution manifest requires non-empty {name} list")
    out = []
    for i, cmd in enumerate(value):
        if not isinstance(cmd, list) or not cmd or not all(isinstance(x, (str, int, float)) for x in cmd):
            die(f"{name}[{i}] must be a non-empty argv array")
        out.append([str(x) for x in cmd])
    return out

def wait_json(url: str, timeout: int = 30):
    deadline = time.time() + timeout
    last = None
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(url, timeout=2) as r:
                raw = r.read()
            return json.loads(raw)
        except Exception as e:
            last = e
            time.sleep(0.5)
    die(f"black-box endpoint did not become ready: {url}: {last}")

def blackbox_probe(candidate: Path, cfg: dict) -> None:
    if not isinstance(cfg, dict):
        die("blackbox must be an object")
    start = cfg.get("start")
    if not isinstance(start, list) or not start:
        die("blackbox.start must be an argv array")
    contract_url = str(cfg.get("contractUrl") or "").strip()
    expected_protocol = str(cfg.get("expectedProtocol") or "").strip()
    if not contract_url or not expected_protocol:
        die("blackbox requires contractUrl and expectedProtocol")

    env = safe_env()
    proc = subprocess.Popen([str(x) for x in start], cwd=candidate, env=env,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                            text=True, start_new_session=True)
    try:
        payload = wait_json(contract_url, timeout=int(cfg.get("readyTimeoutSeconds", 30)))
        got = str(payload.get("protocolVersion") or "")
        if got != expected_protocol:
            die(f"black-box protocol mismatch: got={got!r} expected={expected_protocol!r}")
        if expected_protocol == "balance-apk-v1":
            eps = payload.get("endpoints")
            events = payload.get("events")
            if not isinstance(eps, list) or len(eps) < 72:
                die("balance-apk-v1 compatibility claim has insufficient endpoint surface")
            if not isinstance(events, list) or len(events) < 85:
                die("balance-apk-v1 compatibility claim has insufficient event surface")
        print(f"BLACKBOX_PASS protocol={got}")
    finally:
        try:
            proc.terminate()
            proc.wait(timeout=5)
        except Exception:
            try:
                proc.kill()
            except Exception:
                pass

def evolution_architecture(candidate: Path, full: bool) -> None:
    mf = candidate / EVOLUTION_MANIFEST
    try:
        data = json.loads(mf.read_text(encoding="utf-8"))
    except Exception as e:
        die(f"cannot parse {EVOLUTION_MANIFEST}: {e}")

    if data.get("schemaVersion") != 1:
        die("reasonix-evolution.json schemaVersion must be 1")
    arch = str(data.get("architecture") or "").strip()
    if not arch:
        die("evolution manifest architecture is required")
    if data.get("majorEvolution") is not True:
        die("replacement of the historical architecture requires majorEvolution=true")

    build = validate_cmd_list("build", data.get("build"))
    tests = validate_cmd_list("test", data.get("test"))

    migration = str(data.get("migrationDoc") or "").strip()
    compat = data.get("compatibility")
    if not isinstance(compat, dict):
        die("compatibility object is required")
    v1 = str(compat.get("balance-apk-v1") or "").strip()
    if v1 not in {"preserved", "migration"}:
        die("compatibility.balance-apk-v1 must be preserved or migration")
    if v1 == "migration":
        if not migration or not (candidate / migration).is_file():
            die("protocol migration requires an existing migrationDoc")

    for cmd in build:
        run(cmd, candidate, timeout=1800)
    for cmd in tests:
        run(cmd, candidate, timeout=1800)

    blackbox = data.get("blackbox")
    if v1 == "preserved":
        if blackbox is None:
            die("claiming balance-apk-v1 preserved requires a blackbox contract probe")
        blackbox_probe(candidate, blackbox)
    elif blackbox is not None:
        blackbox_probe(candidate, blackbox)

    source_count = sum(
        1 for p in candidate.rglob("*")
        if p.is_file() and ".git" not in p.parts and p.suffix.lower() in SOURCE_SUFFIXES and p.stat().st_size > 0
    )
    if source_count < 3:
        die("replacement architecture has too little executable/source material to validate")

    print(f"EVOLUTION_ARCHITECTURE_PASS architecture={arch}")

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--candidate", required=True)
    ap.add_argument("--full", action="store_true")
    args = ap.parse_args()

    candidate = Path(args.candidate).resolve()
    structural_guard(candidate)

    if (candidate / CURRENT_CORE / "go.mod").is_file():
        current_architecture(candidate, args.full)
    elif (candidate / EVOLUTION_MANIFEST).is_file():
        evolution_architecture(candidate, args.full)
    else:
        die(
            "no supported architecture found: retain the current Reasonix core or "
            "provide reasonix-evolution.json for an intentional major redesign"
        )

    print("AUTONOMY_VALIDATION_PASS")

if __name__ == "__main__":
    main()
