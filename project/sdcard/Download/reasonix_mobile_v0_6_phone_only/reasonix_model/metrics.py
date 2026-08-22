from __future__ import annotations
from dataclasses import asdict
from .config import ReasonixConfig


def estimate_parameters(cfg: ReasonixConfig) -> dict:
    d,s,l,e,f=cfg.d_model,cfg.d_state,cfg.d_latent,cfg.n_experts,cfg.expert_ff
    sf=cfg.shared_expert_ff or f
    total=cfg.vocab_size*d + (0 if cfg.tie_embeddings else cfg.vocab_size*d) + d
    attention_layers=0; active=cfg.vocab_size*d
    for i in range(cfg.n_layers):
        state=d + d*s + (d*s+s) + s*d + (d*d+d)
        routed_one=3*l*f; shared_one=3*l*sf
        experts=d + d*l + e*routed_one + shared_one + (l*e+e) + l*d
        layer=state+experts
        active_layer=(d*s*3+d*d)+(d*l+routed_one+shared_one+l*e+l*d)
        if (i+1)%cfg.attn_every==0:
            attention_layers+=1; h,kd,vd=cfg.n_heads,cfg.attn_head_dim,cfg.attn_value_dim
            attn=d+d*h*kd+d*h*kd+d*h*vd+h*vd*d+(d*d+d)+h
            layer+=attn; active_layer+=attn
        if (i+1)%cfg.anchor_interval==0: layer+=d
        total+=layer; active+=active_layer
    return {
        "config":asdict(cfg),"parameters":int(total),
        "fp32_mb":round(total*4/1024**2,2),"fp16_mb":round(total*2/1024**2,2),
        "int8_weight_mb_ideal":round(total/1024**2,2),"int4_weight_mb_ideal":round(total*0.5/1024**2,2),
        "attention_layers":attention_layers,"approx_active_linear_weights_per_token":int(active),
    }
