// Package core contains the Reforge Core runtime: monitoring, detection, orchestration.
package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/reforge/core/internal/recovery"
)

const (
	detectFailures = 3
	slowMS         = 1500
	agentStale     = 15 * time.Second
)

type Config struct {
	Addr            string
	DataDir         string
	AdminToken      string
	ViewerToken     string
	IntelligenceURL string
	RecoveryMode    string
	SeedDemo        bool
	DemoAgentSecret string
	ManualBaselineS int
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func ConfigFromEnv() Config {
	mb, _ := strconv.Atoi(env("REFORGE_MANUAL_BASELINE_S", "300"))
	return Config{
		Addr: env("REFORGE_ADDR", ":8080"), DataDir: env("REFORGE_DATA_DIR", "./data"),
		AdminToken: os.Getenv("REFORGE_ADMIN_TOKEN"), ViewerToken: os.Getenv("REFORGE_VIEWER_TOKEN"),
		IntelligenceURL: env("REFORGE_INTELLIGENCE_URL", "http://localhost:8001"),
		RecoveryMode:    env("REFORGE_RECOVERY_MODE", "approval"),
		SeedDemo:        env("REFORGE_SEED_DEMO", "false") == "true",
		DemoAgentSecret: os.Getenv("REFORGE_AGENT_SECRET"), ManualBaselineS: mb,
	}
}

type Core struct {
	cfg      Config
	St       *Store
	ctx      context.Context
	engines  map[string]*recovery.Engine
	engMu    sync.Mutex
	inflight map[string]bool
	ifMu     sync.Mutex
}

func New(ctx context.Context, cfg Config) *Core {
	c := &Core{cfg: cfg, St: NewStore(cfg.DataDir), ctx: ctx, engines: map[string]*recovery.Engine{}, inflight: map[string]bool{}}
	c.seed()
	return c
}

func HashSecret(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func (c *Core) seed() {
	st := c.St
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.Policies) == 0 {
		mode := c.cfg.RecoveryMode
		for _, p := range []*Policy{
			{Name: "Restart stopped or unhealthy containers", Service: "*", MinFailures: 3,
				ContainerStates: []string{"exited", "dead", "unhealthy"}, MinConfidence: 0.6, ActionID: "restart_container",
				Mode: mode, RetryLimit: 3, CooldownS: 20, VerifyTimeoutS: 30, Priority: 10, Enabled: true},
			{Name: "Run application recovery hook", Service: "*", MinFailures: 3, MinConfidence: 0.2,
				ActionID: "app_recovery_hook", Mode: mode, RetryLimit: 2, CooldownS: 20, VerifyTimeoutS: 30, Priority: 20, Enabled: true},
		} {
			p.ID = newID("pol")
			st.Policies[p.ID] = p
		}
	}
	if !c.cfg.SeedDemo || len(st.Services) > 0 {
		return
	}
	names := []string{"payment-service", "user-service", "inventory-service"}
	for _, n := range names {
		base := "http://" + n + ":8080"
		s := &Service{ID: newID("svc"), Name: n, Kind: "application", Environment: "demo", BaseURL: base,
			HealthURL: base + "/health", ExpectedStatus: 200, TimeoutMS: 2000, IntervalS: 3, Container: n,
			HookURL: base + "/recover", ControlURL: base, ChaosTarget: true, Status: "unknown"}
		if n == "payment-service" {
			s.DependsOn = []string{"user-service", "inventory-service"}
		}
		st.Services[s.ID] = s
	}
	if c.cfg.DemoAgentSecret != "" {
		st.Agents["agent-demo"] = &Agent{ID: "agent-demo", Name: "demo-agent", SecretHash: HashSecret(c.cfg.DemoAgentSecret),
			Permissions: []string{"metrics.read", "containers.inspect", "containers.restart"}, Targets: names, CreatedAt: time.Now().UTC()}
	}
	st.dirty = true
}

// Run starts background workers and blocks until ctx is cancelled.
func (c *Core) Run() {
	t := time.NewTicker(time.Second)
	f := time.NewTicker(5 * time.Second)
	defer t.Stop()
	defer f.Stop()
	for {
		select {
		case <-c.ctx.Done():
			c.St.Flush()
			return
		case <-t.C:
			c.tick()
		case <-f.C:
			c.St.Flush()
		}
	}
}

func (c *Core) tick() {
	now := time.Now()
	var due []string
	var gone []string
	c.St.mu.Lock()
	for id, s := range c.St.Services {
		if now.Sub(s.LastCheck) >= time.Duration(s.IntervalS)*time.Second {
			due = append(due, id)
		}
	}
	for _, a := range c.St.Agents {
		if a.Connected && now.Sub(a.LastHeartbeat) > agentStale {
			a.Connected = false
			gone = append(gone, a.ID)
		}
	}
	c.St.mu.Unlock()
	for _, id := range gone {
		c.St.Audit("system", "agent.disconnected", id, "warning", "", nil)
		c.St.Publish("agent_disconnected", map[string]string{"agent_id": id})
	}
	for _, id := range due {
		c.ifMu.Lock()
		busy := c.inflight[id]
		c.inflight[id] = true
		c.ifMu.Unlock()
		if busy {
			continue
		}
		go func(id string) {
			defer func() { c.ifMu.Lock(); delete(c.inflight, id); c.ifMu.Unlock() }()
			c.CheckService(id)
		}(id)
	}
}

func (c *Core) probe(s Service) (int, int64, error) {
	client := &http.Client{Timeout: time.Duration(s.TimeoutMS) * time.Millisecond}
	start := time.Now()
	resp, err := client.Get(s.HealthURL)
	lat := time.Since(start).Milliseconds()
	if err != nil {
		return 0, lat, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != s.ExpectedStatus {
		return resp.StatusCode, lat, fmt.Errorf("unexpected status %d (expected %d)", resp.StatusCode, s.ExpectedStatus)
	}
	return resp.StatusCode, lat, nil
}

func (c *Core) CheckService(id string) {
	c.St.mu.RLock()
	sp := c.St.Services[id]
	if sp == nil {
		c.St.mu.RUnlock()
		return
	}
	svc := *sp
	c.St.mu.RUnlock()

	code, lat, err := c.probe(svc)
	now := time.Now().UTC()
	ok := err == nil

	c.St.mu.Lock()
	s := c.St.Services[id]
	if s == nil {
		c.St.mu.Unlock()
		return
	}
	s.LastCheck, s.LastStatusCode, s.LastLatencyMS = now, code, lat
	if ok {
		s.ConsecutiveFailures, s.LastError = 0, ""
		t := now
		s.LastSuccess = &t
		if lat > slowMS {
			s.SlowStreak++
		} else {
			s.SlowStreak = 0
		}
	} else {
		s.ConsecutiveFailures++
		s.TotalFailures++
		s.SlowStreak = 0
		s.LastError = err.Error()
	}
	prev := s.Status
	switch {
	case s.ConsecutiveFailures >= detectFailures:
		s.Status = "failed"
	case s.ConsecutiveFailures > 0 || s.SlowStreak > 0:
		s.Status = "degraded"
	default:
		s.Status = "healthy"
	}
	name, status := s.Name, s.Status
	c.St.dirty = true
	c.St.mu.Unlock()

	c.St.AddPoint("svc:"+name, Point{TS: now.UnixMilli(), Value: float64(lat), OK: ok})
	if prev != status {
		c.St.Publish("service_status", map[string]string{"service": name, "status": status})
	}
	c.evaluate(name)
}

func (c *Core) evaluate(name string) {
	var created *Incident
	var resolved []string
	c.St.mu.Lock()
	var svc *Service
	for _, s := range c.St.Services {
		if s.Name == name {
			svc = s
		}
	}
	if svc == nil {
		c.St.mu.Unlock()
		return
	}
	switch {
	case svc.ConsecutiveFailures >= detectFailures:
		key := "svc:" + name + ":unavailable"
		if c.St.openIncident(key) == nil {
			created = c.newIncidentLocked(name, key, name+" is unavailable", "Service health checks are failing.", "critical", "open",
				[]string{fmt.Sprintf("%d consecutive failed health checks", svc.ConsecutiveFailures), "last error: " + svc.LastError})
		}
	case svc.SlowStreak >= 3:
		key := "svc:" + name + ":slow"
		if c.St.openIncident(key) == nil {
			created = c.newIncidentLocked(name, key, name+" responding slowly", "Response time is above the threshold.", "warning", "open",
				[]string{fmt.Sprintf("%d consecutive responses slower than %dms (last %dms)", svc.SlowStreak, slowMS, svc.LastLatencyMS)})
		}
	}
	if svc.ConsecutiveFailures == 0 && svc.SlowStreak == 0 {
		for _, inc := range c.St.Incidents {
			if inc.Service == name && inc.Status != "resolved" && strings.HasPrefix(inc.DedupKey, "svc:") && !c.activeOpLocked(inc.ID) {
				c.resolveLocked(inc, "Health checks are passing again; no active recovery was in progress.")
				resolved = append(resolved, inc.ID)
			}
		}
	}
	c.St.dirty = true
	c.St.mu.Unlock()

	for _, id := range resolved {
		c.St.Audit("system", "incident.auto_resolved", name, "success", id, nil)
		c.St.Publish("incident", map[string]string{"id": id, "status": "resolved"})
	}
	if created != nil {
		c.St.Audit("system", "incident.detected", name, "success", created.ID, map[string]string{"severity": created.Severity})
		c.St.Publish("incident", map[string]string{"id": created.ID, "status": "open"})
		go c.runIncident(created.ID)
	}
}

func (c *Core) newIncidentLocked(svc, key, title, desc, sev, status string, evidence []string) *Incident {
	inc := &Incident{ID: newID("inc"), Service: svc, Title: title, Description: desc, Severity: sev, Status: status,
		DedupKey: key, DetectedAt: time.Now().UTC(), Evidence: evidence}
	inc.addEvent("detected", strings.Join(evidence, "; "))
	c.St.Incidents = append(c.St.Incidents, inc)
	for _, e := range c.St.Experiments {
		if e.Service == svc && e.DetectedAt == nil && time.Since(e.InjectedAt) < 15*time.Minute {
			t := inc.DetectedAt
			e.DetectedAt, e.IncidentID = &t, inc.ID
		}
	}
	return inc
}

func (c *Core) activeOpLocked(incID string) bool {
	for _, o := range c.St.Operations {
		if o.IncidentID == incID && (o.State == "executing" || o.State == "verifying" || o.State == "validating") {
			return true
		}
	}
	return false
}

func (c *Core) resolveLocked(inc *Incident, summary string) {
	t := time.Now().UTC()
	inc.Status, inc.ResolvedAt, inc.Summary = "resolved", &t, summary
	inc.addEvent("resolved", summary)
	for _, o := range c.St.Operations {
		if o.IncidentID == inc.ID && o.State == "awaiting_approval" {
			o.State = "cancelled"
			o.Error = "incident resolved before approval"
		}
	}
}

// ---------- telemetry ----------

type Telemetry struct {
	Hostname     string              `json:"hostname"`
	OS           string              `json:"os"`
	Version      string              `json:"version"`
	Capabilities []string            `json:"capabilities"`
	Metrics      map[string]*float64 `json:"metrics"`
	Unsupported  []string            `json:"unsupported"`
	Containers   []ContainerInfo     `json:"containers"`
}

var hostLimits = map[string]float64{"cpu_percent": 90, "memory_percent": 90, "disk_percent": 90}

func (c *Core) Ingest(agentID string, t Telemetry) {
	now := time.Now().UTC()
	var created []*Incident
	var resolved []string
	c.St.mu.Lock()
	a := c.St.Agents[agentID]
	if a == nil || a.Revoked {
		c.St.mu.Unlock()
		return
	}
	was := a.Connected
	a.Hostname, a.OS, a.Version, a.Capabilities = t.Hostname, t.OS, t.Version, t.Capabilities
	a.LastHeartbeat, a.Connected = now, true
	a.Metrics = map[string]float64{}
	for k, v := range t.Metrics {
		if v != nil {
			a.Metrics[k] = *v
		}
	}
	a.Unsupported, a.Containers = t.Unsupported, t.Containers
	for _, s := range c.St.Services {
		if s.Container == "" || !contains(a.Targets, s.Container) {
			continue
		}
		s.ContainerState = "missing"
		for _, ci := range t.Containers {
			if ci.Name == s.Container {
				s.ContainerState = ci.State
			}
		}
	}
	for m, limit := range hostLimits {
		v, ok := a.Metrics[m]
		if !ok {
			continue
		}
		key := "host:" + a.ID + ":" + m
		open := c.St.openIncident(key)
		switch {
		case v > limit && open == nil:
			sev := "warning"
			if v > 97 {
				sev = "critical"
			}
			inc := c.newIncidentLocked("host:"+a.Hostname, key, fmt.Sprintf("High %s on %s", m, a.Hostname),
				"Host resource threshold exceeded. Alert only: no automatic remediation exists for host metrics.", sev, "escalated",
				[]string{fmt.Sprintf("%s=%.1f (limit %.0f)", m, v, limit)})
			created = append(created, inc)
		case v < limit-5 && open != nil:
			c.resolveLocked(open, fmt.Sprintf("%s back to %.1f", m, v))
			resolved = append(resolved, open.ID)
		}
	}
	c.St.dirty = true
	c.St.mu.Unlock()

	for m, v := range t.Metrics {
		if v != nil {
			c.St.AddPoint(agentID+"|"+m, Point{TS: now.UnixMilli(), Value: *v, OK: true})
		}
	}
	if !was {
		c.St.Audit("agent:"+agentID, "agent.connected", agentID, "success", "", nil)
		c.St.Publish("agent_connected", map[string]string{"agent_id": agentID})
	}
	for _, i := range created {
		c.St.Audit("system", "incident.detected", i.Service, "success", i.ID, nil)
		c.St.Publish("incident", map[string]string{"id": i.ID, "status": i.Status})
	}
	for _, id := range resolved {
		c.St.Publish("incident", map[string]string{"id": id, "status": "resolved"})
	}
	c.St.Publish("metrics", map[string]string{"agent_id": agentID})
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// ---------- agent command queue ----------

func (c *Core) enqueue(agentID, action, target string) *Command {
	cmd := &Command{ID: newID("cmd"), AgentID: agentID, Action: action, Target: target, CreatedAt: time.Now().UTC(),
		done: make(chan CommandResult, 1)}
	c.St.mu.Lock()
	c.St.Commands[cmd.ID] = cmd
	c.St.mu.Unlock()
	return cmd
}

func (c *Core) PendingCommands(agentID string) []*Command {
	c.St.mu.Lock()
	defer c.St.mu.Unlock()
	var out []*Command
	for id, cmd := range c.St.Commands {
		if time.Since(cmd.CreatedAt) > 2*time.Minute {
			delete(c.St.Commands, id)
			continue
		}
		if cmd.AgentID == agentID && !cmd.delivered {
			cmd.delivered = true
			out = append(out, cmd)
		}
	}
	return out
}

func (c *Core) CommandResult(agentID, cmdID string, r CommandResult) bool {
	c.St.mu.Lock()
	cmd := c.St.Commands[cmdID]
	if cmd == nil || cmd.AgentID != agentID {
		c.St.mu.Unlock()
		return false
	}
	delete(c.St.Commands, cmdID)
	c.St.mu.Unlock()
	cmd.done <- r
	return true
}

func (c *Core) waitCommand(ctx context.Context, cmd *Command) error {
	wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tk := time.NewTicker(time.Second)
	defer tk.Stop()
	for {
		select {
		case r := <-cmd.done:
			if !r.OK {
				return fmt.Errorf("agent reported failure: %s", r.Error)
			}
			return nil
		case <-tk.C:
			c.St.mu.RLock()
			a := c.St.Agents[cmd.AgentID]
			alive := a != nil && a.Connected && !a.Revoked
			c.St.mu.RUnlock()
			if !alive {
				return fmt.Errorf("agent disconnected during recovery")
			}
		case <-wctx.Done():
			return fmt.Errorf("agent did not complete the action in time: %w", wctx.Err())
		}
	}
}

func (c *Core) agentFor(container string) *Agent {
	c.St.mu.RLock()
	defer c.St.mu.RUnlock()
	for _, a := range c.St.Agents {
		if !a.Revoked && a.Connected && contains(a.Targets, container) {
			cp := *a
			return &cp
		}
	}
	return nil
}

func (c *Core) containerInfo(name string) *ContainerInfo {
	c.St.mu.RLock()
	defer c.St.mu.RUnlock()
	for _, a := range c.St.Agents {
		if a.Revoked || !a.Connected {
			continue
		}
		for _, ci := range a.Containers {
			if ci.Name == name {
				cp := ci
				return &cp
			}
		}
	}
	return nil
}

func (c *Core) serviceByName(name string) *Service {
	c.St.mu.RLock()
	defer c.St.mu.RUnlock()
	for _, s := range c.St.Services {
		if s.Name == name {
			cp := *s
			return &cp
		}
	}
	return nil
}

func postJSON(ctx context.Context, url string, body any, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}
