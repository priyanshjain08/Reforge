# REFORGE

**Autonomous self-healing for software and infrastructure.** REFORGE monitors services, detects failures, explains the likely cause, applies a *policy-approved* fix (for example restarting a crashed container), then **verifies** the service is really healthy again. Every step is audited.

```
MONITOR -> DETECT -> DIAGNOSE -> DECIDE -> REMEDIATE -> VERIFY -> AUDIT
```

## What is in the box

| Part | Tech | What it does |
|---|---|---|
| `apps/core` | Go (standard library only) | API, health monitoring, incident detection, policy engine, recovery orchestration, audit log, reports (JSON/PDF/CSV) |
| `apps/agent` | Go (standard library only) | Reports host metrics from `/proc`, inspects Docker containers, executes only allowlisted restart commands |
| `apps/intelligence` | Python, FastAPI | Statistical detectors (z-score, trend, threshold) and rule-based diagnosis with evidence and confidence |
| `apps/dashboard` | React, TypeScript, Vite, Tailwind, Recharts, React Flow | 12-page operations dashboard |
| `infrastructure/chaos-lab` | Go | Demo services you can safely break (crash, 503s, errors, slowness, CPU, memory) |

## Run it (Windows, no coding needed)

1. Install **Docker Desktop** (https://www.docker.com/products/docker-desktop) and start it.
2. Open **PowerShell** in this folder.
3. Create your secrets (generates random ones and prints your login token):
   ```powershell
   .\scripts\setup.ps1
   ```
   (If PowerShell blocks scripts: `Set-ExecutionPolicy -Scope Process Bypass` first.)
4. Start everything:
   ```powershell
   docker compose up --build
   ```
   The first build takes a few minutes.
5. Open **http://localhost:3000** and paste the token from step 3 (it is also in `.env` as `REFORGE_ADMIN_TOKEN`).

macOS/Linux: use `./scripts/setup.sh` instead of step 3.

### Try the demo
1. Open **Chaos Lab**, pick `payment-service` and **Process crash**, click **Inject failure**.
2. Watch **Incidents**: REFORGE detects it (about 10 s), diagnoses it (container observed `exited`, so *confirmed*), and proposes `restart_container`.
3. Default mode is `approval`, so open **Recovery Operations** and click **Approve**. The agent restarts the container, REFORGE verifies health, and the incident resolves.
4. Want it fully automatic? Set `REFORGE_RECOVERY_MODE=auto` in `.env` and run `docker compose up -d`, or switch the policy mode on the **Recovery Policies** page.
5. Try the **EMERGENCY STOP** button on Recovery Operations, then inject another failure: nothing is executed.

## Safety model (short version)
- Only two allowlisted actions exist: `restart_container` and `app_recovery_hook`. There is no arbitrary command execution anywhere.
- Each action needs a permission **and** an explicitly authorised target, checked by the core *and again by the agent*.
- Modes: `simulate` (never executes), `approval` (default), `auto`.
- Cooldowns, retry limits (then escalation), a global kill switch, and execution timeouts prevent restart loops.
- A recovery is only reported successful if post-action verification passes. Failed or unverified recoveries roll back (when defined) or escalate.
- Diagnoses are labelled **suspected** unless a direct observation confirms them, and always show their evidence.
- The audit log is append-only through the API. See `docs/security.md`.


## Honest status

**This code was written in an environment without Go, Docker, or internet, so most of it has not been compiled or run.** What was actually verified:

| Check | Result |
|---|---|
| Python detectors and diagnosis (`apps/intelligence`), 10 unit tests | Ran and passed |
| Dashboard TypeScript/TSX files | Syntax-checked only. Not type-checked, not built, tests not run |
| Go core, agent, demo service and their tests | **Not compiled, not run** |
| Docker Compose stack, Dockerfiles, end-to-end demo | **Never started** |

Expect to fix some compile or type errors on first run. The GitHub CI workflow will show them. After the first successful `npm install`, commit the generated `package-lock.json`.

## Differences from the original spec (deliberate or forced)
- **Storage:** the core persists to a JSON file (`/data/state.json`) instead of PostgreSQL/Redis, so there is nothing to install or configure. `database/migrations/0001_init.sql` is the planned Postgres schema and is **not wired in**.
- **Realtime:** Server-Sent Events instead of WebSockets (simpler, no dependency).
- **Auth:** static bearer tokens with two roles (admin, viewer) instead of user accounts, Argon2id and refresh tokens.
- **Dashboard UI:** hand-written Tailwind components, not shadcn/ui. No Playwright end-to-end tests.
- **Agent:** runs inside Docker (Linux metrics only). On Windows/macOS hosts unsupported metrics are reported as unsupported, never invented. The agent talks to Docker through the mounted socket, which is powerful; see `docs/security.md`.
- **Detection:** the core opens incidents from consecutive failed health checks, slow responses and host CPU/memory/disk thresholds. The Python anomaly detectors (`/v1/detect`) exist and are tested but are not yet called by the core.
- **Chaos lab:** crash, health failure, error spike, slow response, CPU burn, memory pressure. Not implemented: network latency/packet loss, database outage, repeated-crash loop, pause/resume of an experiment.
- **"Manual recovery" comparison** uses an adjustable *estimate* (`REFORGE_MANUAL_BASELINE_S`) and is labelled as such.
- **Not built:** user-defined alert channels (email/webhook), a learning/feedback stage, multi-tenant support.
