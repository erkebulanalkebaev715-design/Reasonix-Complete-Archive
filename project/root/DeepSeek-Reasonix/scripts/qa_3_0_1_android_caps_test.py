#!/usr/bin/env python3
"""Tool-fabric availability truth tests for reasonix_android_tools capabilities.

ADB binary presence must never be reported as a connected or authorized device.
"""
import importlib.util
import unittest

spec = importlib.util.spec_from_file_location(
    'rat', '/root/reasonix-mobile-v1.5.1-backend/reasonix_android_tools.py')
rat = importlib.util.module_from_spec(spec)
spec.loader.exec_module(rat)


class AdbTruthTest(unittest.TestCase):
    def test_no_binary_reports_nothing_connected(self):
        rat.which = lambda _n: None
        rat.run = lambda *_a, **_k: {'exitCode': 127, 'stdout': '', 'stderr': ''}
        s = rat.adb_status()
        self.assertFalse(s['binary'])
        self.assertFalse(s['connected'])
        self.assertFalse(s['authorized'])

    def test_binary_but_no_devices(self):
        rat.which = lambda _n: '/usr/bin/adb'
        rat.run = lambda *_a, **_k: {'exitCode': 0, 'stdout': 'List of devices attached\n\n', 'stderr': ''}
        s = rat.adb_status()
        self.assertTrue(s['binary'])
        self.assertFalse(s['connected'])
        self.assertFalse(s['authorized'])

    def test_unauthorized_device_is_connected_not_authorized(self):
        rat.which = lambda _n: '/usr/bin/adb'
        rat.run = lambda *_a, **_k: {'exitCode': 0, 'stdout': 'List of devices attached\nABC123\tunauthorized\n', 'stderr': ''}
        s = rat.adb_status()
        self.assertTrue(s['connected'])
        self.assertFalse(s['authorized'])

    def test_authorized_device(self):
        rat.which = lambda _n: '/usr/bin/adb'
        rat.run = lambda *_a, **_k: {'exitCode': 0, 'stdout': 'List of devices attached\nABC123\tdevice\n', 'stderr': ''}
        s = rat.adb_status()
        self.assertTrue(s['connected'])
        self.assertTrue(s['authorized'])


class CapabilityPresenceTest(unittest.TestCase):
    def test_caps_shape(self):
        c = rat.caps()
        self.assertIn('adb', c)
        self.assertIn('termuxApi', c)
        self.assertIn('rootSu', c)
        self.assertIn('androidSystemShell', c)
        # availability must reflect real tooling state, not a hardcoded true
        self.assertIsInstance(c['termuxApi'], bool)


if __name__ == '__main__':
    unittest.main(verbosity=2)
