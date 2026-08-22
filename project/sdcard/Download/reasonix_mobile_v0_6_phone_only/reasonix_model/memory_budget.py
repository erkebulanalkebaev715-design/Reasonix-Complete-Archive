from __future__ import annotations
from dataclasses import dataclass, asdict
from pathlib import Path


@dataclass(frozen=True)
class MemoryBudget:
    mem_total_mb: int
    mem_available_mb: int
    android_reserve_mb: int
    safe_process_budget_mb: int
    source: str

    def as_dict(self) -> dict:
        return asdict(self)


def parse_meminfo(text: str) -> tuple[int, int]:
    vals = {}
    for line in text.splitlines():
        if ":" not in line:
            continue
        k, rest = line.split(":", 1)
        parts = rest.strip().split()
        if parts and parts[0].isdigit():
            vals[k] = int(parts[0]) // 1024
    return vals.get("MemTotal", 0), vals.get("MemAvailable", 0)


def choose_budget(mem_total_mb: int, mem_available_mb: int) -> MemoryBudget:
    """Conservative starting heuristic; it must be calibrated by phone measurements.

    It never treats all physical RAM as model RAM. Available memory is the hard
    runtime signal; the 6 GB device profile is only the intended hardware class.
    """
    if mem_total_mb <= 0 or mem_available_mb <= 0:
        raise ValueError("positive memory values required")
    reserve = max(1400, int(mem_total_mb * 0.30))
    physical_cap = max(256, mem_total_mb - reserve)
    available_cap = max(256, int(mem_available_mb * 0.70))
    safe = min(physical_cap, available_cap)
    return MemoryBudget(mem_total_mb, mem_available_mb, reserve, safe, "heuristic-v0.4-until-phone-calibrated")


def read_local_budget(path: str = "/proc/meminfo") -> MemoryBudget:
    text = Path(path).read_text(encoding="utf-8")
    total, avail = parse_meminfo(text)
    return choose_budget(total, avail)
