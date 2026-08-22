from __future__ import annotations
import argparse, json, random, time
from pathlib import Path
import torch
from .config import ReasonixConfig, smoke_config
from .model import ReasonixLM
from .tokenizer import ByteTokenizer


def make_batch(ids: list[int], batch: int, seq: int, device: str) -> torch.Tensor:
    if len(ids) <= seq + 1:
        ids = ids * ((seq + 2) // max(1, len(ids)) + 1)
    starts = [random.randint(0, len(ids) - seq - 1) for _ in range(batch)]
    return torch.tensor([ids[s:s+seq+1] for s in starts], dtype=torch.long, device=device)


def train_smoke(
    corpus: str,
    steps: int = 40,
    seq: int = 32,
    batch: int = 2,
    lr: float = 3e-3,
    device: str = "cpu",
    cfg: ReasonixConfig | None = None,
    seed: int = 7,
) -> dict:
    tok = ByteTokenizer()
    ids = tok.encode(corpus, bos=False, eos=True)
    torch.manual_seed(seed); random.seed(seed)
    model = ReasonixLM(cfg or smoke_config()).to(device)
    opt = torch.optim.AdamW(model.parameters(), lr=lr)
    losses = []
    t0 = time.perf_counter()
    model.train()
    for step in range(steps):
        x = make_batch(ids, batch, seq, device)
        mode = ("fast", "standard", "deep")[step % 3]
        loss = model.loss(x, mode=mode)
        opt.zero_grad(set_to_none=True)
        loss.backward()
        torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
        opt.step()
        losses.append(float(loss.detach()))
    elapsed = time.perf_counter() - t0
    tail_n = min(5, len(losses))
    return {
        "steps": steps,
        "initial_loss": losses[0],
        "final_loss": losses[-1],
        "best_loss": min(losses),
        "tail_mean_loss": sum(losses[-tail_n:]) / tail_n,
        "elapsed_s": elapsed,
        "loss_decreased": losses[-1] < losses[0],
        "losses": losses,
        "model": model,
        "tokenizer": tok,
    }


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--corpus", required=True)
    ap.add_argument("--steps", type=int, default=40)
    ap.add_argument("--out", default="results/smoke_train.json")
    args = ap.parse_args()
    text = Path(args.corpus).read_text(encoding="utf-8")
    r = train_smoke(text, steps=args.steps)
    serial = {k:v for k,v in r.items() if k not in {"model","tokenizer","losses"}}
    Path(args.out).parent.mkdir(parents=True, exist_ok=True)
    Path(args.out).write_text(json.dumps(serial, indent=2), encoding="utf-8")
    print(json.dumps(serial, indent=2))


if __name__ == "__main__":
    main()
