package core

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	c    *Core
	mux  *http.ServeMux
	rlMu sync.Mutex
	rl   map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

func NewServer(c *Core) *Server {
	s := &Server{c: c, mux: http.NewServeMux(), rl: map[string]*bucket{}}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.secure(s.rateLimit(s.mux)) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func ok(w http.ResponseWriter, v any)                     { writeJSON(w, 200, map[string]any{"data": v}) }
func fail(w http.ResponseWriter, code int, msg string)    { writeJSON(w, code, map[string]any{"error": map[string]string{"message": msg}}) }
func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(v)
}

// ---------- middleware ----------

func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		s.rlMu.Lock()
		b := s.rl[ip]
		now := time.Now()
		if b == nil {
			b = &bucket{tokens: 200, last: now}
			s.rl[ip] = b
		}
		b.tokens += now.Sub(b.last).Seconds() * 50 // 50 req/s sustained, burst 200
		if b.tokens > 200 {
			b.tokens = 200
		}
		b.last = now
		allowed := b.tokens >= 1
		if allowed {
			b.tokens--
		}
		s.rlMu.Unlock()
		if !allowed {
			fail(w, 429, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) role(r *http.Request) string {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" && r.URL.Path == "/api/v1/events" {
		tok = r.URL.Query().Get("token")
	}
	if tok == "" {
		return ""
	}
	if s.c.cfg.AdminToken != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(s.c.cfg.AdminToken)) == 1 {
		return "admin"
	}
	if s.c.cfg.ViewerToken != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(s.c.cfg.ViewerToken)) == 1 {
		return "viewer"
	}
	return ""
}

// viewer: any authenticated role (read). admin: admin role only (mutations).
func (s *Server) viewer(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.role(r) == "" {
			fail(w, 401, "authentication required")
			return
		}
		h(w, r)
	}
}

func (s *Server) admin(h func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch s.role(r) {
		case "admin":
			h(w, r, "admin")
		case "":
			fail(w, 401, "authentication required")
		default:
			fail(w, 403, "admin role required")
		}
	}
}

func (s *Server) agentAuth(h func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, secret := r.Header.Get("X-Agent-ID"), r.Header.Get("X-Agent-Secret")
		s.c.St.mu.RLock()
		a := s.c.St.Agents[id]
		valid := a != nil && !a.Revoked && secret != "" &&
			subtle.ConstantTimeCompare([]byte(HashSecret(secret)), []byte(a.SecretHash)) == 1
		s.c.St.mu.RUnlock()
		if !valid {
			fail(w, 401, "invalid agent credentials")
			return
		}
		h(w, r, id)
	}
}

// ---------- routes ----------

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)

