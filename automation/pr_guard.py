#!/usr/bin/env python3
from __future__ import annotations
import argparse, json, os, sys, urllib.request

PROTECTED = (
    ".github/",
    "automation/",
    ".reasonix/governance/",
)
EXACT_PROTECTED = {
    ".gitmodules",
}

def api(path: str):
    token = os.environ["GITHUB_TOKEN"]
    req = urllib.request.Request(
        "https://api.github.com" + path,
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "reasonix-autonomy",
        },
    )
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.loads(r.read())

def all_files(repo: str, pr: int):
    page = 1
    out = []
    while True:
        batch = api(f"/repos/{repo}/pulls/{pr}/files?per_page=100&page={page}")
        out.extend(batch)
        if len(batch) < 100:
            break
        page += 1
    return out

def main():
    p = argparse.ArgumentParser()
    p.add_argument("--repo", required=True)
    p.add_argument("--pr", type=int, required=True)
    p.add_argument("--expected-head", required=True)
    p.add_argument("--expected-base", required=True)
    args = p.parse_args()

    pr = api(f"/repos/{args.repo}/pulls/{args.pr}")
    if pr.get("state") != "open":
        raise SystemExit("PR_GUARD_FAIL: PR is not open")
    if pr.get("draft"):
        raise SystemExit("PR_GUARD_FAIL: draft PR")
    if pr["base"]["ref"] != "next":
        raise SystemExit("PR_GUARD_FAIL: base must be next")
    if pr["head"]["sha"] != args.expected_head:
        raise SystemExit("PR_GUARD_FAIL: head changed since validation")
    if pr["base"]["sha"] != args.expected_base:
        raise SystemExit("PR_GUARD_FAIL: next changed since validation; re-validation required")

    files = all_files(args.repo, args.pr)
    if not files:
        raise SystemExit("PR_GUARD_FAIL: empty PR")
    if len(files) > 500:
        raise SystemExit("PR_GUARD_FAIL: PR changes more than 500 files; split it")

    blocked = []
    for f in files:
        name = f["filename"]
        if name in EXACT_PROTECTED or any(name.startswith(prefix) for prefix in PROTECTED):
            blocked.append(name)
    if blocked:
        raise SystemExit("PR_GUARD_FAIL: governance paths are not auto-mergeable: " + ", ".join(blocked[:30]))

    print(f"PR_GUARD_PASS files={len(files)} head={args.expected_head} base={args.expected_base}")

if __name__ == "__main__":
    main()
