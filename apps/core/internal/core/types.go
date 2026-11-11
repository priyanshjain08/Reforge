package core

import "time"

type Service struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	Environment    string   `json:"environment"`
	BaseURL        string   `json:"base_url"`
	HealthURL      string   `json:"health_url"`
	ExpectedStatus int      `json:"expected_status"`
	TimeoutMS      int      `json:"timeout_ms"`
	IntervalS      int      `json:"interval_s"`
	Container      string   `json:"container,omitempty"`
	HookURL        string   `json:"hook_url,omitempty"`
	ControlURL     string   `json:"control_url,omitempty"`
	ChaosTarget    bool     `json:"chaos_target"`
	DependsOn      []string `json:"depends_on,omitempty"`

	// runtime state
	Status              string     `json:"status"` // unknown|healthy|degraded|failed
	ConsecutiveFailures int        `json:"consecutive_failures"`
	TotalFailures       int        `json:"total_failures"`
	SlowStreak          int        `json:"slow_streak"`
	LastStatusCode      int        `json:"last_status_code"`
	LastLatencyMS       int64      `json:"last_latency_ms"`
	LastError           string     `json:"last_error,omitempty"`
	LastCheck           time.Time  `json:"last_check"`
	LastSuccess         *time.Time `json:"last_success,omitempty"`
	ContainerState      string     `json:"container_state,omitempty"`
}

type ContainerInfo struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Status   string `json:"status"`
	Health   string `json:"health,omitempty"`
	Restarts int    `json:"restarts"`
	ExitCode int    `json:"exit_code"`
}

type Agent struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	SecretHash    string             `json:"secret_hash,omitempty"`
	Hostname      string             `json:"hostname"`
	OS            string             `json:"os"`
	Version       string             `json:"version"`
	Capabilities  []string           `json:"capabilities"`
	Permissions   []string           `json:"permissions"`
	Targets       []string           `json:"targets"` // allowed container names
	Revoked       bool               `json:"revoked"`
	CreatedAt     time.Time          `json:"created_at"`
	LastHeartbeat time.Time          `json:"last_heartbeat"`
	Connected     bool               `json:"connected"`
	Metrics       map[string]float64 `json:"metrics"`
	Unsupported   []string           `json:"unsupported_metrics"`
	Containers    []ContainerInfo    `json:"containers"`
}

type Diagnosis struct {
	ProbableCause     string    `json:"probable_cause"`
	AffectedComponent string    `json:"affected_component"`
	Status            string    `json:"status"` // suspected|confirmed
	Evidence          []string  `json:"evidence"`
	Confidence        float64   `json:"confidence"`
	RecommendedAction string    `json:"recommended_action"`
	Alternatives      []string  `json:"alternatives"`
	Explanation       string    `json:"explanation"`
	Source            string    `json:"source"` // intelligence|fallback
	CreatedAt         time.Time `json:"created_at"`
}

type IncidentEvent struct {
	TS     time.Time `json:"ts"`
	Kind   string    `json:"kind"`
	Detail string    `json:"detail"`
}

type Note struct {
	TS     time.Time `json:"ts"`
	Author string    `json:"author"`
	Text   string    `json:"text"`
}

type Incident struct {
	ID           string          `json:"id"`
	Service      string          `json:"service"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Severity     string          `json:"severity"` // info|warning|critical
	Status       string          `json:"status"`   // open|investigating|recovering|resolved|escalated
	DedupKey     string          `json:"dedup_key"`
	DetectedAt   time.Time       `json:"detected_at"`
	ResolvedAt   *time.Time      `json:"resolved_at,omitempty"`
	Acknowledged bool            `json:"acknowledged"`
	Assignee     string          `json:"assignee,omitempty"`
	Summary      string          `json:"summary,omitempty"`
	Evidence     []string        `json:"evidence"`
	Diagnoses    []Diagnosis     `json:"diagnoses"`
	Events       []IncidentEvent `json:"events"`
	Notes        []Note          `json:"notes"`
}

type Operation struct {
	ID             string     `json:"id"`
	IncidentID     string     `json:"incident_id"`
	PolicyID       string     `json:"policy_id"`
	PolicyName     string     `json:"policy_name"`
	ActionID       string     `json:"action_id"`
	Target         string     `json:"target"`
	Service        string     `json:"service"`
	Mode           string     `json:"mode"`
	State          string     `json:"state"`
	Steps          []string   `json:"steps"`
	Error          string     `json:"error,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	PreviousState  string     `json:"previous_state,omitempty"`
	ResultingState string     `json:"resulting_state,omitempty"`
	Verified       *bool      `json:"verified,omitempty"`
}

type Policy struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Service         string   `json:"service"` // service name or "*"
	MinFailures     int      `json:"min_failures"`
	ContainerStates []string `json:"container_states"`
	MinConfidence   float64  `json:"min_confidence"`
	ActionID        string   `json:"action_id"`
	Mode            string   `json:"mode"` // simulate|approval|auto
	RetryLimit      int      `json:"retry_limit"`
	CooldownS       int      `json:"cooldown_s"`
	VerifyTimeoutS  int      `json:"verify_timeout_s"`
	Priority        int      `json:"priority"`
	Enabled         bool     `json:"enabled"`
}

type Experiment struct {
	ID                string     `json:"id"`
	Service           string     `json:"service"`
	Type              string     `json:"type"`
	DurationS         int        `json:"duration_s"`
	Intensity         int        `json:"intensity"`
	Status            string     `json:"status"` // running|completed|stopped
	InjectedAt        time.Time  `json:"injected_at"`
	DetectedAt        *time.Time `json:"detected_at,omitempty"`
	DiagnosedAt       *time.Time `json:"diagnosed_at,omitempty"`
	RecoveryStartedAt *time.Time `json:"recovery_started_at,omitempty"`
	RecoveredAt       *time.Time `json:"recovered_at,omitempty"`
	Verified          *bool      `json:"verified,omitempty"`
	IncidentID        string     `json:"incident_id,omitempty"`
}

type Audit struct {
	ID         int64             `json:"id"`
	TS         time.Time         `json:"ts"`
	Actor      string            `json:"actor"`
	Action     string            `json:"action"`
	Target     string            `json:"target"`
	Outcome    string            `json:"outcome"`
	IncidentID string            `json:"incident_id,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type Point struct {
	TS    int64   `json:"ts"`
	Value float64 `json:"value"`
	OK    bool    `json:"ok"`
}

type Command struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"-"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	CreatedAt time.Time `json:"created_at"`
	delivered bool
	done      chan CommandResult
}

type CommandResult struct {
	OK             bool   `json:"ok"`
	Error          string `json:"error"`
	ContainerState string `json:"container_state"`
}