func (s *Server) routes() {
	m, c, st := s.mux, s.c, s.c.St
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { ok(w, map[string]string{"status": "ok"}) })

	// agent endpoints
	m.HandleFunc("POST /api/v1/agent/telemetry", s.agentAuth(func(w http.ResponseWriter, r *http.Request, id string) {
		var t Telemetry
		if err := decode(r, &t); err != nil {
			fail(w, 400, "invalid telemetry: "+err.Error())
			return
		}
		c.Ingest(id, t)
		ok(w, map[string]string{"status": "accepted"})
	}))
	m.HandleFunc("GET /api/v1/agent/commands", s.agentAuth(func(w http.ResponseWriter, r *http.Request, id string) {
		cmds := c.PendingCommands(id)
		if cmds == nil {
			cmds = []*Command{}
		}
		ok(w, cmds)
	}))
	m.HandleFunc("POST /api/v1/agent/commands/{id}/result", s.agentAuth(func(w http.ResponseWriter, r *http.Request, id string) {
		var res CommandResult
		if err := decode(r, &res); err != nil {
			fail(w, 400, err.Error())
			return
		}
		if !c.CommandResult(id, r.PathValue("id"), res) {
			fail(w, 404, "unknown command")
			return
		}
		ok(w, map[string]string{"status": "recorded"})
	}))

	// dashboard
	m.HandleFunc("GET /api/v1/dashboard", s.viewer(s.dashboard))
	m.HandleFunc("GET /api/v1/events", s.viewer(s.events))
	m.HandleFunc("GET /api/v1/topology", s.viewer(s.topology))
	m.HandleFunc("GET /api/v1/metrics", s.viewer(func(w http.ResponseWriter, r *http.Request) {
		agent := r.URL.Query().Get("agent_id")
		out := map[string][]Point{}
		st.mu.RLock()
		for k, v := range st.History {
			if strings.HasPrefix(k, agent+"|") {
				out[strings.TrimPrefix(k, agent+"|")] = append([]Point(nil), v...)
			}
		}
		st.mu.RUnlock()
		ok(w, out)
	}))

	// services
	m.HandleFunc("GET /api/v1/services", s.viewer(func(w http.ResponseWriter, r *http.Request) {
		st.mu.RLock()
		out := []Service{}
		for _, v := range st.Services {
			out = append(out, *v)
		}
		st.mu.RUnlock()
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		ok(w, out)
	}))
	m.HandleFunc("POST /api/v1/services", s.admin(s.createService))
	m.HandleFunc("DELETE /api/v1/services/{id}", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) {
		st.mu.Lock()
		v := st.Services[r.PathValue("id")]
		if v != nil {
			delete(st.Services, v.ID)
			st.dirty = true
		}
		st.mu.Unlock()
		if v == nil {
			fail(w, 404, "service not found")
			return
		}
		st.Audit(actor, "service.deleted", v.Name, "success", "", nil)
		ok(w, map[string]string{"deleted": v.ID})
	}))
	m.HandleFunc("GET /api/v1/services/{id}/history", s.viewer(func(w http.ResponseWriter, r *http.Request) {
		st.mu.RLock()
		var pts []Point
		if v := st.Services[r.PathValue("id")]; v != nil {
			pts = append(pts, st.History["svc:"+v.Name]...)
		}
		st.mu.RUnlock()
		ok(w, pts)
	}))

	// agents
	m.HandleFunc("GET /api/v1/agents", s.viewer(func(w http.ResponseWriter, r *http.Request) {
		st.mu.RLock()
		out := []Agent{}
		for _, a := range st.Agents {
			cp := *a
			cp.SecretHash = "" // never expose secrets
			out = append(out, cp)
		}
		st.mu.RUnlock()
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		ok(w, out)
	}))
	m.HandleFunc("POST /api/v1/agents", s.admin(s.createAgent))
	m.HandleFunc("POST /api/v1/agents/{id}/revoke", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) {
		st.mu.Lock()
		a := st.Agents[r.PathValue("id")]
		if a != nil {
			a.Revoked, a.Connected = true, false
			st.dirty = true
		}
		st.mu.Unlock()
		if a == nil {
			fail(w, 404, "agent not found")
			return
		}
		st.Audit(actor, "agent.revoked", a.ID, "success", "", nil)
		ok(w, map[string]string{"revoked": a.ID})
	}))

	// incidents
	m.HandleFunc("GET /api/v1/incidents", s.viewer(s.listIncidents))
	m.HandleFunc("GET /api/v1/incidents/{id}", s.viewer(func(w http.ResponseWriter, r *http.Request) {
		st.mu.RLock()
		defer st.mu.RUnlock()
		i := st.incidentByID(r.PathValue("id"))
		if i == nil {
			fail(w, 404, "incident not found")
			return
		}
		ops := []Operation{}
		for _, o := range st.Operations {
			if o.IncidentID == i.ID {
				ops = append(ops, *o)
			}
		}
		ok(w, map[string]any{"incident": i, "operations": ops})
	}))
	m.HandleFunc("POST /api/v1/incidents/{id}/ack", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) {
		s.mutateIncident(w, r, actor, "incident.acknowledged", func(i *Incident) error {
			i.Acknowledged = true
			i.addEvent("acknowledged", "by "+actor)
			return nil
		})
	}))
	m.HandleFunc("POST /api/v1/incidents/{id}/resolve", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) {
		var b struct{ Summary string `json:"summary"` }
		if decode(r, &b) != nil || strings.TrimSpace(b.Summary) == "" {
			fail(w, 400, "summary is required")
			return
		}
		s.mutateIncident(w, r, actor, "incident.resolved_manually", func(i *Incident) error {
			c.resolveLocked(i, "Manually resolved by "+actor+": "+b.Summary)
			return nil
		})
	}))
	m.HandleFunc("POST /api/v1/incidents/{id}/notes", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) {
		var b struct{ Text string `json:"text"` }
		if decode(r, &b) != nil || strings.TrimSpace(b.Text) == "" {
			fail(w, 400, "text is required")
			return
		}
		s.mutateIncident(w, r, actor, "incident.note_added", func(i *Incident) error {
			i.Notes = append(i.Notes, Note{TS: time.Now().UTC(), Author: actor, Text: b.Text})
			i.addEvent("note", b.Text)
			return nil
		})
	}))
	m.HandleFunc("POST /api/v1/incidents/{id}/assign", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) {
		var b struct{ Assignee string `json:"assignee"` }
		if decode(r, &b) != nil {
			fail(w, 400, "invalid body")
			return
		}
		s.mutateIncident(w, r, actor, "incident.assigned", func(i *Incident) error {
			i.Assignee = b.Assignee
			i.addEvent("assigned", "to "+b.Assignee)
			return nil
		})
	}))

	// recovery
	m.HandleFunc("GET /api/v1/recovery", s.viewer(func(w http.ResponseWriter, r *http.Request) {
		st.mu.RLock()
		out := []Operation{}
		for i := len(st.Operations) - 1; i >= 0; i-- {
			out = append(out, *st.Operations[i])
		}
		st.mu.RUnlock()
		ok(w, out)
	}))
	m.HandleFunc("POST /api/v1/recovery/{id}/approve", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) {
		if err := c.ApproveOperation(r.PathValue("id"), actor); err != nil {
			fail(w, 409, err.Error())
			return
		}
		ok(w, map[string]string{"status": "approved"})
	}))
	m.HandleFunc("POST /api/v1/recovery/{id}/cancel", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) {
		if err := c.CancelOperation(r.PathValue("id"), actor); err != nil {
			fail(w, 409, err.Error())
			return
		}
		ok(w, map[string]string{"status": "cancelled"})
	}))
	m.HandleFunc("GET /api/v1/killswitch", s.viewer(func(w http.ResponseWriter, r *http.Request) {
		ok(w, map[string]bool{"enabled": c.killSwitch()})
	}))
	m.HandleFunc("POST /api/v1/killswitch", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) {
		var b struct{ Enabled bool `json:"enabled"` }
		if decode(r, &b) != nil {
			fail(w, 400, "invalid body")
			return
		}
		c.SetKillSwitch(b.Enabled, actor)
		ok(w, map[string]bool{"enabled": b.Enabled})
	}))

	// policies
	m.HandleFunc("GET /api/v1/policies", s.viewer(func(w http.ResponseWriter, r *http.Request) {
		st.mu.RLock()
		out := []Policy{}
		for _, p := range st.Policies {
			out = append(out, *p)
		}
		st.mu.RUnlock()
		sort.Slice(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
		ok(w, out)
	}))
	m.HandleFunc("POST /api/v1/policies", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) { s.savePolicy(w, r, actor, "") }))
	m.HandleFunc("PUT /api/v1/policies/{id}", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) {
		s.savePolicy(w, r, actor, r.PathValue("id"))
	}))
	m.HandleFunc("DELETE /api/v1/policies/{id}", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) {
		id := r.PathValue("id")
		st.mu.Lock()
		_, found := st.Policies[id]
		delete(st.Policies, id)
		st.dirty = true
		st.mu.Unlock()
		if !found {
			fail(w, 404, "policy not found")
			return
		}
		c.InvalidateEngine(id)
		st.Audit(actor, "policy.deleted", id, "success", "", nil)
		ok(w, map[string]string{"deleted": id})
	}))
	m.HandleFunc("POST /api/v1/policies/test", s.viewer(func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Service             string  `json:"service"`
			ConsecutiveFailures int     `json:"consecutive_failures"`
			ContainerState      string  `json:"container_state"`
			Confidence          float64 `json:"confidence"`
			Action              string  `json:"action"`
		}
		if decode(r, &b) != nil {
			fail(w, 400, "invalid body")
			return
		}
		p, why := c.matchPolicy(Service{Name: b.Service, ConsecutiveFailures: b.ConsecutiveFailures, ContainerState: b.ContainerState},
			Diagnosis{Confidence: b.Confidence, RecommendedAction: b.Action})
		ok(w, map[string]any{"matched": p != nil, "policy": p, "reason": why, "executed": false})
	}))

	// chaos
	m.HandleFunc("GET /api/v1/chaos", s.viewer(func(w http.ResponseWriter, r *http.Request) {
		st.mu.RLock()
		exps := []Experiment{}
		for i := len(st.Experiments) - 1; i >= 0; i-- {
			exps = append(exps, *st.Experiments[i])
		}
		st.mu.RUnlock()
		ok(w, map[string]any{"types": ChaosTypes, "experiments": exps, "manual_baseline_s": c.cfg.ManualBaselineS,
			"manual_baseline_note": "ESTIMATE: configured assumption (REFORGE_MANUAL_BASELINE_S), not a measurement."})
	}))
	m.HandleFunc("POST /api/v1/chaos", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) {
		var b struct {
			Service   string `json:"service"`
			Type      string `json:"type"`
			DurationS int    `json:"duration_s"`
			Intensity int    `json:"intensity"`
		}
		if decode(r, &b) != nil {
			fail(w, 400, "invalid body")
			return
		}
		e, err := c.StartExperiment(b.Service, b.Type, b.DurationS, b.Intensity, actor)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		ok(w, e)
	}))
	m.HandleFunc("POST /api/v1/chaos/{id}/stop", s.admin(func(w http.ResponseWriter, r *http.Request, actor string) {
		_ = c.StopExperiment(r.PathValue("id"), actor)
		ok(w, map[string]string{"status": "stopped"})
	}))

	m.HandleFunc("GET /api/v1/audit", s.viewer(s.audit))
	m.HandleFunc("GET /api/v1/reports/summary", s.viewer(s.reportSummary))
	m.HandleFunc("GET /api/v1/reports/incidents/{id}", s.viewer(s.incidentReport))
}

