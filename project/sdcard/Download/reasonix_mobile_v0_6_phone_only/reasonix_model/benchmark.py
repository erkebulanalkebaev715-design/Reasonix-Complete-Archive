from __future__ import annotations
import json, time
from pathlib import Path
import torch
from .config import smoke_config, mobile_s_config, mobile_m_config
from .metrics import estimate_parameters
from .model import ReasonixLM

@torch.no_grad()
def microbench(steps: int = 100) -> dict:
    torch.manual_seed(1)
    cfg = smoke_config()
    m = ReasonixLM(cfg).eval()
    token = torch.tensor([65])
    cache = m.init_cache(1)
    # warmup
    for _ in range(10):
        _, cache, _ = m.step(token, cache, mode="deep")
    cache = m.init_cache(1)
    t0 = time.perf_counter()
    for _ in range(steps):
        _, cache, _ = m.step(token, cache, mode="deep")
    dt = time.perf_counter() - t0
    return {
        "host_only_not_phone": True,
        "smoke_decode_steps": steps,
        "elapsed_s": dt,
        "tokens_per_s": steps / dt,
        "profiles": {
            "smoke": estimate_parameters(smoke_config()),
            "mobile_s": estimate_parameters(mobile_s_config()),
            "mobile_m": estimate_parameters(mobile_m_config()),
        },
    }

def main() -> None:
    r = microbench()
    Path("results").mkdir(exist_ok=True)
    Path("results/host_arch_bench.json").write_text(json.dumps(r, indent=2), encoding="utf-8")
    print(json.dumps(r, indent=2))

if __name__ == "__main__": main()
