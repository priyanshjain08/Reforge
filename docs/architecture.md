# Architecture

```
 dashboard (React) --HTTP/SSE--> core (Go) --HTTP--> intelligence (Python)
                                   ^  |  \--HTTP--> demo services (health, hook, chaos)
                      telemetry,   |  | commands
                      results      |  v
                                 agent (Go) --Docker socket--> containers
```

- **Core** probes registered services, tracks consecutive failures, opens one incident per `dedup_key`, asks the intelligence service for a diagnosis, matches policies, and runs the recovery engine (`apps/core/internal/recovery`).
- **Recovery engine** lifecycle: validate -> authorise -> loop guards (cooldown, retry limit) -> mode gate (simulate/approval/auto) -> execute -> verify -> rollback or escalate.
- **Agent** polls the core for commands every second and posts telemetry every 5 seconds. It re-checks every command against its own local allowlist before touching Docker.
- **Intelligence** is stateless: `POST /v1/diagnose`, `POST /v1/detect`.
- **State** is held in memory and flushed to `/data/state.json` every few seconds.

Incident states: open -> investigating -> recovering -> resolved | escalated.
Operation states: validating -> awaiting_approval -> executing -> verifying -> successful | failed | rolled_back | escalated | cancelled.