func (s *Server) mutateIncident(w http.ResponseWriter, r *http.Request, actor, action string, f func(*Incident) error) {
	st := s.c.St
	id := r.PathValue("id")
	st.mu.Lock()
	i := st.incidentByID(id)
	if i == nil {
		st.mu.Unlock()
		fail(w, 404, "incident not found")
		return
	}
	err := f(i)
	st.dirty = true
	copyI := *i
	st.mu.Unlock()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	st.Audit(actor, action, id, "success", id, nil)
	st.Publish("incident", map[string]string{"id": id, "status": copyI.Status})
	ok(w, copyI)
}

func validURL(u string) bool {
	p, err := url.Parse(u)
	return err == nil && (p.Scheme == "http" || p.Scheme == "https") && p.Host != ""
}

func (s *Server) createService(w http.ResponseWriter, r *http.Request, actor string) {
	var v Service
	if err := decode(r, &v); err != nil {
		fail(w, 400, "invalid body: "+err.Error())
		return
	}
	if !nameRe.MatchString(v.Name) {
		fail(w, 400, "name must be 1-63 chars: letters, digits, '.', '_' or '-'")
		return
	}
	if !validURL(v.HealthURL) {
		fail(w, 400, "health_url must be a valid http(s) URL")
		return
	}
	for _, u := range []string{v.BaseURL, v.HookURL, v.ControlURL} {
		if u != "" && !validURL(u) {
			fail(w, 400, "base_url, hook_url and control_url must be valid http(s) URLs")
			return
		}
	}
	if v.ExpectedStatus == 0 {
		v.ExpectedStatus = 200
	}
	if v.TimeoutMS == 0 {
		v.TimeoutMS = 2000
	}
	if v.IntervalS == 0 {
		v.IntervalS = 10
	}
	if v.ExpectedStatus < 100 || v.ExpectedStatus > 599 || v.TimeoutMS < 100 || v.TimeoutMS > 30000 || v.IntervalS < 1 || v.IntervalS > 3600 {
		fail(w, 400, "expected_status 100-599, timeout_ms 100-30000, interval_s 1-3600")
		return
	}
	if v.Container != "" && !nameRe.MatchString(v.Container) {
		fail(w, 400, "invalid container name")
		return
	}
	if v.Kind == "" {
		v.Kind = "application"
	}
	if v.Environment == "" {
		v.Environment = "production"
	}
	st := s.c.St
	st.mu.Lock()
	for _, e := range st.Services {
		if e.Name == v.Name {
			st.mu.Unlock()
			fail(w, 409, "a service with this name already exists")
			return
		}
	}
	v.ID, v.Status, v.LastCheck = newID("svc"), "unknown", time.Time{}
	v.ConsecutiveFailures, v.TotalFailures, v.SlowStreak, v.LastError, v.LastSuccess, v.ContainerState = 0, 0, 0, "", nil, ""
	st.Services[v.ID] = &v
	st.dirty = true
	st.mu.Unlock()
	st.Audit(actor, "service.registered", v.Name, "success", "", nil)
	ok(w, v)
}

