from __future__ import annotations

class ByteTokenizer:
    """Tokenizer owned by the project: raw UTF-8 bytes + four control tokens."""
    PAD = 256
    BOS = 257
    EOS = 258
    SEP = 259
    vocab_size = 260

    def encode(self, text: str, bos: bool = True, eos: bool = False) -> list[int]:
        ids = list(text.encode("utf-8", errors="strict"))
        if bos:
            ids.insert(0, self.BOS)
        if eos:
            ids.append(self.EOS)
        return ids

    def decode(self, ids: list[int]) -> str:
        raw = bytes(i for i in ids if 0 <= int(i) < 256)
        return raw.decode("utf-8", errors="replace")
