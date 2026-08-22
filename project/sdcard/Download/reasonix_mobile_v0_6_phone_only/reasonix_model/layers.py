from __future__ import annotations
import math
from dataclasses import dataclass
import torch
from torch import nn
import torch.nn.functional as F


class RMSNorm(nn.Module):
    def __init__(self, d: int, eps: float = 1e-6):
        super().__init__(); self.weight = nn.Parameter(torch.ones(d)); self.eps = eps
    def forward(self, x: torch.Tensor) -> torch.Tensor:
        return x * torch.rsqrt(x.pow(2).mean(dim=-1, keepdim=True) + self.eps) * self.weight


@dataclass
class StateCache:
    state: torch.Tensor


class SelectivePocketState(nn.Module):
    """Compact project recurrent state with token-conditioned keep/write/read gates."""
    def __init__(self, d_model: int, d_state: int):
        super().__init__()
        self.norm = RMSNorm(d_model)
        self.to_write = nn.Linear(d_model, d_state, bias=False)
        self.to_keep = nn.Linear(d_model, d_state, bias=True)
        self.to_read = nn.Linear(d_state, d_model, bias=False)
        self.to_gate = nn.Linear(d_model, d_model, bias=True)
        nn.init.constant_(self.to_keep.bias, 1.5); nn.init.constant_(self.to_gate.bias, 1.0)
    def init_cache(self, batch: int, device, dtype) -> StateCache:
        return StateCache(torch.zeros(batch, self.to_write.out_features, device=device, dtype=dtype))
    def step(self, x: torch.Tensor, cache: StateCache) -> tuple[torch.Tensor, StateCache]:
        z = self.norm(x)
        keep = torch.sigmoid(self.to_keep(z)); write = torch.tanh(self.to_write(z))
        new_state = keep * cache.state + (1.0 - keep) * write
        return self.to_read(new_state) * torch.sigmoid(self.to_gate(z)), StateCache(new_state)


@dataclass
class AttentionCache:
    keys: torch.Tensor
    values: torch.Tensor


class WindowLatentAttention(nn.Module):
    """Rare causal local attention; K/V history is hard-bounded by window."""
    def __init__(self, d_model: int, n_heads: int, k_dim: int, v_dim: int, window: int):
        super().__init__()
        self.n_heads, self.k_dim, self.v_dim, self.window = n_heads, k_dim, v_dim, window
        self.norm = RMSNorm(d_model)
        self.q = nn.Linear(d_model, n_heads*k_dim, bias=False)
        self.k = nn.Linear(d_model, n_heads*k_dim, bias=False)
        self.v = nn.Linear(d_model, n_heads*v_dim, bias=False)
        self.out = nn.Linear(n_heads*v_dim, d_model, bias=False)
        self.gate = nn.Linear(d_model, d_model, bias=True)
        self.recency_log_slope = nn.Parameter(torch.full((n_heads,), -2.0))
        nn.init.constant_(self.gate.bias, 0.5)
    def init_cache(self, batch: int, device, dtype) -> AttentionCache:
        return AttentionCache(
            torch.empty(batch,self.n_heads,0,self.k_dim,device=device,dtype=dtype),
            torch.empty(batch,self.n_heads,0,self.v_dim,device=device,dtype=dtype),
        )
    def step(self, x: torch.Tensor, cache: AttentionCache) -> tuple[torch.Tensor, AttentionCache]:
        b=x.shape[0]; z=self.norm(x)
        q=self.q(z).view(b,self.n_heads,self.k_dim)
        k=self.k(z).view(b,self.n_heads,1,self.k_dim); v=self.v(z).view(b,self.n_heads,1,self.v_dim)
        keys=torch.cat((cache.keys,k),dim=2)[:,:,-self.window:,:]
        values=torch.cat((cache.values,v),dim=2)[:,:,-self.window:,:]
        scores=torch.einsum("bhd,bhtd->bht",q,keys)/math.sqrt(self.k_dim)
        t=keys.shape[2]; distance=torch.arange(t-1,-1,-1,device=x.device,dtype=x.dtype)
        scores=scores-F.softplus(self.recency_log_slope).view(1,self.n_heads,1)*distance.view(1,1,t)
        probs=torch.softmax(scores.float(),dim=-1).to(x.dtype)
        mixed=torch.einsum("bht,bhtd->bhd",probs,values).reshape(b,self.n_heads*self.v_dim)
        return self.out(mixed)*torch.sigmoid(self.gate(z)), AttentionCache(keys,values)


class TinyGLUExpert(nn.Module):
    def __init__(self, d_latent: int, d_ff: int):
        super().__init__(); self.g=nn.Linear(d_latent,d_ff,bias=False); self.u=nn.Linear(d_latent,d_ff,bias=False); self.down=nn.Linear(d_ff,d_latent,bias=False)
    def forward(self,x): return self.down(F.silu(self.g(x))*self.u(x))


class SparseLatentExperts(nn.Module):
    """Compact shared micro-expert + exactly one top-1 routed latent expert per token."""
    def __init__(self, d_model: int, d_latent: int, n_experts: int, d_ff: int, shared_ff: int | None = None):
        super().__init__(); self.n_experts=n_experts; self.shared_ff=shared_ff or d_ff
        self.norm=RMSNorm(d_model); self.down=nn.Linear(d_model,d_latent,bias=False)
        self.shared=TinyGLUExpert(d_latent,self.shared_ff)
        self.experts=nn.ModuleList(TinyGLUExpert(d_latent,d_ff) for _ in range(n_experts))
        self.router=nn.Linear(d_latent,n_experts,bias=True); self.up=nn.Linear(d_latent,d_model,bias=False)
    def forward(self,x):
        z=self.down(self.norm(x)); probs=torch.softmax(self.router(z).float(),dim=-1).to(z.dtype); chosen=probs.argmax(dim=-1)
        routed=torch.zeros_like(z)
        for idx,expert in enumerate(self.experts):
            mask=chosen==idx
            if mask.any(): routed[mask]=expert(z[mask])*probs[mask,idx:idx+1]
        out=self.up(self.shared(z)+routed)
        mean_prob=probs.mean(dim=0); target=torch.full_like(mean_prob,1.0/self.n_experts)
        return out,(mean_prob-target).pow(2).mean()


class AnchorMixer(nn.Module):
    """Cheap cross-depth residual preservation with one block anchor."""
    def __init__(self,d_model:int):
        super().__init__(); self.logit=nn.Parameter(torch.full((d_model,),1.4))
    def forward(self,current,anchor):
        a=torch.sigmoid(self.logit); return a*current+(1.0-a)*anchor
