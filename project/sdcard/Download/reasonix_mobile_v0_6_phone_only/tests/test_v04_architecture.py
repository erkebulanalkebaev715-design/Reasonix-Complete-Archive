import unittest
import torch
from reasonix_model import ReasonixLM, smoke_config, v03_smoke_config, mobile_s_config, mobile_m_config
from reasonix_model.layers import SelectivePocketState, AnchorMixer

class V04ArchitectureTests(unittest.TestCase):
    def test_v04_retains_only_ablation_winner(self):
        c=smoke_config(); self.assertEqual(c.shared_expert_ff,c.expert_ff//2); self.assertEqual(c.attn_every,4)
    def test_mobile_profiles_do_not_contain_rejected_mutations(self):
        for c in (mobile_s_config(),mobile_m_config()):
            self.assertEqual(c.shared_expert_ff,c.expert_ff//2); self.assertEqual(c.attn_every,4)
            m=ReasonixLM(c.with_ablation(d_model=64,n_layers=2,d_state=32,d_latent=32,n_experts=2,expert_ff=64,shared_expert_ff=32,n_heads=4,attn_head_dim=16,attn_value_dim=16,window_size=16,anchor_interval=2))
            self.assertTrue(all(isinstance(l.state,SelectivePocketState) for l in m.layers))
            self.assertTrue(all(l.anchor is None or isinstance(l.anchor,AnchorMixer) for l in m.layers))
    def test_v03_control(self):
        c=v03_smoke_config(); self.assertEqual(c.shared_expert_ff,0); self.assertEqual(c.attn_every,2)
    def test_long_decode_stays_finite(self):
        m=ReasonixLM(smoke_config()).eval(); cache=m.init_cache(1)
        with torch.no_grad():
            for i in range(128): logits,cache,aux=m.step(torch.tensor([i%256]),cache,"deep")
        self.assertTrue(torch.isfinite(logits).all()); self.assertTrue(torch.isfinite(aux))
