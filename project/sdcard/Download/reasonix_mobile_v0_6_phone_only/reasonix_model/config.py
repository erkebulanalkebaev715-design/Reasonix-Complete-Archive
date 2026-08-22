from __future__ import annotations
from dataclasses import dataclass, asdict, replace


@dataclass(frozen=True)
class ReasonixConfig:
    vocab_size: int = 260
    d_model: int = 64
    n_layers: int = 4
    d_state: int = 32
    d_latent: int = 32
    n_experts: int = 2
    expert_ff: int = 64
    shared_expert_ff: int = 0  # 0 => same width as routed expert
    attn_every: int = 2
    n_heads: int = 4
    attn_head_dim: int = 16
    attn_value_dim: int = 16
    window_size: int = 16
    anchor_interval: int = 2
    dropout: float = 0.0
    tie_embeddings: bool = True
    fast_depth_fraction: float = 0.50
    standard_depth_fraction: float = 0.75

    def validate(self) -> None:
        assert self.vocab_size >= 260
        assert self.d_model > 0 and self.n_layers > 0
        assert self.d_state > 0 and self.d_latent > 0
        assert self.n_experts >= 1 and self.expert_ff > 0
        assert self.shared_expert_ff >= 0
        assert self.attn_every >= 1 and self.n_heads >= 1
        assert self.window_size >= 1 and self.anchor_interval >= 1
        assert 0 < self.fast_depth_fraction <= self.standard_depth_fraction <= 1

    def as_dict(self) -> dict:
        return asdict(self)

    def depth_for_mode(self, mode: str) -> int:
        mode = mode.lower()
        if mode == "fast": frac = self.fast_depth_fraction
        elif mode == "standard": frac = self.standard_depth_fraction
        elif mode == "deep": return self.n_layers
        else: raise ValueError(f"unknown mode: {mode}")
        return max(1, min(self.n_layers, round(self.n_layers * frac)))

    def with_ablation(self, **changes) -> "ReasonixConfig":
        return replace(self, **changes)


def v03_smoke_config() -> ReasonixConfig:
    return ReasonixConfig(shared_expert_ff=0, attn_every=2)


def smoke_config() -> ReasonixConfig:
    # v0.4 winner: half-width always-on expert + rarer local attention.
    return ReasonixConfig(shared_expert_ff=32, attn_every=4)


def mobile_s_config() -> ReasonixConfig:
    return ReasonixConfig(
        d_model=384, n_layers=12, d_state=160, d_latent=192,
        n_experts=4, expert_ff=512, shared_expert_ff=256, attn_every=4,
        n_heads=6, attn_head_dim=48, attn_value_dim=48,
        window_size=128, anchor_interval=4,
    )


def mobile_m_config() -> ReasonixConfig:
    return ReasonixConfig(
        d_model=512, n_layers=16, d_state=224, d_latent=256,
        n_experts=6, expert_ff=768, shared_expert_ff=384, attn_every=4,
        n_heads=8, attn_head_dim=48, attn_value_dim=48,
        window_size=192, anchor_interval=4,
    )
