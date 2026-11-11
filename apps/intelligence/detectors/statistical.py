"""Transparent statistical detectors: rolling z-score, threshold, trend (memory growth)."""
from __future__ import annotations
from dataclasses import dataclass
import numpy as np


@dataclass
class Detection:
    detector: str
    triggered: bool
    severity: str  # info|warning|critical
    evidence: str
    score: float


def threshold(values, limit: float, name: str = "metric") -> Detection:
    if not values:
        return Detection("threshold", False, "info", f"no {name} samples", 0.0)
    latest = float(values[-1])
    hit = latest > limit
    sev = "critical" if latest > limit * 1.15 else "warning"
    return Detection("threshold", hit, sev if hit else "info",
                     f"{name} latest={latest:.1f} limit={limit:.1f}", latest / limit if limit else 0.0)


def zscore_spike(values, window: int = 30, z: float = 3.0, name: str = "metric") -> Detection:
    arr = np.asarray(values, dtype=float)
    if arr.size < max(8, window // 3):
        return Detection("zscore", False, "info", f"insufficient {name} history ({arr.size})", 0.0)
    base, latest = arr[-window - 1:-1], arr[-1]
    mu, sd = base.mean(), base.std()
    if sd < 1e-9:
        sd = max(abs(mu) * 0.01, 1e-6)
    score = float((latest - mu) / sd)
    hit = score >= z
    return Detection("zscore", hit, "critical" if score >= z * 2 else "warning" if hit else "info",
                     f"{name} latest={latest:.1f} mean={mu:.1f} sd={sd:.2f} z={score:.1f}", score)


def gradual_growth(values, min_slope: float = 0.2, name: str = "memory") -> Detection:
    """Least-squares slope per sample; flags sustained growth (possible leak, not confirmed)."""
    arr = np.asarray(values, dtype=float)
    if arr.size < 10:
        return Detection("trend", False, "info", f"insufficient {name} history ({arr.size})", 0.0)
    x = np.arange(arr.size)
    slope, _ = np.polyfit(x, arr, 1)
    r = np.corrcoef(x, arr)[0, 1]
    hit = slope >= min_slope and r > 0.9
    return Detection("trend", hit, "warning" if hit else "info",
                     f"{name} slope={slope:.2f}/sample r={r:.2f}", float(slope))


def consecutive_failures(statuses, limit: int = 3) -> Detection:
    n = 0
    for ok in reversed(statuses):
        if ok:
            break
        n += 1
    hit = n >= limit
    return Detection("consecutive", hit, "critical" if hit else "info",
                     f"{n} consecutive failed health checks (limit {limit})", float(n))