var allowedPerms = map[string]bool{"metrics.read": true, "containers.inspect": true, "containers.restart": true}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request, actor string) {
	var b struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
		Targets     []string `json:"targets"`
	}
	if decode(r, &b) != nil || !nameRe.MatchString(b.Name) {
		fail(w, 400, "valid name required")
		return
	}
	for _, p := range b.Permissions {
		if !allowedPerms[p] {
			fail(w, 400, "unknown permission "+p)
			return
		}
	}
	for _, t := range b.Targets {
		if !nameRe.MatchString(t) {
			fail(w, 400, "invalid target "+t)
			return
		}
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	secret := hex.EncodeToString(raw)
	a := &Agent{ID: newID("agent"), Name: b.Name, SecretHash: HashSecret(secret), Permissions: b.Permissions,
		Targets: b.Targets, CreatedAt: time.Now().UTC()}
	s.c.St.mu.Lock()
	s.c.St.Agents[a.ID] = a
	s.c.St.dirty = true
	s.c.St.mu.Unlock()
	s.c.St.Audit(actor, "agent.registered", a.ID, "success", "", nil)
	cp := *a
	cp.SecretHash = ""
	ok(w, map[string]any{"agent": cp, "secret": secret, "note": "Store this secret now. It is never shown again."})
}

