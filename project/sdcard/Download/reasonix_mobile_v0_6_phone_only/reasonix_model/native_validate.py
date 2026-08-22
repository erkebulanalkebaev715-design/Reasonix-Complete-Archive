from __future__ import annotations
import argparse
import json
import subprocess
from pathlib import Path

from .native_export import build_fixture


def run_validation(root: str | Path = ".") -> dict:
    root = Path(root).resolve()
    fixture_dir = root / "results" / "native_fixture"
    ref = build_fixture(fixture_dir)
    cli = root / "native" / "reasonix-native"
    if not cli.exists():
        subprocess.run([str(root / "native" / "build_host.sh")], check=True, cwd=root)
    prompt = ",".join(str(x) for x in ref["prompt"])
    results = {}
    for mode, mref in ref["modes"].items():
        cp = subprocess.run([str(cli), str(fixture_dir / "smoke_v05.rxm"), prompt, str(len(mref["greedy"])), mode], check=True, text=True, capture_output=True)
        lines = dict(line.split("=",1) for line in cp.stdout.strip().splitlines() if "=" in line)
        native_greedy = [int(x) for x in lines["GREEDY"].split(",") if x]
        native_logits = [float(x) for x in lines["LOGITS"].split(",") if x]
        ref_logits = mref["last_logits"]
        if len(native_logits) != len(ref_logits):
            raise AssertionError((mode, len(native_logits), len(ref_logits)))
        max_abs = max(abs(a-b) for a,b in zip(native_logits, ref_logits))
        mean_abs = sum(abs(a-b) for a,b in zip(native_logits, ref_logits))/len(ref_logits)
        greedy_match = native_greedy == mref["greedy"]
        results[mode] = {
            "greedy_match": greedy_match, "python_greedy": mref["greedy"], "native_greedy": native_greedy,
            "max_abs_logit_error": max_abs, "mean_abs_logit_error": mean_abs,
            "pass": greedy_match and max_abs < 0.02,
        }
    result = {"modes": results, "pass": all(x["pass"] for x in results.values())}
    (root / "results" / "native_validation.json").write_text(json.dumps(result, indent=2), encoding="utf-8")
    if not result["pass"]: raise AssertionError(result)
    return result



if __name__ == "__main__":
    ap=argparse.ArgumentParser(); ap.add_argument("--root",default="."); args=ap.parse_args()
    print(json.dumps(run_validation(args.root), indent=2))
