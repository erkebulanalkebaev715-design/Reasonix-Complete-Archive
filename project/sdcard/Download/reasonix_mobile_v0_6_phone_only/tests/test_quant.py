import tempfile, unittest
import torch
from reasonix_model.quant import quantize_int8_per_row, dequantize_int8_per_row, export_int8
from reasonix_model import ReasonixLM, smoke_config

class QuantTests(unittest.TestCase):
    def test_row_quant_error_is_bounded(self):
        torch.manual_seed(0); w=torch.randn(16,32)*0.2
        q=quantize_int8_per_row(w); r=dequantize_int8_per_row(q)
        self.assertLess(float((w-r).abs().max()),0.01)
    def test_export(self):
        with tempfile.TemporaryDirectory() as td:
            man=export_int8(ReasonixLM(smoke_config()),td)
            self.assertEqual(man["format"],"reasonix-int8-row-v1")
