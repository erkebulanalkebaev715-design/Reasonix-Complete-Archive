import unittest
import torch
from reasonix_model.layers import SparseLatentExperts

class ExpertTests(unittest.TestCase):
    def test_sparse_expert_output(self):
        torch.manual_seed(0); m=SparseLatentExperts(32,16,4,32)
        x=torch.randn(9,32); y,bal=m(x)
        self.assertEqual(y.shape,x.shape); self.assertTrue(torch.isfinite(y).all()); self.assertGreaterEqual(float(bal),0.0)
