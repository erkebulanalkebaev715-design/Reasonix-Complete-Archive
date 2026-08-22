import unittest
from reasonix_model.tokenizer import ByteTokenizer

class TokenizerTests(unittest.TestCase):
    def test_unicode_roundtrip(self):
        t = ByteTokenizer(); s = "Привет, мир! 🤖"
        self.assertEqual(t.decode(t.encode(s, bos=False)), s)
    def test_control_tokens_do_not_decode(self):
        t = ByteTokenizer(); self.assertEqual(t.decode([t.BOS,65,t.EOS]), "A")