func (s *Server) savePolicy(w http.ResponseWriter, r *http.Request, actor, id string) {
	var p Policy
	if err := decode(r, &p); err != nil {
		fail(w, 400, "invalid body: "+err.Error())
		return
	}
	st := s.c.St
	st.mu.Lock()
	err := s.validatePolicyLocked(&p, id)
	if err != nil {
		st.mu.Unlock()
		fail(w, 422, err.Error())
		return
	}
	action := "policy.updated"
	if id == "" {
		p.ID, action = newID("pol"), "policy.created"
	} else {
		p.ID = id
	}
	st.Policies[p.ID] = &p
	st.dirty = true
	st.mu.Unlock()
	s.c.InvalidateEngine(p.ID)
	st.Audit(actor, action, p.Name, "success", "", map[string]string{"mode": p.Mode, "action": p.ActionID})
	ok(w, p)
}

func (s *Server) validatePolicyLocked(p *Policy, selfID string) error {
	switch {
	case strings.TrimSpace(p.Name) == "":
		return fmt.Errorf("name is required")
	case !ValidActions[p.ActionID]:
		return fmt.Errorf("action_id must be one of the allowlisted actions: restart_container, app_recovery_hook")
	case p.Mode != "simulate" && p.Mode != "approval" && p.Mode != "auto":
		return fmt.Errorf("mode must be simulate, approval or auto")
	case p.RetryLimit < 1 || p.RetryLimit > 10:
		return fmt.Errorf("retry_limit must be 1-10")
	case p.CooldownS < 0 || p.CooldownS > 3600:
		return fmt.Errorf("cooldown_s must be 0-3600")
	case p.VerifyTimeoutS < 5 || p.VerifyTimeoutS > 300:
		return fmt.Errorf("verify_timeout_s must be 5-300")
	case p.MinFailures < 1 || p.MinFailures > 20:
		return fmt.Errorf("min_failures must be 1-20")
	case p.Priority < 1 || p.Priority > 1000:
		return fmt.Errorf("priority must be 1-1000")
	case p.MinConfidence < 0 || p.MinConfidence > 1:
		return fmt.Errorf("min_confidence must be between 0 and 1")
	}
	if p.Service != "*" {
		found := false
		for _, sv := range s.c.St.Services {
			if sv.Name == p.Service {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("service %q is not registered (use \"*\" for all)", p.Service)
		}
	}
	if p.Enabled {
		for _, o := range s.c.St.Policies {
			if o.ID != selfID && o.Enabled && o.Service == p.Service && o.ActionID == p.ActionID && o.Priority == p.Priority {
				return fmt.Errorf("conflicts with policy %q (same service, action and priority)", o.Name)
			}
		}
	}
	return nil
}

// ---------- read endpoints ----------

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	st := s.c.St
	st.mu.RLock()
	defer st.mu.RUnlock()
	d := map[string]any{}
	var agentsC, agentsT, h, dg, f, u, active, inprog, okOps, badOps int
	for _, a := range st.Agents {
		if a.Revoked {
			continue
		}
		agentsT++
		if a.Connected {
			agentsC++
		}
	}
	for _, v := range st.Services {
		switch v.Status {
		case "healthy":
			h++
		case "degraded":
			dg++
		case "failed":
			f++
		default:
			u++
		}
	}
	var sumRec float64
	var nRec int
	for _, i := range st.Incidents {
		if i.Status != "resolved" {
			active++
		} else if i.ResolvedAt != nil && strings.HasPrefix(i.DedupKey, "svc:") {
			sumRec += i.ResolvedAt.Sub(i.DetectedAt).Seconds()
			nRec++
		}
	}
	for _, o := range st.Operations {
		switch o.State {
		case "executing", "verifying", "validating":
			inprog++
		case "successful":
			okOps++
		case "failed", "rolled_back", "escalated":
			badOps++
		}
	}
	d["agents_total"], d["agents_connected"] = agentsT, agentsC
	d["services"] = map[string]int{"healthy": h, "degraded": dg, "failed": f, "unknown": u, "total": len(st.Services)}
	d["active_incidents"], d["recoveries_in_progress"] = active, inprog
	d["successful_recoveries"], d["failed_recoveries"] = okOps, badOps
	if nRec > 0 {
		d["avg_recovery_seconds"] = sumRec / float64(nRec)
	} else {
		d["avg_recovery_seconds"] = nil
	}
	d["kill_switch"] = st.KillSwitch
	n := len(st.AuditLog)
	start := n - 25
	if start < 0 {
		start = 0
	}
	feed := []*Audit{}
	for i := n - 1; i >= start; i-- {
		feed = append(feed, st.AuditLog[i])
	}
	d["activity"] = feed
	ok(w, d)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, okf := w.(http.Flusher)
	if !okf {
		fail(w, 500, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	ch := s.c.St.Subscribe()
	defer s.c.St.Unsubscribe(ch)
	tk := time.NewTicker(15 * time.Second)
	defer tk.Stop()
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-tk.C:
			fmt.Fprint(w, ": keepalive\n\n")
			fl.Flush()
		}
	}
}

