import unittest
from reasonix_model import ReasonixLM, smoke_config
from reasonix_model.metrics import estimate_parameters

class MetricTests(unittest.TestCase):
    def test_estimator_matches_smoke_model(self):
        cfg=smoke_config(); m=ReasonixLM(cfg)
        actual=sum(p.numel() for p in m.parameters())
        est=estimate_parameters(cfg)["parameters"]
        self.assertEqual(actual,est)
