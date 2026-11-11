CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE users (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), email TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL, role TEXT NOT NULL CHECK (role IN ('admin','operator','viewer')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE agents (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), hostname TEXT NOT NULL, os TEXT, version TEXT,
  secret_hash TEXT NOT NULL, revoked BOOLEAN NOT NULL DEFAULT false, last_heartbeat TIMESTAMPTZ,
  capabilities JSONB NOT NULL DEFAULT '[]', created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE agent_permissions (agent_id UUID REFERENCES agents(id) ON DELETE CASCADE, permission TEXT NOT NULL,
  target TEXT NOT NULL, PRIMARY KEY (agent_id, permission, target));
CREATE TABLE services (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), name TEXT UNIQUE NOT NULL, kind TEXT NOT NULL,
  health_url TEXT, expected_status INT DEFAULT 200, timeout_ms INT DEFAULT 2000, interval_s INT DEFAULT 10,
  environment TEXT DEFAULT 'demo', agent_id UUID REFERENCES agents(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE metrics (ts TIMESTAMPTZ NOT NULL, agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  name TEXT NOT NULL, value DOUBLE PRECISION NOT NULL);
CREATE INDEX metrics_lookup ON metrics (agent_id, name, ts DESC);
CREATE TABLE health_checks (id BIGSERIAL PRIMARY KEY, service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
  ts TIMESTAMPTZ NOT NULL DEFAULT now(), ok BOOLEAN NOT NULL, status INT, latency_ms INT, error TEXT);
CREATE INDEX health_checks_lookup ON health_checks (service_id, ts DESC);
CREATE TABLE incidents (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), service_id UUID REFERENCES services(id),
  title TEXT NOT NULL, severity TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'open'
  CHECK (status IN ('open','investigating','recovering','resolved','escalated')),
  dedup_key TEXT NOT NULL, detected_at TIMESTAMPTZ NOT NULL DEFAULT now(), resolved_at TIMESTAMPTZ, summary TEXT);
CREATE UNIQUE INDEX one_open_incident ON incidents (dedup_key) WHERE status <> 'resolved';
CREATE TABLE incident_events (id BIGSERIAL PRIMARY KEY, incident_id UUID NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
  ts TIMESTAMPTZ NOT NULL DEFAULT now(), kind TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}');
CREATE TABLE diagnostic_results (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), incident_id UUID NOT NULL REFERENCES incidents(id),
  probable_cause TEXT NOT NULL, status TEXT NOT NULL CHECK (status IN ('suspected','confirmed')),
  evidence JSONB NOT NULL, confidence REAL NOT NULL, recommended_action TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE recovery_policies (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), name TEXT UNIQUE NOT NULL, service_id UUID REFERENCES services(id),
  condition JSONB NOT NULL, action_id TEXT NOT NULL, mode TEXT NOT NULL DEFAULT 'simulate' CHECK (mode IN ('simulate','approval','auto')),
  retry_limit INT NOT NULL DEFAULT 3, cooldown_s INT NOT NULL DEFAULT 60, verify_timeout_s INT NOT NULL DEFAULT 30,
  priority INT NOT NULL DEFAULT 100, enabled BOOLEAN NOT NULL DEFAULT true);
CREATE TABLE recovery_operations (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), incident_id UUID REFERENCES incidents(id),
  policy_id UUID REFERENCES recovery_policies(id), action_id TEXT NOT NULL, target TEXT NOT NULL, state TEXT NOT NULL,
  started_at TIMESTAMPTZ, finished_at TIMESTAMPTZ, previous_state JSONB, resulting_state JSONB, verification JSONB, error TEXT);
CREATE TABLE audit_logs (id BIGSERIAL PRIMARY KEY, ts TIMESTAMPTZ NOT NULL DEFAULT now(), actor TEXT NOT NULL, action TEXT NOT NULL,
  target TEXT, outcome TEXT NOT NULL, incident_id UUID, metadata JSONB NOT NULL DEFAULT '{}');
CREATE INDEX audit_ts ON audit_logs (ts DESC);
-- Audit records are append-only for ordinary roles.
CREATE RULE audit_no_update AS ON UPDATE TO audit_logs DO INSTEAD NOTHING;
CREATE RULE audit_no_delete AS ON DELETE TO audit_logs DO INSTEAD NOTHING;