type topoNode struct {
	ID     string         `json:"id"`
	Kind   string         `json:"kind"`
	Label  string         `json:"label"`
	Status string         `json:"status"`
	Detail map[string]any `json:"detail"`
}
type topoEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
}

func (s *Server) topology(w http.ResponseWriter, r *http.Request) {
	st := s.c.St
	st.mu.RLock()
	defer st.mu.RUnlock()
	nodes, edges := []topoNode{}, []topoEdge{}
	for _, v := range st.Services {
		open := 0
		for _, i := range st.Incidents {
			if i.Service == v.Name && i.Status != "resolved" {
				open++
			}
		}
		nodes = append(nodes, topoNode{ID: "svc:" + v.Name, Kind: v.Kind, Label: v.Name, Status: v.Status, Detail: map[string]any{
			"latency_ms": v.LastLatencyMS, "consecutive_failures": v.ConsecutiveFailures, "total_failures": v.TotalFailures,
			"open_incidents": open, "container_state": v.ContainerState}})
		for _, d := range v.DependsOn {
			edges = append(edges, topoEdge{Source: "svc:" + v.Name, Target: "svc:" + d, Kind: "depends_on"})
		}
		if v.Container != "" {
			cs := v.ContainerState
			if cs == "" {
				cs = "unknown"
			}
			stat := "healthy"
			if cs != "running" {
				stat = "failed"
				if cs == "unknown" {
					stat = "unknown"
				}
			}
			nodes = append(nodes, topoNode{ID: "ctr:" + v.Container, Kind: "container", Label: v.Container, Status: stat,
				Detail: map[string]any{"state": cs}})
			edges = append(edges, topoEdge{Source: "ctr:" + v.Container, Target: "svc:" + v.Name, Kind: "runs"})
		}
	}
	ok(w, map[string]any{"nodes": nodes, "edges": edges})
}

