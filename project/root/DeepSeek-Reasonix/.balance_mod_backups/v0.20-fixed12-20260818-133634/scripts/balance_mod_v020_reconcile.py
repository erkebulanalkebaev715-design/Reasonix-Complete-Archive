#!/usr/bin/env python3
from __future__ import annotations
import argparse,json,re
from pathlib import Path
from typing import Any

def load_jsonish(path):
    raw=Path(path).read_text(encoding="utf-8",errors="replace"); out=[]
    try: out.append(json.loads(raw))
    except Exception: pass
    for line in raw.splitlines():
        line=line.strip()
        if line.startswith("data:"):
            data=line[5:].strip()
            if data and data!="[DONE]":
                try: out.append(json.loads(data))
                except Exception: pass
    return out

def walk(n:Any):
    yield n
    if isinstance(n,dict):
        for v in n.values(): yield from walk(v)
    elif isinstance(n,list):
        for v in n: yield from walk(v)

def norm(k): return re.sub(r"[^a-z0-9]","",str(k).lower())
def num(d,*names):
    t={norm(k):v for k,v in d.items()}
    for name in names:
        v=t.get(norm(name))
        if isinstance(v,(int,float)) and not isinstance(v,bool): return float(v)
    return None

def candidates(roots):
    out=[]
    for root in roots:
        for n in walk(root):
            if not isinstance(n,dict): continue
            p=num(n,"prompt_tokens","promptTokens","input_tokens","inputTokens")
            c=num(n,"completion_tokens","completionTokens","output_tokens","outputTokens")
            if p is None or c is None: continue
            hit=num(n,"prompt_cache_hit_tokens","cacheHitTokens","cache_read_input_tokens","cached_tokens") or 0.0
            miss=num(n,"prompt_cache_miss_tokens","cacheMissTokens")
            miss=max(0.0,p-hit) if miss is None else miss
            model=""
            for k,v in n.items():
                if norm(k) in ("model","modelref") and isinstance(v,str): model=v
            out.append({"prompt":p,"completion":c,"hit":hit,"miss":miss,"model":model})
    return out

def calc(u,m,fx):
    r=m["pricingSnapshot"]["deepseek-v4-flash"]; unit=float(m["pricingSnapshot"]["unitTokens"])
    usd=(u["hit"]*r["cacheHit"]+u["miss"]*r["input"]+u["completion"]*r["output"])/unit
    return usd,usd*fx

def spent(path):
    p=json.loads(Path(path).read_text()); b=p.get("budget",p); v=b.get("spentKzt")
    if not isinstance(v,(int,float)): raise SystemExit("V020_RECONCILE_FAIL: spentKzt missing")
    return float(v)

def selftest():
    m={"pricingSnapshot":{"unitTokens":1000000,"deepseek-v4-flash":{"cacheHit":.0028,"input":.14,"output":.28}},"reconciliation":{"absoluteToleranceKzt":.05,"relativeTolerance":.05}}
    u={"prompt":1000,"completion":100,"hit":400,"miss":600}
    usd,kzt=calc(u,m,500); assert usd>0 and kzt>0
    print("V020_RECONCILE_SELFTEST_PASS")

ap=argparse.ArgumentParser(); ap.add_argument("--self-test",action="store_true"); ap.add_argument("--manifest"); ap.add_argument("--before"); ap.add_argument("--after"); ap.add_argument("--fx",type=float); ap.add_argument("sources",nargs="*"); a=ap.parse_args()
if a.self_test: selftest(); raise SystemExit
if not (a.manifest and a.before and a.after and a.fx and a.fx>0 and a.sources): raise SystemExit("V020_RECONCILE_FAIL: incomplete arguments")
m=json.loads(Path(a.manifest).read_text()); roots=[]
for s in a.sources: roots+=load_jsonish(s)
cs=candidates(roots)
if not cs: raise SystemExit("V020_RECONCILE_FAIL: provider-reported usage not found")
flash=[x for x in cs if not x["model"] or "deepseek-v4-flash" in x["model"].lower()]; cs=flash or cs
u=max(cs,key=lambda x:x["prompt"]+x["completion"])
if u["prompt"]<=0 or u["completion"]<=0 or u["hit"]+u["miss"]>u["prompt"]+1: raise SystemExit(f"V020_RECONCILE_FAIL: invalid usage {u}")
usd,kzt=calc(u,m,a.fx); delta=spent(a.after)-spent(a.before)
if delta<=0: raise SystemExit(f"V020_RECONCILE_FAIL: spent delta not positive {delta}")
tol=max(float(m["reconciliation"]["absoluteToleranceKzt"]),kzt*float(m["reconciliation"]["relativeTolerance"]))
if abs(delta-kzt)>tol: raise SystemExit(f"V020_RECONCILE_FAIL: expectedKzt={kzt:.6f} backendDeltaKzt={delta:.6f} tolerance={tol:.6f} usage={u}")
print(f"V020_RECONCILE_PASS prompt={int(u['prompt'])} hit={int(u['hit'])} miss={int(u['miss'])} output={int(u['completion'])} rateCardUSD={usd:.8f} rateCardKZT={kzt:.6f} backendDeltaKZT={delta:.6f}")
