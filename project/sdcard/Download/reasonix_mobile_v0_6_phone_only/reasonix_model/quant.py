from __future__ import annotations
from dataclasses import dataclass
from pathlib import Path
import json
import torch
from torch import nn

@dataclass
class QuantizedTensor:
    q: torch.Tensor
    scale: torch.Tensor


def quantize_int8_per_row(w: torch.Tensor) -> QuantizedTensor:
    if w.ndim != 2:
        raise ValueError("only matrices are supported")
    w = w.detach().float().cpu()
    scale = w.abs().amax(dim=1, keepdim=True).clamp_min(1e-8) / 127.0
    q = torch.clamp(torch.round(w / scale), -127, 127).to(torch.int8)
    return QuantizedTensor(q, scale.squeeze(1).to(torch.float32))


def dequantize_int8_per_row(qt: QuantizedTensor) -> torch.Tensor:
    return qt.q.float() * qt.scale[:, None]


def export_int8(model: nn.Module, out_dir: str | Path) -> dict:
    """Project-owned simple weight export. This is correctness-first, not yet the final mobile kernel format."""
    out = Path(out_dir)
    out.mkdir(parents=True, exist_ok=True)
    manifest = {"format": "reasonix-int8-row-v1", "tensors": {}}
    for name, tensor in model.state_dict().items():
        safe = name.replace(".", "__")
        if tensor.ndim == 2 and tensor.dtype.is_floating_point:
            qt = quantize_int8_per_row(tensor)
            torch.save({"q": qt.q, "scale": qt.scale}, out / f"{safe}.pt")
            manifest["tensors"][name] = {"kind": "int8_per_row", "file": f"{safe}.pt", "shape": list(tensor.shape)}
        else:
            torch.save(tensor.detach().cpu(), out / f"{safe}.pt")
            manifest["tensors"][name] = {"kind": "raw", "file": f"{safe}.pt", "shape": list(tensor.shape)}
    (out / "manifest.json").write_text(json.dumps(manifest, indent=2), encoding="utf-8")
    return manifest
