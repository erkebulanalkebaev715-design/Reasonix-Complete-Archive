import tempfile, unittest
from pathlib import Path
import torch
from reasonix_model import ReasonixLM, smoke_config
from reasonix_model.native_export_v6 import export_rxm6, q4_groupwise, deq_q4
class V6Tests(unittest.TestCase):
    def test_q4_roundtrip_bounded(self):
        torch.manual_seed(4); w=torch.randn(7,45)*0.1; p,s=q4_groupwise(w); d=deq_q4(p,s,45)
        self.assertEqual(tuple(d.shape),tuple(w.shape)); self.assertLess(float((d-w).abs().mean()),0.02)
    def test_magic_and_mixed_smaller_than_int8(self):
        torch.manual_seed(5); m=ReasonixLM(smoke_config()).eval()
        with tempfile.TemporaryDirectory() as td:
            a=Path(td)/'a.rxm6'; b=Path(td)/'b.rxm6'; ma=export_rxm6(m,a,'int8'); mb=export_rxm6(m,b,'mixed')
            self.assertEqual(a.read_bytes()[:8],b'RXM6MMAP'); self.assertLess(mb['bytes'],ma['bytes'])