func intParam(r *http.Request, k string, d int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(k)); err == nil && v > 0 {
		return v
	}
	return d
}

func (s *Server) listIncidents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status, sev, text, sortBy := q.Get("status"), q.Get("severity"), strings.ToLower(q.Get("q")), q.Get("sort")
	st := s.c.St
	st.mu.RLock()
	items := []Incident{}
	for _, i := range st.Incidents {
		if status != "" && i.Status != status || sev != "" && i.Severity != sev {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(i.Title+" "+i.Service+" "+i.ID), text) {
			continue
		}
		cp := *i
		cp.Events, cp.Notes, cp.Diagnoses = nil, nil, nil // list view stays light
		items = append(items, cp)
	}
	st.mu.RUnlock()
	sort.Slice(items, func(a, b int) bool {
		if sortBy == "severity" {
			rank := map[string]int{"critical": 0, "warning": 1, "info": 2}
			return rank[items[a].Severity] < rank[items[b].Severity]
		}
		return items[a].DetectedAt.After(items[b].DetectedAt)
	})
	page, size := intParam(r, "page", 1), intParam(r, "page_size", 25)
	if size > 100 {
		size = 100
	}
	total := len(items)
	lo := (page - 1) * size
	if lo > total {
		lo = total
	}
	hi := lo + size
	if hi > total {
		hi = total
	}
	ok(w, map[string]any{"items": items[lo:hi], "total": total, "page": page, "page_size": size})
}

func (s *Server) filteredAudit(r *http.Request) []*Audit {
	q := r.URL.Query()
	actor, action, text := q.Get("actor"), q.Get("action"), strings.ToLower(q.Get("q"))
	from, _ := time.Parse(time.RFC3339, q.Get("from"))
	to, _ := time.Parse(time.RFC3339, q.Get("to"))
	st := s.c.St
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := []*Audit{}
	for i := len(st.AuditLog) - 1; i >= 0; i-- {
		a := st.AuditLog[i]
		if actor != "" && a.Actor != actor || action != "" && !strings.HasPrefix(a.Action, action) {
			continue
		}
		if !from.IsZero() && a.TS.Before(from) || !to.IsZero() && a.TS.After(to) {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(a.Action+" "+a.Target+" "+a.Actor+" "+a.IncidentID), text) {
			continue
		}
		out = append(out, a)
	}
	return out
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	all := s.filteredAudit(r)
	if r.URL.Query().Get("format") == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="reforge-audit.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"id", "timestamp", "actor", "action", "target", "outcome", "incident_id", "metadata"})
		for _, a := range all {
			md, _ := json.Marshal(a.Metadata)
			_ = cw.Write([]string{strconv.FormatInt(a.ID, 10), a.TS.Format(time.RFC3339), a.Actor, a.Action, a.Target, a.Outcome, a.IncidentID, string(md)})
		}
		cw.Flush()
		return
	}
	page, size := intParam(r, "page", 1), intParam(r, "page_size", 50)
	if size > 200 {
		size = 200
	}
	lo := (page - 1) * size
	if lo > len(all) {
		lo = len(all)
	}
	hi := lo + size
	if hi > len(all) {
		hi = len(all)
	}
	ok(w, map[string]any{"items": all[lo:hi], "total": len(all), "page": page, "page_size": size})
}

