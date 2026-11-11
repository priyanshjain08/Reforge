"""Rule-based diagnosis. Confidence is derived from the amount of matched evidence, not a model guess.
A cause is 'confirmed' only when a rule matches a direct observation (e.g. container state == exited);
otherwise it is 'suspected'."""
from __future__ import annotations
from dataclasses import dataclass, field


@dataclass
class Diagnosis:
    probable_cause: str
    affected_component: str
    status: str  # "suspected" | "confirmed"
    evidence: list[str]
    confidence: float
    recommended_action: str
    alternatives: list[str] = field(default_factory=list)
    explanation: str = ""


def diagnose(ctx: dict) -> Diagnosis:
    svc = ctx.get("service", "unknown")
    ev: list[str] = []
    hc_fail = int(ctx.get("consecutive_failures", 0))
    cstate = ctx.get("container_state")
    restarts = int(ctx.get("container_restarts", 0))
    cpu, mem = ctx.get("cpu"), ctx.get("memory")
    latency_z = ctx.get("latency_z", 0)
    err_rate = ctx.get("error_rate", 0)

    if hc_fail >= 3:
        ev.append(f"{hc_fail} consecutive failed health checks")

    if cstate in ("exited", "dead"):
        ev.append(f"container state observed: {cstate}")
        if ctx.get("exit_code") not in (None, 0):
            ev.append(f"exit code {ctx['exit_code']}")
        conf = min(0.95, 0.5 + 0.15 * len(ev))
        return Diagnosis(f"{svc} process terminated", svc, "confirmed", ev, round(conf, 2),
                         "restart_container",
                         ["host resource exhaustion (OOM kill)"] if mem and mem > 90 else [],
                         "Container is observed stopped; failing health checks are consistent with it.")
    if restarts >= 3:
        ev.append(f"{restarts} container restarts in window")
        return Diagnosis(f"{svc} crash loop", svc, "suspected", ev, round(min(0.8, 0.4 + 0.1 * len(ev)), 2),
                         "escalate", ["bad configuration", "failing dependency"],
                         "Repeated restarts: another automatic restart is unlikely to help; escalate.")
    if cpu is not None and cpu > 90:
        ev.append(f"CPU {cpu:.0f}%")
        return Diagnosis(f"{svc} CPU saturation", svc, "suspected", ev, round(min(0.75, 0.35 + 0.1 * len(ev)), 2),
                         "restart_container", ["traffic surge", "runaway loop"],
                         "High CPU without a stopped process; cause not confirmed.")
    if mem is not None and mem > 90:
        ev.append(f"memory {mem:.0f}%")
        return Diagnosis(f"{svc} memory pressure", svc, "suspected", ev, round(min(0.7, 0.35 + 0.1 * len(ev)), 2),
                         "restart_container", ["memory leak", "undersized limit"], "Memory high; a leak is unproven.")
    if hc_fail >= 3:
        if err_rate and err_rate > 0.5:
            ev.append(f"error rate {err_rate:.0%}")
        if latency_z >= 3:
            ev.append(f"latency z-score {latency_z:.1f}")
        return Diagnosis(f"{svc} application unhealthy", svc, "suspected", ev,
                         round(min(0.65, 0.3 + 0.1 * len(ev)), 2), "app_recovery_hook",
                         ["dependency outage", "network fault"], "Health fails but the process appears to be running.")
    return Diagnosis("no failure identified", svc, "suspected", ev or ["insufficient evidence"], 0.1,
                     "none", [], "Evidence does not support a diagnosis.")
