import unittest
from reasonix_model.memory_budget import parse_meminfo, choose_budget


class MemoryBudgetTests(unittest.TestCase):
    def test_parse_and_budget(self):
        total, avail = parse_meminfo("MemTotal: 6291456 kB\nMemAvailable: 3145728 kB\n")
        self.assertEqual(total, 6144)
        self.assertEqual(avail, 3072)
        b = choose_budget(total, avail)
        self.assertLess(b.safe_process_budget_mb, avail)
        self.assertGreaterEqual(b.android_reserve_mb, 1400)

    def test_low_available_memory_tightens_budget(self):
        hi = choose_budget(6144, 4000)
        lo = choose_budget(6144, 1200)
        self.assertLess(lo.safe_process_budget_mb, hi.safe_process_budget_mb)
