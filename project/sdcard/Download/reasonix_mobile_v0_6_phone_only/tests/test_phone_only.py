from pathlib import Path
import unittest
class PhoneOnlyTests(unittest.TestCase):
    def test_phone_script_has_no_pc_dependency(self):
        s=Path('scripts/phone_one_tap.sh').read_text()
        for bad in ('adb ','windows','powershell','cmake --build'):
            self.assertNotIn(bad.lower(),s.lower())
        self.assertIn('pkg install',s)
