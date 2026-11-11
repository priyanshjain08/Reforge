package core

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestCore(t *testing.T) *Core {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return New(ctx, Config{AdminToken: "admin-token-1234567890", ViewerToken: "viewer-token", IntelligenceURL: "http://127.0.0.1:1",
		RecoveryMode: "approval", DemoAgentSecret: "s3cret", ManualBaselineS: 300})
}

func addService(c *Core, name string, failures int) {
	c.St.mu.Lock()
	c.St.Services["svc_"+name] = &Service{ID: "svc_" + name, Name: name, Status: "failed", ConsecutiveFailures: failures,
		ExpectedStatus: 200, TimeoutMS: 500, IntervalS: 5, Container: name, HookURL: "http://x/recover"}
	c.St.mu.Unlock()
}

func do(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthAndRoles(t *testing.T) {
	c := newTestCore(t)
	h := NewServer(c).Handler()
	if r := do(h, "GET", "/api/v1/dashboard", "", ""); r.Code != 401 {
		t.Fatalf("no token: got %d", r.Code)
	}
	if r := do(h, "GET", "/api/v1/dashboard", "wrong", ""); r.Code != 401 {
		t.Fatalf("bad token: got %d", r.Code)
	}
	if r := do(h, "GET", "/api/v1/dashboard", "viewer-token", ""); r.Code != 200 {
		t.Fatalf("viewer read: got %d", r.Code)
	}
	if r := do(h, "POST", "/api/v1/killswitch", "viewer-token", `{"enabled":true}`); r.Code != 403 {
		t.Fatalf("viewer must not mutate: got %d", r.Code)
	}
	if r := do(h, "POST", "/api/v1/killswitch", "admin-token-1234567890", `{"enabled":true}`); r.Code != 200 || !c.killSwitch() {
		t.Fatalf("admin kill switch: got %d", r.Code)
	}
}

func TestAgentAuthAndTelemetry(t *testing.T) {
	c := newTestCore(t)
	c.St.mu.Lock()
	c.St.Agents["agent-test"] = &Agent{ID: "agent-test", SecretHash: HashSecret("good"), Targets: []string{"a"}}
	c.St.mu.Unlock()
	h := NewServer(c).Handler()
	post := func(secret string) int {
		req := httptest.NewRequest("POST", "/api/v1/agent/telemetry", bytes.NewBufferString(`{"hostname":"h","metrics":{"cpu_percent":12.5}}`))
		req.Header.Set("X-Agent-ID", "agent-test")
		req.Header.Set("X-Agent-Secret", secret)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if post("bad") != 401 {
		t.Fatal("bad secret must be rejected")
	}
	if post("good") != 200 {
		t.Fatal("good secret must be accepted")
	}
	c.St.mu.RLock()
	defer c.St.mu.RUnlock()
	if a := c.St.Agents["agent-test"]; !a.Connected || a.Metrics["cpu_percent"] != 12.5 {
		t.Fatalf("telemetry not recorded: %+v", a)
	}
}

func TestRevokedAgentRejected(t *testing.T) {
	c := newTestCore(t)
	c.St.mu.Lock()
	c.St.Agents["a"] = &Agent{ID: "a", SecretHash: HashSecret("k"), Revoked: true}
	c.St.mu.Unlock()
	req := httptest.NewRequest("GET", "/api/v1/agent/commands", nil)
	req.Header.Set("X-Agent-ID", "a")
	req.Header.Set("X-Agent-Secret", "k")
	rec := httptest.NewRecorder()
	NewServer(c).Handler().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("revoked agent got %d", rec.Code)
	}
}

func TestIncidentDeduplication(t *testing.T) {
	c := newTestCore(t)
	addService(c, "payment", 3)
	c.evaluate("payment")
	c.evaluate("payment")
	c.evaluate("payment")
	c.St.mu.RLock()
	n := len(c.St.Incidents)
	c.St.mu.RUnlock()
	if n != 1 {
		t.Fatalf("expected 1 deduplicated incident, got %d", n)
	}
}

func TestHostAlertCreatesAndResolvesIncident(t *testing.T) {
	c := newTestCore(t)
	c.St.mu.Lock()
	c.St.Agents["h"] = &Agent{ID: "h", Hostname: "box"}
	c.St.mu.Unlock()
	hi, lo := 95.0, 20.0
	c.Ingest("h", Telemetry{Hostname: "box", Metrics: map[string]*float64{"cpu_percent": &hi}})
	c.St.mu.RLock()
	if len(c.St.Incidents) != 1 || c.St.Incidents[0].Status != "escalated" {
		c.St.mu.RUnlock()
		t.Fatal("expected one escalated host incident")
	}
	c.St.mu.RUnlock()
	c.Ingest("h", Telemetry{Hostname: "box", Metrics: map[string]*float64{"cpu_percent": &lo}})
	c.St.mu.RLock()
	defer c.St.mu.RUnlock()
	if c.St.Incidents[0].Status != "resolved" {
		t.Fatal("incident should resolve when metric recovers")
	}
}

func TestMatchPolicy(t *testing.T) {
	c := newTestCore(t)
	svc := Service{Name: "payment", ConsecutiveFailures: 3, ContainerState: "exited"}
	p, _ := c.matchPolicy(svc, Diagnosis{RecommendedAction: "restart_container", Confidence: 0.9})
	if p == nil || p.ActionID != "restart_container" {
		t.Fatal("expected restart policy to match")
	}
	if p, why := c.matchPolicy(svc, Diagnosis{RecommendedAction: "restart_container", Confidence: 0.1}); p != nil || why == "" {
		t.Fatal("low confidence must not match")
	}
	svc.ContainerState = "running"
	if p, _ := c.matchPolicy(svc, Diagnosis{RecommendedAction: "restart_container", Confidence: 0.9}); p != nil {
		t.Fatal("running container must not be restarted by the default policy")
	}
	if p, _ := c.matchPolicy(Service{Name: "x", ConsecutiveFailures: 1}, Diagnosis{RecommendedAction: "restart_container", Confidence: 0.9}); p != nil {
		t.Fatal("too few failures must not match")
	}
	if p, _ := c.matchPolicy(svc, Diagnosis{RecommendedAction: "rm -rf", Confidence: 1}); p != nil {
		t.Fatal("unknown action must never match")
	}
}

func TestPolicyValidationAndConflicts(t *testing.T) {
	c := newTestCore(t)
	h := NewServer(c).Handler()
	tok := "admin-token-1234567890"
	good := `{"name":"n","service":"*","min_failures":3,"container_states":[],"min_confidence":0.5,"action_id":"restart_container","mode":"simulate","retry_limit":3,"cooldown_s":10,"verify_timeout_s":30,"priority":10,"enabled":true}`
	if r := do(h, "POST", "/api/v1/policies", tok, good); r.Code != 422 { // seeded policy already uses priority 10
		t.Fatalf("expected conflict 422, got %d: %s", r.Code, r.Body.String())
	}
	if r := do(h, "POST", "/api/v1/policies", tok, strings.Replace(good, `"priority":10`, `"priority":77`, 1)); r.Code != 200 {
		t.Fatalf("expected success, got %d: %s", r.Code, r.Body.String())
	}
	if r := do(h, "POST", "/api/v1/policies", tok, strings.Replace(good, `"restart_container"`, `"format_disk"`, 1)); r.Code != 422 {
		t.Fatal("non-allowlisted action must be rejected")
	}
	if r := do(h, "POST", "/api/v1/policies", tok, strings.Replace(good, `"mode":"simulate"`, `"mode":"yolo"`, 1)); r.Code != 422 {
		t.Fatal("invalid mode must be rejected")
	}
}

func TestServiceRegistrationValidation(t *testing.T) {
	c := newTestCore(t)
	h := NewServer(c).Handler()
	tok := "admin-token-1234567890"
	if r := do(h, "POST", "/api/v1/services", tok, `{"name":"ok-svc","health_url":"http://a:1/health"}`); r.Code != 200 {
		t.Fatalf("valid service rejected: %s", r.Body.String())
	}
	if r := do(h, "POST", "/api/v1/services", tok, `{"name":"ok-svc","health_url":"http://a:1/health"}`); r.Code != 409 {
		t.Fatal("duplicate name must conflict")
	}
	if r := do(h, "POST", "/api/v1/services", tok, `{"name":"bad","health_url":"file:///etc/passwd"}`); r.Code != 400 {
		t.Fatal("non-http URL must be rejected")
	}
	if r := do(h, "POST", "/api/v1/services", tok, `{"name":"../x","health_url":"http://a/h"}`); r.Code != 400 {
		t.Fatal("bad name must be rejected")
	}
}

func TestAuditIsAppendOnlyAndFilterable(t *testing.T) {
	c := newTestCore(t)
	c.St.Audit("admin", "policy.created", "p1", "success", "", nil)
	c.St.Audit("system", "incident.detected", "svc", "success", "inc1", nil)
	h := NewServer(c).Handler()
	r := do(h, "GET", "/api/v1/audit?action=policy.", "viewer-token", "")
	var out struct{ Data struct{ Total int } }
	_ = json.Unmarshal(r.Body.Bytes(), &out)
	if out.Data.Total != 1 {
		t.Fatalf("expected 1 filtered entry, got %d", out.Data.Total)
	}
	if r := do(h, "DELETE", "/api/v1/audit", "admin-token-1234567890", ""); r.Code == 200 {
		t.Fatal("audit log must not be deletable via the API")
	}
	if csv := do(h, "GET", "/api/v1/audit?format=csv", "viewer-token", ""); !strings.Contains(csv.Body.String(), "incident.detected") {
		t.Fatal("csv export missing entries")
	}
}

func TestPDFIsWellFormed(t *testing.T) {
	b := simplePDF("T", []string{"hello (world)", strings.Repeat("long ", 80)})
	if !bytes.HasPrefix(b, []byte("%PDF-1.4")) || !bytes.HasSuffix(b, []byte("%%EOF")) {
		t.Fatal("malformed pdf envelope")
	}
	if !bytes.Contains(b, []byte(`hello \(world\)`)) {
		t.Fatal("parentheses must be escaped")
	}
}

func TestIncidentReportEndpoints(t *testing.T) {
	c := newTestCore(t)
	c.St.mu.Lock()
	c.St.Incidents = append(c.St.Incidents, &Incident{ID: "inc_x", Service: "s", Title: "T", Severity: "critical", Status: "open", DetectedAt: time.Now()})
	c.St.mu.Unlock()
	h := NewServer(c).Handler()
	if r := do(h, "GET", "/api/v1/reports/incidents/inc_x?format=pdf", "viewer-token", ""); r.Code != 200 || r.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("pdf report failed: %d", r.Code)
	}
	if r := do(h, "GET", "/api/v1/reports/incidents/nope", "viewer-token", ""); r.Code != 404 {
		t.Fatal("unknown incident must 404")
	}
}