func (s *Server) reportSummary(w http.ResponseWriter, r *http.Request) {
	from, _ := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, _ := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	st := s.c.St
	st.mu.RLock()
	defer st.mu.RUnlock()
	in := func(t time.Time) bool { return (from.IsZero() || !t.Before(from)) && (to.IsZero() || !t.After(to)) }
	byService, byType, byPolicy := map[string]int{}, map[string]int{}, map[string]int{}
	var total, resolved int
	var sumRec float64
	var nRec int
	for _, i := range st.Incidents {
		if !in(i.DetectedAt) {
			continue
		}
		total++
		byService[i.Service]++
		if k := strings.Split(i.DedupKey, ":"); len(k) > 0 {
			byType[k[len(k)-1]]++
		}
		if i.Status == "resolved" {
			resolved++
			if i.ResolvedAt != nil {
				sumRec += i.ResolvedAt.Sub(i.DetectedAt).Seconds()
				nRec++
			}
		}
	}
	var okOps, badOps int
	for _, o := range st.Operations {
		if !in(o.StartedAt) {
			continue
		}
		if o.State == "successful" {
			okOps++
			byPolicy[o.PolicyName]++
		} else if o.State == "failed" || o.State == "rolled_back" || o.State == "escalated" {
			badOps++
		}
	}
	var sumDet, sumAuto float64
	var nDet, nAuto int
	for _, e := range st.Experiments {
		if !in(e.InjectedAt) {
			continue
		}
		if e.DetectedAt != nil {
			sumDet += e.DetectedAt.Sub(e.InjectedAt).Seconds()
			nDet++
		}
		if e.RecoveredAt != nil {
			sumAuto += e.RecoveredAt.Sub(e.InjectedAt).Seconds()
			nAuto++
		}
	}
	avg := func(sum float64, n int) any {
		if n == 0 {
			return nil
		}
		return sum / float64(n)
	}
	rate := any(nil)
	if okOps+badOps > 0 {
		rate = float64(okOps) / float64(okOps+badOps)
	}
	ok(w, map[string]any{
		"total_incidents": total, "resolved_incidents": resolved, "successful_recoveries": okOps, "failed_recoveries": badOps,
		"recovery_success_rate": rate, "avg_time_to_detect_s": avg(sumDet, nDet) /* chaos experiments only */,
		"avg_time_to_recover_s": avg(sumRec, nRec), "chaos_avg_automated_recovery_s": avg(sumAuto, nAuto),
		"manual_baseline_s": s.c.cfg.ManualBaselineS, "manual_baseline_is_estimate": true,
		"most_affected_services": byService, "failure_types": byType, "effective_policies": byPolicy,
		"note": "avg_time_to_detect_s is measured from chaos-experiment injection, since real failure start times are unknown.",
	})
}

func (s *Server) incidentReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	st := s.c.St
	st.mu.RLock()
	inc := st.incidentByID(id)
	if inc == nil {
		st.mu.RUnlock()
		fail(w, 404, "incident not found")
		return
	}
	cp := *inc
	ops := []Operation{}
	for _, o := range st.Operations {
		if o.IncidentID == id {
			ops = append(ops, *o)
		}
	}
	trail := []Audit{}
	for _, a := range st.AuditLog {
		if a.IncidentID == id {
			trail = append(trail, *a)
		}
	}
	st.mu.RUnlock()
	if r.URL.Query().Get("format") == "pdf" {
		lines := []string{"Incident: " + cp.Title, "ID: " + cp.ID, "Service: " + cp.Service, "Severity: " + cp.Severity + "   Status: " + cp.Status,
			"Detected: " + cp.DetectedAt.Format(time.RFC3339), "Summary: " + cp.Summary, "", "EVIDENCE"}
		for _, e := range cp.Evidence {
			lines = append(lines, " - "+e)
		}
		lines = append(lines, "", "DIAGNOSIS (suspected unless marked confirmed)")
		for _, d := range cp.Diagnoses {
			lines = append(lines, fmt.Sprintf(" - %s [%s] confidence %.2f, recommended: %s", d.ProbableCause, d.Status, d.Confidence, d.RecommendedAction))
		}
		lines = append(lines, "", "RECOVERY OPERATIONS")
		for _, o := range ops {
			lines = append(lines, fmt.Sprintf(" - %s on %s: %s %s", o.ActionID, o.Target, o.State, o.Error))
		}
		lines = append(lines, "", "TIMELINE")
		for _, e := range cp.Events {
			lines = append(lines, fmt.Sprintf(" %s  %s: %s", e.TS.Format("15:04:05"), e.Kind, e.Detail))
		}
		lines = append(lines, "", "AUDIT TRAIL")
		for _, a := range trail {
			lines = append(lines, fmt.Sprintf(" %s  %s  %s  %s", a.TS.Format("15:04:05"), a.Actor, a.Action, a.Outcome))
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="incident-`+id+`.pdf"`)
		_, _ = w.Write(simplePDF("REFORGE Incident Report", lines))
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="incident-`+id+`.json"`)
	ok(w, map[string]any{"incident": cp, "operations": ops, "audit_trail": trail, "generated_at": time.Now().UTC()})
}
