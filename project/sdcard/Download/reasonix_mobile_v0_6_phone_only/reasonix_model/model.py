from __future__ import annotations
from dataclasses import dataclass
import torch
from torch import nn
import torch.nn.functional as F
from .config import ReasonixConfig
from .layers import RMSNorm, SelectivePocketState, WindowLatentAttention, SparseLatentExperts, AnchorMixer, StateCache, AttentionCache


@dataclass
class LayerCache:
    state: StateCache
    attention: AttentionCache | None

@dataclass
class ReasonixCache:
    layers: list[LayerCache]


class ReasonixLayer(nn.Module):
    def __init__(self,cfg:ReasonixConfig,index:int):
        super().__init__(); self.index=index
        self.state=SelectivePocketState(cfg.d_model,cfg.d_state)
        self.attention=None
        if (index+1)%cfg.attn_every==0:
            self.attention=WindowLatentAttention(cfg.d_model,cfg.n_heads,cfg.attn_head_dim,cfg.attn_value_dim,cfg.window_size)
        self.experts=SparseLatentExperts(cfg.d_model,cfg.d_latent,cfg.n_experts,cfg.expert_ff,shared_ff=(cfg.shared_expert_ff or cfg.expert_ff))
        self.anchor=AnchorMixer(cfg.d_model) if (index+1)%cfg.anchor_interval==0 else None
    def init_cache(self,batch,device,dtype):
        return LayerCache(self.state.init_cache(batch,device,dtype), self.attention.init_cache(batch,device,dtype) if self.attention else None)
    def step(self,x,cache,anchor):
        y,st=self.state.step(x,cache.state); x=x+y; at=cache.attention
        if self.attention is not None:
            y,at=self.attention.step(x,at); x=x+y
        y,balance=self.experts(x); x=x+y
        if self.anchor is not None: x=self.anchor(x,anchor)
        return x,LayerCache(st,at),balance


class ReasonixLM(nn.Module):
    """From-scratch autoregressive model. No external model weights or tokenizer."""
    def __init__(self,cfg:ReasonixConfig):
        super().__init__(); cfg.validate(); self.cfg=cfg
        self.embed=nn.Embedding(cfg.vocab_size,cfg.d_model)
        self.layers=nn.ModuleList(ReasonixLayer(cfg,i) for i in range(cfg.n_layers))
        self.final_norm=RMSNorm(cfg.d_model); self.lm_head=nn.Linear(cfg.d_model,cfg.vocab_size,bias=False)
        if cfg.tie_embeddings: self.lm_head.weight=self.embed.weight
        self.apply(self._init_weights)
        for layer in self.layers:
            nn.init.constant_(layer.state.to_keep.bias,1.5); nn.init.constant_(layer.state.to_gate.bias,1.0)
            if layer.attention is not None: nn.init.constant_(layer.attention.gate.bias,0.5)
    @staticmethod
    def _init_weights(m):
        if isinstance(m,nn.Linear):
            nn.init.normal_(m.weight,mean=0.0,std=0.02)
            if m.bias is not None: nn.init.zeros_(m.bias)
        elif isinstance(m,nn.Embedding): nn.init.normal_(m.weight,mean=0.0,std=0.02)
    def init_cache(self,batch,device=None,dtype=None):
        p=next(self.parameters()); device=device or p.device; dtype=dtype or p.dtype
        return ReasonixCache([l.init_cache(batch,device,dtype) for l in self.layers])
    def step(self,token,cache,mode="deep"):
        depth=self.cfg.depth_for_mode(mode); x=self.embed(token); anchor=x; new=[]; aux=x.new_zeros(())
        for i,layer in enumerate(self.layers):
            if i<depth:
                x,lc,bal=layer.step(x,cache.layers[i],anchor); aux=aux+bal
                if layer.anchor is not None: anchor=x
                new.append(lc)
            else: new.append(cache.layers[i])
        return self.lm_head(self.final_norm(x)),ReasonixCache(new),aux/max(1,depth)
    def forward(self,tokens,mode="deep"):
        if tokens.ndim!=2: raise ValueError("tokens must have shape [batch, seq]")
        b,t=tokens.shape
        if t==0: raise ValueError("sequence must not be empty")
        cache=self.init_cache(b,tokens.device,self.embed.weight.dtype); logits=[]; aux=self.embed.weight.new_zeros(())
        for pos in range(t):
            l,cache,bal=self.step(tokens[:,pos],cache,mode); logits.append(l); aux=aux+bal
        return torch.stack(logits,dim=1),aux/t
    @torch.no_grad()
    def generate(self,prompt,max_new_tokens=32,mode="standard",temperature=0.8):
        if not prompt: raise ValueError("prompt must not be empty")
        device=next(self.parameters()).device; cache=self.init_cache(1,device=device); logits=None
        for tok in prompt: logits,cache,_=self.step(torch.tensor([tok],dtype=torch.long,device=device),cache,mode)
        out=list(prompt)
        for _ in range(max_new_tokens):
            if temperature<=0: nxt=int(logits.argmax(dim=-1).item())
            else: nxt=int(torch.multinomial(torch.softmax(logits.float()/temperature,dim=-1),1).item())
            out.append(nxt); logits,cache,_=self.step(torch.tensor([nxt],dtype=torch.long,device=device),cache,mode)
        return out
    def loss(self,tokens,mode="deep",balance_weight=0.01):
        if tokens.shape[1]<2: raise ValueError("need sequence length >= 2")
        logits,aux=self.forward(tokens[:,:-1],mode); target=tokens[:,1:]
        return F.cross_entropy(logits.reshape(-1,logits.shape[-1]),target.reshape(-1))+balance_weight*aux
