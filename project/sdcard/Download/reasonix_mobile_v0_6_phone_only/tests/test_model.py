import unittest
import torch
from reasonix_model import ReasonixLM, smoke_config

class ModelTests(unittest.TestCase):
    def setUp(self):
        torch.manual_seed(0); self.m = ReasonixLM(smoke_config()).eval()
    def test_forward_shapes_all_modes(self):
        x = torch.randint(0,260,(2,7))
        for mode in ("fast","standard","deep"):
            y, aux = self.m(x, mode=mode)
            self.assertEqual(tuple(y.shape),(2,7,260)); self.assertTrue(torch.isfinite(y).all()); self.assertTrue(torch.isfinite(aux))
    def test_incremental_matches_full_last_logit(self):
        x = torch.randint(0,260,(1,8))
        full,_ = self.m(x, mode="deep")
        cache = self.m.init_cache(1); last=None
        for i in range(x.shape[1]): last, cache,_ = self.m.step(x[:,i], cache, mode="deep")
        self.assertTrue(torch.allclose(full[:,-1], last, atol=1e-5, rtol=1e-4))
    def test_cache_is_window_bounded(self):
        cfg = smoke_config(); cache=self.m.init_cache(1)
        for _ in range(cfg.window_size+10):
            _,cache,_=self.m.step(torch.tensor([65]),cache,mode="deep")
        for lc in cache.layers:
            if lc.attention is not None: self.assertLessEqual(lc.attention.keys.shape[2], cfg.window_size)
    def test_adaptive_depth(self):
        c=smoke_config(); self.assertLess(c.depth_for_mode("fast"),c.depth_for_mode("deep"))
