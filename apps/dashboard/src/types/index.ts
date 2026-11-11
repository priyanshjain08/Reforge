export interface Service {
  id: string; name: string; kind: string; environment: string; health_url: string; status: string;
  consecutive_failures: number; total_failures: number; last_latency_ms: number; last_error?: string;
  container?: string; container_state?: string; interval_s: number; chaos_target: boolean; last_check: string;
}
export interface ContainerInfo { name: string; state: string; status: string; health?: string; restarts: number; exit_code: number }
export interface Agent {
  id: string; name: string; hostname: string; os: string; version: string; capabilities: string[]; permissions: string[];
  targets: string[]; revoked: boolean; connected: boolean; last_heartbeat: string; metrics: Record<string, number>;
  unsupported_metrics: string[]; containers: ContainerInfo[];
}
export interface Diagnosis {
  probable_cause: string; affected_component: string; status: "suspected" | "confirmed"; evidence: string[]; confidence: number;
  recommended_action: string; alternatives: string[]; explanation: string; source: string; created_at: string;
}
export interface Incident {
  id: string; service: string; title: string; description: string; severity: string; status: string; detected_at: string;
  resolved_at?: string; acknowledged: boolean; assignee?: string; summary?: string; evidence: string[];
  diagnoses: Diagnosis[]; events: { ts: string; kind: string; detail: string }[]; notes: { ts: string; author: string; text: string }[];
}
export interface Operation {
  id: string; incident_id: string; policy_name: string; action_id: string; target: string; service: string; mode: string;
  state: string; steps: string[]; error?: string; started_at: string; finished_at?: string; previous_state?: string;
  resulting_state?: string; verified?: boolean;
}
export interface Policy {
  id: string; name: string; service: string; min_failures: number; container_states: string[]; min_confidence: number;
  action_id: string; mode: string; retry_limit: number; cooldown_s: number; verify_timeout_s: number; priority: number; enabled: boolean;
}
export interface Experiment {
  id: string; service: string; type: string; duration_s: number; intensity: number; status: string; injected_at: string;
  detected_at?: string; diagnosed_at?: string; recovery_started_at?: string; recovered_at?: string; verified?: boolean; incident_id?: string;
}
export interface AuditEntry { id: number; ts: string; actor: string; action: string; target: string; outcome: string; incident_id?: string }
