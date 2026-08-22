#!/usr/bin/env python3
from __future__ import annotations
import argparse, json
from pathlib import Path
SCHEMA="balance-provider-usage-receipt-v1"

def read_json(path):
    try: return json.loads(Path(path).read_text(encoding="utf-8"))
    except Exception as e: raise SystemExit(f"V020_RECONCILE_FAIL: cannot read JSON: {e}")

def number(v,name):
    if isinstance(v,bool) or not isinstance(v,(int,float)): raise SystemExit(f"V020_RECONCILE_FAIL: usage receipt {name} missing/non-numeric")
    return float(v)

def load_usage(path):
    p=read_json(path)
    if not isinstance(p,dict) or p.get("schema")!=SCHEMA: raise SystemExit("V020_RECONCILE_FAIL: exact provider usage receipt missing/invalid schema")
    model=str(p.get("modelRef","")).strip().lower()
    if "deepseek-v4-flash" not in model or "deepseek-v4-pro" in model: raise SystemExit(f"V020_RECONCILE_FAIL: usage receipt model mismatch: {model!r}")
    if p.get("estimated") is True: raise SystemExit("V020_RECONCILE_FAIL: provider usage receipt is estimated, exact usage required")
    req=int(number(p.get("requestCount"),"requestCount"))
    if req!=1: raise SystemExit(f"V020_RECONCILE_FAIL: requestCount={req}, expected exactly 1")
    prompt=int(number(p.get("promptTokens"),"promptTokens")); completion=int(number(p.get("completionTokens"),"completionTokens")); total=int(number(p.get("totalTokens"),"totalTokens")); hit=int(number(p.get("cacheHitTokens"),"cacheHitTokens")); miss=int(number(p.get("cacheMissTokens"),"cacheMissTokens"))
    if prompt<=0 or completion<=0: raise SystemExit(f"V020_RECONCILE_FAIL: invalid exact usage prompt={prompt} completion={completion}")
    if hit<0 or miss<0: raise SystemExit(f"V020_RECONCILE_FAIL: negative cache counters hit={hit} miss={miss}")
    if hit+miss==0: miss=prompt
    if hit+miss>prompt: raise SystemExit(f"V020_RECONCILE_FAIL: cache counters exceed prompt hit={hit} miss={miss} prompt={prompt}")
    if hit+miss<prompt: miss += prompt-(hit+miss)
    if total>0 and total<prompt+completion: raise SystemExit(f"V020_RECONCILE_FAIL: totalTokens={total} < prompt+completion={prompt+completion}")
    return {"prompt":prompt,"completion":completion,"hit":hit,"miss":miss,"model":model,"requestCount":req}

def spent(path):
    p=read_json(path); b=p.get("budget",p); v=b.get("spentKzt") if isinstance(b,dict) else None
    if isinstance(v,bool) or not isinstance(v,(int,float)): raise SystemExit("V020_RECONCILE_FAIL: spentKzt missing")
    return float(v)

def calc(u,m,fx):
    snap=m["pricingSnapshot"]; r=snap["deepseek-v4-flash"]; unit=float(snap["unitTokens"])
    usd=(u["hit"]*float(r["cacheHit"])+u["miss"]*float(r["input"])+u["completion"]*float(r["output"]))/unit
    return usd,usd*fx

def selftest():
    import tempfile
    with tempfile.TemporaryDirectory() as td:
        d=Path(td); m={"pricingSnapshot":{"unitTokens":1000000,"deepseek-v4-flash":{"cacheHit":.0028,"input":.14,"output":.28}},"reconciliation":{"absoluteToleranceKzt":.05,"relativeTolerance":.05}}
        good={"schema":SCHEMA,"modelRef":"deepseek-v20/deepseek-v4-flash","promptTokens":1000,"completionTokens":100,"totalTokens":1100,"cacheHitTokens":400,"cacheMissTokens":600,"reasoningTokens":0,"requestCount":1,"estimated":False,"finishReason":"stop"}
        (d/'u.json').write_text(json.dumps(good)); u=load_usage(d/'u.json'); assert calc(u,m,500)[1]>0
        bad=dict(good); bad['estimated']=True; (d/'bad.json').write_text(json.dumps(bad))
        try: load_usage(d/'bad.json')
        except SystemExit: pass
        else: raise AssertionError('estimated receipt accepted')
    print('V020_RECONCILE_SELFTEST_PASS')

ap=argparse.ArgumentParser(); ap.add_argument('--self-test',action='store_true'); ap.add_argument('--manifest'); ap.add_argument('--before'); ap.add_argument('--after'); ap.add_argument('--fx',type=float); ap.add_argument('--usage-receipt'); a=ap.parse_args()
if a.self_test: selftest(); raise SystemExit
if not (a.manifest and a.before and a.after and a.fx and a.fx>0 and a.usage_receipt): raise SystemExit('V020_RECONCILE_FAIL: incomplete arguments')
m=read_json(a.manifest); u=load_usage(a.usage_receipt); usd,kzt=calc(u,m,a.fx); delta=spent(a.after)-spent(a.before)
if delta<=0: raise SystemExit(f'V020_RECONCILE_FAIL: spent delta not positive {delta}')
tol=max(float(m['reconciliation']['absoluteToleranceKzt']),kzt*float(m['reconciliation']['relativeTolerance']))
if abs(delta-kzt)>tol: raise SystemExit(f'V020_RECONCILE_FAIL: expectedKzt={kzt:.6f} backendDeltaKzt={delta:.6f} tolerance={tol:.6f} usage={u}')
print(f"V020_RECONCILE_PASS prompt={u['prompt']} hit={u['hit']} miss={u['miss']} output={u['completion']} requests={u['requestCount']} rateCardUSD={usd:.8f} rateCardKZT={kzt:.6f} backendDeltaKZT={delta:.6f} usageReceipt=exact")
