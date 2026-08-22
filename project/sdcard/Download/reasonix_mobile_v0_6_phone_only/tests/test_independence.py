import unittest
from pathlib import Path

class IndependenceTests(unittest.TestCase):
    def test_runtime_has_no_third_party_model_loader(self):
        root=Path(__file__).resolve().parents[1]/"reasonix_model"
        text="\n".join(p.read_text(encoding="utf-8") for p in root.glob("*.py"))
        forbidden=("transformers", "AutoModel", "AutoTokenizer", "llama_cpp", "GGUF", "from_pretrained")
        for term in forbidden:
            self.assertNotIn(term, text, term)
