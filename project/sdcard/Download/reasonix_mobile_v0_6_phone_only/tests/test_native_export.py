import tempfile
import unittest
from pathlib import Path
import torch
from reasonix_model import ReasonixLM, smoke_config
from reasonix_model.native_export import export_rxm5, make_quant_reference

class NativeExportTests(unittest.TestCase):
    def test_export_has_project_magic(self):
        torch.manual_seed(1); m=ReasonixLM(smoke_config()).eval()
        with tempfile.TemporaryDirectory() as td:
            p=Path(td)/"m.rxm"; meta=export_rxm5(m,p)
            self.assertEqual(p.read_bytes()[:8], b"RXM5BIN\0")
            self.assertEqual(meta["format"], "reasonix-native-rxm5-v1")
    def test_quant_reference_runs(self):
        torch.manual_seed(1); q=make_quant_reference(ReasonixLM(smoke_config()).eval())
        c=q.init_cache(1); logits,c,_=q.step(torch.tensor([65]),c,"deep")
        self.assertEqual(tuple(logits.shape),(1,260)); self.assertTrue(torch.isfinite(logits).all())
