from __future__ import annotations
import json, statistics, time
from pathlib import Path
import torch
from .config import v03_smoke_config
from .metrics import estimate_parameters
from .train import train_smoke


@torch.no_grad()
def decode_bench(model, steps: int = 100, repeats: int = 3) -> float:
    model.eval(); rates=[]; tok=torch.tensor([65])
    for _ in range(repeats):
        cache=model.init_cache(1)
        for _ in range(8): _,cache,_=model.step(tok,cache,mode="deep")
        cache=model.init_cache(1); t0=time.perf_counter()
        for _ in range(steps): _,cache,_=model.step(tok,cache,mode="deep")
        rates.append(steps/(time.perf_counter()-t0))
    return statistics.median(rates)


def run_ablation(corpus: str, steps: int = 36) -> dict:
    base=v03_smoke_config()
    candidates={
        "v03_control": base,
        "compact_shared_half": base.with_ablation(shared_expert_ff=32),
        "compact_shared_quarter": base.with_ablation(shared_expert_ff=16),
        "v04_compact_rare": base.with_ablation(shared_expert_ff=32, attn_every=4),
    }
    results={}
    for name,cfg in candidates.items():
        r=train_smoke(corpus,steps=steps,seq=32,batch=2,lr=3e-3,cfg=cfg,seed=7)
        model=r["model"]
        actual=sum(p.numel() for p in model.parameters())
        est=estimate_parameters(cfg)["parameters"]
        if actual != est:
            raise RuntimeError(f"parameter estimate mismatch {name}: {actual} != {est}")
        results[name]={
            "parameters":actual,
            "tail_mean_loss":r["tail_mean_loss"],
            "best_loss":r["best_loss"],
            "final_loss":r["final_loss"],
            "host_decode_tokens_s":decode_bench(model),
            "host_only_not_phone":True,
        }
    ctrl=results["v03_control"]
    for v in results.values():
        v["tail_pct_vs_v03"]=(v["tail_mean_loss"]/ctrl["tail_mean_loss"]-1)*100
        v["params_pct_vs_v03"]=(v["parameters"]/ctrl["parameters"]-1)*100
        v["decode_pct_vs_v03"]=(v["host_decode_tokens_s"]/ctrl["host_decode_tokens_s"]-1)*100
    return {
        "steps":steps,
        "retained":"v04_compact_rare",
        "selection_reason":"only candidate in this smoke gate with lower tail loss, fewer parameters, and faster host decode than v03 control",
        "candidates":results,
        "note":"host decode is not Realme Note 60 performance",
    }


def main():
    corpus=Path("data/smoke_corpus.txt").read_text(encoding="utf-8")
    r=run_ablation(corpus)
    Path("results").mkdir(exist_ok=True)
    Path("results/ablation_v04.json").write_text(json.dumps(r,indent=2),encoding="utf-8")
    print(json.dumps(r,indent=2))


if __name__ == "__main__": main()
