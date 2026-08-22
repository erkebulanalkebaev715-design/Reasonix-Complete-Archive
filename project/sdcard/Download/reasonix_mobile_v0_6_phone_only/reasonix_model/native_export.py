from __future__ import annotations
import copy
import json
import math
import struct
from pathlib import Path
from typing import Iterable

import torch
from torch import nn

from .config import ReasonixConfig, smoke_config
from .model import ReasonixLM
from .quant import quantize_int8_per_row

MAGIC = b"RXM5BIN\0"
VERSION = 1


class DynamicInt8Linear(nn.Module):
    """Reference implementation of Reasonix's dynamic INT8 matvec.

    Weights are per-row INT8. Each input vector is quantized symmetrically to INT8
    at call time, matching native/reasonix_matvec.cpp.
    """
    def __init__(self, src: nn.Linear):
        super().__init__()
        self.in_features = src.in_features
        self.out_features = src.out_features
        qt = quantize_int8_per_row(src.weight)
        self.register_buffer("qweight", qt.q.contiguous())
        self.register_buffer("row_scales", qt.scale.contiguous())
        if src.bias is None:
            self.bias = None
        else:
            self.register_buffer("bias", src.bias.detach().float().cpu().contiguous())

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        xf = x.float()
        max_abs = xf.abs().amax(dim=-1, keepdim=True)
        x_scale = torch.where(max_abs > 1e-12, max_abs / 127.0, torch.ones_like(max_abs))
        qx = torch.clamp(torch.round(xf / x_scale), -127, 127).to(torch.int32)
        qw = self.qweight.to(torch.int32)
        acc = torch.matmul(qx, qw.t()).float()
        y = acc * x_scale * self.row_scales
        if self.bias is not None:
            y = y + self.bias
        return y.to(x.dtype)


def _replace_linears(module: nn.Module) -> None:
    for name, child in list(module.named_children()):
        if isinstance(child, nn.Linear):
            setattr(module, name, DynamicInt8Linear(child))
        else:
            _replace_linears(child)


def make_quant_reference(model: ReasonixLM) -> ReasonixLM:
    q = copy.deepcopy(model).cpu().eval()
    _replace_linears(q)
    return q


def _write_u16(f, v: int): f.write(struct.pack("<H", v))
def _write_u32(f, v: int): f.write(struct.pack("<I", v))
def _write_f32(f, v: float): f.write(struct.pack("<f", float(v)))


def export_rxm5(model: ReasonixLM, path: str | Path) -> dict:
    """Export project-owned single-file native format.

    - embed.weight remains float32 for exact row lookup in v0.5.
    - every other 2-D floating tensor is INT8 per-row.
    - 1-D/scalar floating tensors remain float32.
    """
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    cfg = model.cfg
    sd = model.state_dict()
    tensor_meta = []
    with p.open("wb") as f:
        f.write(MAGIC)
        _write_u32(f, VERSION)
        config_ints = [
            cfg.vocab_size, cfg.d_model, cfg.n_layers, cfg.d_state, cfg.d_latent,
            cfg.n_experts, cfg.expert_ff, (cfg.shared_expert_ff or cfg.expert_ff),
            cfg.attn_every, cfg.n_heads, cfg.attn_head_dim, cfg.attn_value_dim,
            cfg.window_size, cfg.anchor_interval,
        ]
        for v in config_ints: _write_u32(f, int(v))
        _write_f32(f, cfg.fast_depth_fraction)
        _write_f32(f, cfg.standard_depth_fraction)
        _write_u32(f, len(sd))

        for name, tensor in sd.items():
            t = tensor.detach().float().cpu().contiguous()
            raw_name = name.encode("utf-8")
            _write_u16(f, len(raw_name)); f.write(raw_name)
            if t.ndim == 2 and name != "embed.weight":
                kind = 1
            else:
                kind = 0
            f.write(struct.pack("<B", kind))
            f.write(struct.pack("<B", t.ndim))
            for d in t.shape: _write_u32(f, int(d))
            if kind == 1:
                qt = quantize_int8_per_row(t)
                f.write(qt.scale.numpy().astype("<f4", copy=False).tobytes(order="C"))
                f.write(qt.q.numpy().astype("i1", copy=False).tobytes(order="C"))
            else:
                f.write(t.numpy().astype("<f4", copy=False).tobytes(order="C"))
            tensor_meta.append({"name": name, "kind": "int8_row" if kind else "f32", "shape": list(t.shape)})
    return {"format": "reasonix-native-rxm5-v1", "path": str(p), "tensors": tensor_meta, "config": cfg.as_dict()}


@torch.no_grad()
def build_fixture(out_dir: str | Path, seed: int = 20260818) -> dict:
    out = Path(out_dir); out.mkdir(parents=True, exist_ok=True)
    torch.manual_seed(seed)
    model = ReasonixLM(smoke_config()).eval()
    rxm = out / "smoke_v05.rxm"
    meta = export_rxm5(model, rxm)
    qref = make_quant_reference(model)
    prompt = [82, 101, 97, 115, 111, 110, 105, 120]  # "Reasonix"
    modes = {}
    for mode in ("fast", "standard", "deep"):
        cache = qref.init_cache(1)
        logits = None
        for tok in prompt:
            logits, cache, _ = qref.step(torch.tensor([tok], dtype=torch.long), cache, mode=mode)
        greedy = []
        for _ in range(8):
            nxt = int(logits.argmax(dim=-1).item())
            greedy.append(nxt)
            logits, cache, _ = qref.step(torch.tensor([nxt], dtype=torch.long), cache, mode=mode)
        modes[mode] = {"greedy": greedy, "last_logits": logits[0].float().cpu().tolist()}
    ref = {"seed": seed, "prompt": prompt, "modes": modes, "model": meta}
    (out / "native_reference.json").write_text(json.dumps(ref), encoding="utf-8")
    return ref


if __name__ == "__main__":
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument("--out-dir", default="results/native_fixture")
    args = ap.parse_args()
    r = build_fixture(args.out_dir)
    print(json.dumps({"prompt": r["prompt"], "modes": {k:v["greedy"] for k,v in r["modes"].items()}, "model": r["model"]["path"]}, indent=2))
