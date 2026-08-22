from __future__ import annotations
import copy, json, math, struct
from pathlib import Path
from typing import Literal

import torch
from torch import nn

from .config import smoke_config, mobile_s_config
from .model import ReasonixLM
from .quant import quantize_int8_per_row

MAGIC=b"RXM6MMAP"
VERSION=1
K_F32=0
K_I8=1
K_I4=2
ALIGN=64
GROUP=32


def _u16(f,v): f.write(struct.pack('<H',int(v)))
def _u32(f,v): f.write(struct.pack('<I',int(v)))
def _u64(f,v): f.write(struct.pack('<Q',int(v)))
def _f32(f,v): f.write(struct.pack('<f',float(v)))
def _align(f,a=ALIGN):
    pad=(-f.tell())%a
    if pad: f.write(b'\0'*pad)


def q4_groupwise(w: torch.Tensor, group_size:int=GROUP):
    if w.ndim!=2: raise ValueError('q4 only supports matrices')
    w=w.detach().float().cpu().contiguous(); rows,cols=w.shape
    ng=(cols+group_size-1)//group_size
    scales=torch.empty(rows,ng,dtype=torch.float32)
    q=torch.empty(rows,cols,dtype=torch.int8)
    for r in range(rows):
        for g in range(ng):
            s=g*group_size; e=min(cols,s+group_size)
            block=w[r,s:e]
            scale=max(float(block.abs().max().item())/7.0,1e-8)
            scales[r,g]=scale
            q[r,s:e]=torch.clamp(torch.round(block/scale),-7,7).to(torch.int8)
    # 2 signed 4-bit values per byte, encoded as two's complement nibble.
    packed=torch.zeros(rows,(cols+1)//2,dtype=torch.uint8)
    for c in range(cols):
        nib=(q[:,c].to(torch.int16)&0x0F).to(torch.uint8)
        if c&1: packed[:,c//2]|=(nib<<4)
        else: packed[:,c//2]|=nib
    return packed.contiguous(),scales.contiguous()


def deq_q4(packed:torch.Tensor,scales:torch.Tensor,cols:int,group_size:int=GROUP):
    rows=packed.shape[0]; out=torch.empty(rows,cols,dtype=torch.float32)
    for c in range(cols):
        b=packed[:,c//2]
        nib=((b>>4)&15) if c&1 else (b&15)
        qi=nib.to(torch.int16); qi=torch.where(qi>=8,qi-16,qi).float()
        out[:,c]=qi*scales[:,c//group_size]
    return out


def _use_i4(name:str,policy:str)->bool:
    if policy!='mixed': return False
    # Keep state/attention/router/up and LM head in INT8. Compress only expert MLP matrices.
    return '.experts.shared.' in name or '.experts.experts.' in name


class DynamicMixedLinear(nn.Module):
    def __init__(self, src:nn.Linear, name:str, policy:str):
        super().__init__(); self.in_features=src.in_features; self.out_features=src.out_features
        self.kind='i4' if _use_i4(name,policy) else 'i8'
        if self.kind=='i4':
            p,s=q4_groupwise(src.weight)
            self.register_buffer('q4',p); self.register_buffer('q4_scales',s)
        else:
            qt=quantize_int8_per_row(src.weight)
            self.register_buffer('q8',qt.q.contiguous()); self.register_buffer('q8_scales',qt.scale.contiguous())
        if src.bias is None: self.bias=None
        else: self.register_buffer('bias',src.bias.detach().float().cpu().contiguous())

    def forward(self,x:torch.Tensor)->torch.Tensor:
        xf=x.float()
        max_abs=xf.abs().amax(dim=-1,keepdim=True)
        xs=torch.where(max_abs>1e-12,max_abs/127.0,torch.ones_like(max_abs))
        qx=torch.clamp(torch.round(xf/xs),-127,127).to(torch.int32)
        if self.kind=='i8':
            acc=torch.matmul(qx,self.q8.to(torch.int32).t()).float()
            y=acc*xs*self.q8_scales
        else:
            w=deq_q4(self.q4,self.q4_scales,self.in_features).to(xf.device)
            y=torch.matmul(xf,w.t())
        if self.bias is not None: y=y+self.bias
        return y.to(x.dtype)


def make_quant_reference(model:ReasonixLM,policy:Literal['int8','mixed']='mixed')->ReasonixLM:
    q=copy.deepcopy(model).cpu().eval()
    def rec(mod:nn.Module,prefix=''):
        for name,child in list(mod.named_children()):
            full=f'{prefix}.{name}' if prefix else name
            if isinstance(child,nn.Linear): setattr(mod,name,DynamicMixedLinear(child,full,policy))
            else: rec(child,full)
    rec(q)
    return q


def export_rxm6(model:ReasonixLM,path:str|Path,policy:Literal['int8','mixed']='mixed')->dict:
    p=Path(path); p.parent.mkdir(parents=True,exist_ok=True)
    cfg=model.cfg; sd=model.state_dict(); meta=[]
    with p.open('wb') as f:
        f.write(MAGIC); _u32(f,VERSION)
        ints=[cfg.vocab_size,cfg.d_model,cfg.n_layers,cfg.d_state,cfg.d_latent,cfg.n_experts,cfg.expert_ff,(cfg.shared_expert_ff or cfg.expert_ff),cfg.attn_every,cfg.n_heads,cfg.attn_head_dim,cfg.attn_value_dim,cfg.window_size,cfg.anchor_interval]
        for v in ints:_u32(f,v)
        _f32(f,cfg.fast_depth_fraction);_f32(f,cfg.standard_depth_fraction);_u32(f,len(sd))
        for name,tensor in sd.items():
            t=tensor.detach().float().cpu().contiguous(); raw=name.encode(); _u16(f,len(raw)); f.write(raw)
            if t.ndim==2 and name!='embed.weight': kind=K_I4 if _use_i4(name,policy) else K_I8
            else: kind=K_F32
            f.write(struct.pack('<BB',kind,t.ndim))
            for d in t.shape:_u32(f,d)
            group=GROUP if kind==K_I4 else 0; _u32(f,group)
            if kind==K_F32:
                scale_count=0; payload=t.numpy().astype('<f4',copy=False).tobytes()
                scales=b''
            elif kind==K_I8:
                qt=quantize_int8_per_row(t); scales=qt.scale.numpy().astype('<f4',copy=False).tobytes(); payload=qt.q.numpy().astype('i1',copy=False).tobytes(); scale_count=qt.scale.numel()
            else:
                packed,s=q4_groupwise(t,GROUP); scales=s.numpy().astype('<f4',copy=False).tobytes(); payload=packed.numpy().astype('u1',copy=False).tobytes(); scale_count=s.numel()
            _u32(f,scale_count); _u64(f,len(payload)); _align(f); scale_off=f.tell(); f.write(scales); _align(f); data_off=f.tell(); f.write(payload); _align(f)
            meta.append({'name':name,'kind':('f32','int8_row','int4_group')[kind],'shape':list(t.shape),'group_size':group,'scale_offset':scale_off,'data_offset':data_off})
    return {'format':'reasonix-native-rxm6-v1','policy':policy,'path':str(p),'bytes':p.stat().st_size,'tensors':meta,'config':cfg.as_dict()}


@torch.no_grad()
def _reference(model:ReasonixLM,policy:str,prompt:list[int]):
    q=make_quant_reference(model,policy); modes={}
    for mode in ('fast','standard','deep'):
        c=q.init_cache(1); logits=None
        for tok in prompt: logits,c,_=q.step(torch.tensor([tok]),c,mode)
        greedy=[]
        for _ in range(8):
            nxt=int(logits.argmax(-1).item()); greedy.append(nxt); logits,c,_=q.step(torch.tensor([nxt]),c,mode)
        modes[mode]={'greedy':greedy,'last_logits':logits[0].float().tolist()}
    return modes


def build_fixtures(out_dir:str|Path,seed:int=20260818)->dict:
    out=Path(out_dir);out.mkdir(parents=True,exist_ok=True); torch.manual_seed(seed)
    m=ReasonixLM(smoke_config()).eval(); prompt=[82,101,97,115,111,110,105,120]
    manifest={'seed':seed,'prompt':prompt,'variants':{}}
    for policy in ('int8','mixed'):
        p=out/f'smoke_v06_{policy}.rxm6'; meta=export_rxm6(m,p,policy); modes=_reference(m,policy,prompt)
        manifest['variants'][policy]={'model':meta,'modes':modes}
    # Realistic graph-size benchmark only. Random weights: NEVER quality/intelligence evidence.
    torch.manual_seed(seed+1); ms=ReasonixLM(mobile_s_config()).eval(); mp=out/'mobile_s_v06_mixed_BENCH_ONLY.rxm6'; mm=export_rxm6(ms,mp,'mixed')
    manifest['mobile_s_bench_only']={'model':mm,'warning':'random weights; benchmark only; not an intelligent model'}
    (out/'native_reference_v06.json').write_text(json.dumps(manifest),encoding='utf-8')
    return manifest

if __name__=='__main__':
    import argparse
    ap=argparse.ArgumentParser();ap.add_argument('--out-dir',default='results/native_fixture');a=ap.parse_args()
    r=build_fixtures(a.out_dir); print(json.dumps({'variants':{k:v['model']['bytes'] for k,v in r['variants'].items()},'mobile_s_bytes':r['mobile_s_bench_only']['model']['bytes']},indent=2))
