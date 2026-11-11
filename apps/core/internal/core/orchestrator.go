package core

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/reforge/core/internal/recovery"
)

type ctxKey string

const opKey ctxKey = "op"

var ValidActions = map[string]bool{"restart_container": true, "app_recovery_hook": true}

func (c *Core) incEvent(id, kind, detail string) {
	c.St.mu.Lock()
	if i := c.St.incidentByID(id); i != nil {
		i.addEvent(kind, detail)
	}
	c.St.dirty = true
	c.St.mu.Unlock()
}

func (c *Core) setIncStatus(id, status string) {
	c.St.mu.Lock()
	if i := c.St.incidentByID(id); i != nil && i.Status != "resolved" {
		i.Status = status
	}
	c.St.dirty = true
	c.St.mu.Unlock()
	c.St.Publish("incident", map[string]string{"id": id, "status": status})
}

func (c *Core) escalate(incID, reason string) {
	c.St.mu.Lock()
	if i := c.St.incidentByID(incID); i != nil && i.Status != "resolved" {
		i.Status = "escalated"
		i.Summary = reason
		i.addEvent("escalated", reason)
	}
	c.St.dirty = true
	c.St.mu.Unlock()
	c.St.Audit("system", "incident.escalated", incID, "warning", incID, map[string]string{"reason": reason})
	c.St.Publish("incident", map[string]string{"id": incID, "status": "escalated"})
}

func (c *Core) runIncident(incID string) {
	c.St.mu.RLock()
	inc := c.St.incidentByID(incID)
	var svcName string
	if inc != nil {
		svcName = inc.Service
	}
	c.St.mu.RUnlock()
	if svcName == "" {
		return
	}
	c.setIncStatus(incID, "investigating")
	c.incEvent(incID, "diagnosis_started", "Collecting evidence and running diagnosis")

	diag := c.diagnose(c.ctx, svcName)
	c.St.mu.Lock()
	if i := c.St.incidentByID(incID); i != nil {
		i.Diagnoses = append(i.Diagnoses, diag)
		i.addEvent("diagnosis", fmt.Sprintf("%s [%s, confidence %.2f, source %s]; recommended: %s",
			diag.ProbableCause, diag.Status, diag.Confidence, diag.Source, diag.RecommendedAction))
	}
	for _, e := range c.St.Experiments {
		if e.IncidentID == incID && e.DiagnosedAt == nil {
			t := time.Now().UTC()
			e.DiagnosedAt = &t
		}
	}
	c.St.dirty = true
	c.St.mu.Unlock()
	c.St.Audit("intelligence", "diagnosis.completed", svcName, "success", incID, map[string]string{"status": diag.Status, "action": diag.RecommendedAction})

	if diag.RecommendedAction == "" || diag.RecommendedAction == "none" || diag.RecommendedAction == "escalate" {
		c.escalate(incID, "Diagnosis recommends human investigation: "+diag.ProbableCause)
		return
	}
	svc := c.serviceByName(svcName)
	if svc == nil {
		return
	}
	pol, why := c.matchPolicy(*svc, diag)
	if pol == nil {
		c.escalate(incID, "No enabled policy permits an automatic response ("+why+")")
		return
	}
	c.incEvent(incID, "policy_matched", fmt.Sprintf("policy %q (%s mode) selected action %s", pol.Name, pol.Mode, pol.ActionID))
	c.executeRecovery(incID, *pol, *svc, false, nil)
}

func (c *Core) diagnose(ctx context.Context, svcName string) Diagnosis {
	svc := c.serviceByName(svcName)
	dctx := map[string]any{"service": svcName}
	if svc != nil {
		dctx["consecutive_failures"] = svc.ConsecutiveFailures
		dctx["latency_ms"] = svc.LastLatencyMS
		restarts := 0
		if svc.Container != "" {
			if ci := c.containerInfo(svc.Container); ci != nil {
				dctx["container_state"] = ci.State
				dctx["exit_code"] = ci.ExitCode
				restarts = ci.Restarts
			}
		}
		c.St.mu.RLock()
		for _, o := range c.St.Operations {
			if o.Service == svcName && time.Since(o.StartedAt) < 10*time.Minute && o.State != "cancelled" && o.State != "awaiting_approval" {
				restarts++
			}
		}
		for _, a := range c.St.Agents {
			if v, ok := a.Metrics["cpu_percent"]; ok {
				dctx["cpu"] = v
			}
			if v, ok := a.Metrics["memory_percent"]; ok {
				dctx["memory"] = v
			}
		}
		c.St.mu.RUnlock()
		dctx["container_restarts"] = restarts
	}
	var d Diagnosis
	if err := postJSON(ctx, c.cfg.IntelligenceURL+"/v1/diagnose", map[string]any{"context": dctx}, &d); err != nil {
		return Diagnosis{ProbableCause: "unknown (diagnostic engine unavailable)", AffectedComponent: svcName, Status: "suspected",
			Evidence: []string{"intelligence service error: " + err.Error()}, Confidence: 0.1, RecommendedAction: "escalate",
			Explanation: "No diagnosis could be produced, so no automatic action is proposed.", Source: "fallback", CreatedAt: time.Now().UTC()}
	}
	d.Source, d.CreatedAt = "intelligence", time.Now().UTC()
	return d
}

// matchPolicy picks the highest-priority enabled policy whose conditions hold. Pure read; used for dry-runs too.
func (c *Core) matchPolicy(svc Service, d Diagnosis) (*Policy, string) {
	c.St.mu.RLock()
	var list []*Policy
	for _, p := range c.St.Policies {
		list = append(list, p)
	}
	c.St.mu.RUnlock()
	sort.Slice(list, func(i, j int) bool { return list[i].Priority < list[j].Priority })
	why := "no policy for action " + d.RecommendedAction
	for _, p := range list {
		if !p.Enabled || p.ActionID != d.RecommendedAction || (p.Service != "*" && p.Service != svc.Name) {
			continue
		}
		if svc.ConsecutiveFailures < p.MinFailures {
			why = fmt.Sprintf("policy %q needs %d failures, have %d", p.Name, p.MinFailures, svc.ConsecutiveFailures)
			continue
		}
		if d.Confidence < p.MinConfidence {
			why = fmt.Sprintf("policy %q needs confidence %.2f, diagnosis has %.2f", p.Name, p.MinConfidence, d.Confidence)
			continue
		}
		if len(p.ContainerStates) > 0 && !contains(p.ContainerStates, svc.ContainerState) {
			why = fmt.Sprintf("policy %q requires container state in %v, observed %q", p.Name, p.ContainerStates, svc.ContainerState)
			continue
		}
		cp := *p
		return &cp, ""
	}
	return nil, why
}

func (c *Core) engineFor(p Policy) *recovery.Engine {
	c.engMu.Lock()
	defer c.engMu.Unlock()
	e := c.engines[p.ID]
	if e == nil {
		e = recovery.NewEngine(recovery.Config{Mode: recovery.Mode(p.Mode), MaxRetries: p.RetryLimit,
			Cooldown: time.Duration(p.CooldownS) * time.Second, ExecTimeout: 120 * time.Second,
			Audit: func(event string, req recovery.Request, detail string) {
				outcome := "success"
				if event != "recovery.successful" {
					outcome = "warning"
				}
				c.St.Audit("recovery-engine", event, req.Target.Kind+"/"+req.Target.Name, outcome, req.IncidentID,
					map[string]string{"action": req.ActionID, "detail": detail})
			}})
		c.registerActions(e, p)
		c.engines[p.ID] = e
	}
	e.SetKillSwitch(c.killSwitch())
	return e
}

func (c *Core) InvalidateEngine(policyID string) {
	c.engMu.Lock()
	delete(c.engines, policyID)
	c.engMu.Unlock()
}

func (c *Core) killSwitch() bool {
	c.St.mu.RLock()
	defer c.St.mu.RUnlock()
	return c.St.KillSwitch
}

func (c *Core) SetKillSwitch(on bool, actor string) {
	c.St.mu.Lock()
	c.St.KillSwitch = on
	c.St.dirty = true
	c.St.mu.Unlock()
	c.engMu.Lock()
	for _, e := range c.engines {
		e.SetKillSwitch(on)
	}
	c.engMu.Unlock()
	state := "disabled"
	if on {
		state = "enabled"
	}
	c.St.Audit(actor, "killswitch."+state, "recovery-engine", "success", "", nil)
	c.St.Publish("killswitch", map[string]bool{"enabled": on})
}

func (c *Core) opUpdate(ctx context.Context, f func(o *Operation)) {
	id, _ := ctx.Value(opKey).(string)
	c.St.mu.Lock()
	for _, o := range c.St.Operations {
		if o.ID == id {
			f(o)
		}
	}
	c.St.dirty = true
	c.St.mu.Unlock()
	c.St.Publish("recovery", map[string]string{"id": id})
}

func (c *Core) incOfOp(ctx context.Context) string {
	id, _ := ctx.Value(opKey).(string)
	c.St.mu.RLock()
	defer c.St.mu.RUnlock()
	for _, o := range c.St.Operations {
		if o.ID == id {
			return o.IncidentID
		}
	}
	return ""
}

func (c *Core) registerActions(e *recovery.Engine, p Policy) {
	verifyTO := time.Duration(p.VerifyTimeoutS) * time.Second
	if verifyTO <= 0 {
		verifyTO = 30 * time.Second
	}
	_ = e.Register(recovery.ActionDef{
		ID: "restart_container", Permission: "containers.restart", Risk: recovery.RiskMedium,
		Validate: func(ctx context.Context, t recovery.Target) error {
			ci := c.containerInfo(t.Name)
			if ci == nil {
				return fmt.Errorf("container %q state unknown: no connected agent reports it", t.Name)
			}
			state := ci.State
			if ci.State == "running" && ci.Health == "unhealthy" {
				state = "unhealthy"
			}
			if len(p.ContainerStates) > 0 && !contains(p.ContainerStates, state) {
				return fmt.Errorf("container is %q; policy only allows restart when %v", state, p.ContainerStates)
			}
			return nil
		},
		Execute: func(ctx context.Context, t recovery.Target) error {
			c.opUpdate(ctx, func(o *Operation) {
				o.State = "executing"
				o.Steps = append(o.Steps, "sending restart_container to agent")
				if ci := c.containerInfo(t.Name); ci != nil {
					o.PreviousState = ci.State
				}
			})
			c.setIncStatus(c.incOfOp(ctx), "recovering")
			a := c.agentFor(t.Name)
			if a == nil {
				return fmt.Errorf("no connected agent is authorised for container %q", t.Name)
			}
			return c.waitCommand(ctx, c.enqueue(a.ID, "restart_container", t.Name))
		},
		Verify: func(ctx context.Context, t recovery.Target) error {
			c.opUpdate(ctx, func(o *Operation) { o.State = "verifying"; o.Steps = append(o.Steps, "verifying service health") })
			svcName := ""
			c.St.mu.RLock()
			for _, s := range c.St.Services {
				if s.Container == t.Name {
					svcName = s.Name
				}
			}
			c.St.mu.RUnlock()
			return c.verifyService(ctx, svcName, t.Name, verifyTO)
		},
	})
	_ = e.Register(recovery.ActionDef{
		ID: "app_recovery_hook", Permission: "applications.hook", Risk: recovery.RiskLow,
		Validate: func(ctx context.Context, t recovery.Target) error {
			s := c.serviceByName(t.Name)
			if s == nil || s.HookURL == "" {
				return fmt.Errorf("service %q has no registered recovery hook", t.Name)
			}
			return nil
		},
		Execute: func(ctx context.Context, t recovery.Target) error {
			c.opUpdate(ctx, func(o *Operation) { o.State = "executing"; o.Steps = append(o.Steps, "calling registered recovery hook") })
			c.setIncStatus(c.incOfOp(ctx), "recovering")
			s := c.serviceByName(t.Name)
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.HookURL, nil)
			resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				return fmt.Errorf("hook returned status %d", resp.StatusCode)
			}
			return nil
		},
		Verify: func(ctx context.Context, t recovery.Target) error {
			c.opUpdate(ctx, func(o *Operation) { o.State = "verifying"; o.Steps = append(o.Steps, "verifying service health") })
			return c.verifyService(ctx, t.Name, "", verifyTO)
		},
	})
}

func (c *Core) verifyService(ctx context.Context, svcName, container string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	last := ""
	for {
		svc := c.serviceByName(svcName)
		if svc == nil {
			return fmt.Errorf("service %q not found", svcName)
		}
		_, _, err := c.probe(*svc)
		cstate, containerOK := "", true
		if container != "" {
			if ci := c.containerInfo(container); ci == nil {
				cstate, containerOK = "unknown", false
			} else {
				cstate = ci.State
				containerOK = ci.State == "running" && (ci.Health == "" || ci.Health == "healthy")
			}
		}
		if err == nil && containerOK {
			c.opUpdate(ctx, func(o *Operation) { o.ResultingState = "healthy" + map[bool]string{true: ", container " + cstate, false: ""}[container != ""] })
			c.CheckService(svc.ID)
			return nil
		}
		if err != nil {
			last = err.Error()
		} else {
			last = "container state " + cstate
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service not healthy within %s (last: %s)", timeout, last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func set(l []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range l {
		m[x] = true
	}
	return m
}

func (c *Core) executeRecovery(incID string, p Policy, svc Service, approved bool, op *Operation) {
	req := recovery.Request{IncidentID: incID, ActionID: p.ActionID, Approved: approved}
	switch p.ActionID {
	case "restart_container":
		req.Target = recovery.Target{Kind: "container", Name: svc.Container}
		c.St.mu.RLock()
		for _, a := range c.St.Agents {
			if !a.Revoked && contains(a.Targets, svc.Container) {
				req.Permissions, req.Authorized = set(a.Permissions), map[string]bool{}
				for _, t := range a.Targets {
					req.Authorized["container/"+t] = true
				}
			}
		}
		c.St.mu.RUnlock()
	case "app_recovery_hook":
		req.Target = recovery.Target{Kind: "application", Name: svc.Name}
		req.Permissions = map[string]bool{"applications.hook": true}
		req.Authorized = map[string]bool{}
		if svc.HookURL != "" {
			req.Authorized["application/"+svc.Name] = true
		}
	}

	c.St.mu.Lock()
	if op == nil {
		op = &Operation{ID: newID("op"), IncidentID: incID, PolicyID: p.ID, PolicyName: p.Name, ActionID: p.ActionID,
			Target: req.Target.Name, Service: svc.Name, Mode: p.Mode, StartedAt: time.Now().UTC()}
		c.St.Operations = append(c.St.Operations, op)
	}
	op.State = "validating"
	opID := op.ID
	c.St.dirty = true
	c.St.mu.Unlock()
	c.St.Publish("recovery", map[string]string{"id": opID})

	c.St.mu.Lock()
	for _, e := range c.St.Experiments {
		if e.IncidentID == incID && e.RecoveryStartedAt == nil && (approved || p.Mode == "auto") {
			t := time.Now().UTC()
			e.RecoveryStartedAt = &t
		}
	}
	c.St.mu.Unlock()

	ctx := context.WithValue(c.ctx, opKey, opID)
	res := c.engineFor(p).Run(ctx, req)

	now := time.Now().UTC()
	c.St.mu.Lock()
	op.Steps = append(op.Steps, res.Steps...)
	if res.Err != nil && res.State != recovery.StateAwaitingApproval {
		op.Error = res.Err.Error()
	}
	switch res.State {
	case recovery.StateAwaitingApproval:
		op.State = "awaiting_approval"
	case recovery.StateSuccessful:
		op.State = "successful"
		v := true
		op.Verified, op.FinishedAt = &v, &now
	default:
		op.State = string(res.State)
		op.FinishedAt = &now
		if res.State == recovery.StateFailed || res.State == recovery.StateRolledBack || res.State == recovery.StateEscalated {
			v := false
			op.Verified = &v
		}
	}
	state := op.State
	c.St.dirty = true
	c.St.mu.Unlock()
	c.St.Publish("recovery", map[string]string{"id": opID, "state": state})

	switch res.State {
	case recovery.StateAwaitingApproval:
		c.incEvent(incID, "awaiting_approval", fmt.Sprintf("%s on %s needs approval", p.ActionID, req.Target.Name))
	case recovery.StateSuccessful:
		c.St.mu.Lock()
		if i := c.St.incidentByID(incID); i != nil && i.Status != "resolved" {
			c.resolveLocked(i, fmt.Sprintf("Recovered by %s on %s (policy %q); health verified.", p.ActionID, req.Target.Name, p.Name))
		}
		for _, e := range c.St.Experiments {
			if e.IncidentID == incID && e.RecoveredAt == nil {
				t, v := time.Now().UTC(), true
				e.RecoveredAt, e.Verified = &t, &v
			}
		}
		c.St.mu.Unlock()
		c.St.Publish("incident", map[string]string{"id": incID, "status": "resolved"})
		c.St.Audit("recovery-engine", "recovery.verified", req.Target.Kind+"/"+req.Target.Name, "success", incID, nil)
	case recovery.StateCancelled:
		if res.Err == nil {
			c.escalate(incID, fmt.Sprintf("Simulation mode: %s on %s was evaluated but NOT executed.", p.ActionID, req.Target.Name))
		} else {
			c.escalate(incID, "Recovery blocked: "+res.Err.Error())
		}
	default:
		c.escalate(incID, fmt.Sprintf("Recovery %s: %s", res.State, op.Error))
	}
}

// ApproveOperation re-runs an awaiting operation with approval granted.
func (c *Core) ApproveOperation(opID, actor string) error {
	c.St.mu.RLock()
	var op *Operation
	for _, o := range c.St.Operations {
		if o.ID == opID {
			op = o
		}
	}
	if op == nil || op.State != "awaiting_approval" {
		c.St.mu.RUnlock()
		return fmt.Errorf("operation is not awaiting approval")
	}
	pol := c.St.Policies[op.PolicyID]
	incID := op.IncidentID
	svcName := op.Service
	var pc Policy
	if pol != nil {
		pc = *pol
	}
	c.St.mu.RUnlock()
	svc := c.serviceByName(svcName)
	if pol == nil || svc == nil {
		return fmt.Errorf("policy or service no longer exists")
	}
	c.St.Audit(actor, "recovery.approved", op.Target, "success", incID, map[string]string{"operation": opID})
	go c.executeRecovery(incID, pc, *svc, true, op)
	return nil
}

func (c *Core) CancelOperation(opID, actor string) error {
	c.St.mu.Lock()
	var op *Operation
	for _, o := range c.St.Operations {
		if o.ID == opID {
			op = o
		}
	}
	if op == nil || op.State != "awaiting_approval" {
		c.St.mu.Unlock()
		return fmt.Errorf("operation is not awaiting approval")
	}
	now := time.Now().UTC()
	op.State, op.FinishedAt, op.Error = "cancelled", &now, "cancelled by "+actor
	inc := op.IncidentID
	c.St.dirty = true
	c.St.mu.Unlock()
	c.St.Audit(actor, "recovery.cancelled", op.Target, "success", inc, nil)
	c.escalate(inc, "Automatic recovery cancelled by operator")
	return nil
}

// ---------- chaos lab ----------

var ChaosTypes = map[string]string{
	"crash":           "Process crash (container exits)",
	"health_fail":     "Health-check failure (HTTP 503)",
	"error_spike":     "HTTP 500 error spike on requests",
	"slow_response":   "Slow API responses",
	"cpu_burn":        "High CPU simulation",
	"memory_pressure": "Memory pressure simulation",
}

func (c *Core) StartExperiment(service, typ string, durationS, intensity int, actor string) (*Experiment, error) {
	if _, ok := ChaosTypes[typ]; !ok {
		return nil, fmt.Errorf("unknown failure type %q", typ)
	}
	svc := c.serviceByName(service)
	if svc == nil || !svc.ChaosTarget || svc.ControlURL == "" {
		return nil, fmt.Errorf("%q is not a registered chaos-lab target", service)
	}
	if durationS < 5 || durationS > 600 {
		durationS = 60
	}
	if intensity < 1 || intensity > 100 {
		intensity = 50
	}
	url := fmt.Sprintf("%s/chaos/%s?intensity=%d&duration=%d", svc.ControlURL, typ, intensity, durationS)
	req, _ := http.NewRequestWithContext(c.ctx, http.MethodPost, url, nil)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil && typ != "crash" {
		return nil, fmt.Errorf("could not reach chaos endpoint: %w", err)
	}
	if resp != nil {
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("chaos endpoint returned %d", resp.StatusCode)
		}
	}
	e := &Experiment{ID: newID("exp"), Service: service, Type: typ, DurationS: durationS, Intensity: intensity,
		Status: "running", InjectedAt: time.Now().UTC()}
	c.St.mu.Lock()
	c.St.Experiments = append(c.St.Experiments, e)
	c.St.dirty = true
	c.St.mu.Unlock()
	c.St.Audit(actor, "chaos.started", service, "success", "", map[string]string{"type": typ})
	c.St.Publish("chaos", map[string]string{"id": e.ID, "status": "running"})
	time.AfterFunc(time.Duration(durationS)*time.Second, func() { c.finishExperiment(e.ID, "completed") })
	return e, nil
}

func (c *Core) finishExperiment(id, status string) {
	c.St.mu.Lock()
	var e *Experiment
	for _, x := range c.St.Experiments {
		if x.ID == id {
			e = x
		}
	}
	if e == nil || e.Status != "running" {
		c.St.mu.Unlock()
		return
	}
	e.Status = status
	svc, typ := e.Service, e.Type
	c.St.dirty = true
	c.St.mu.Unlock()
	if s := c.serviceByName(svc); s != nil && s.ControlURL != "" && typ != "crash" {
		req, _ := http.NewRequestWithContext(c.ctx, http.MethodPost, s.ControlURL+"/chaos/clear", nil)
		if resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req); err == nil {
			resp.Body.Close()
		}
	}
	c.St.Publish("chaos", map[string]string{"id": id, "status": status})
}

func (c *Core) StopExperiment(id, actor string) error {
	c.finishExperiment(id, "stopped")
	c.St.Audit(actor, "chaos.stopped", id, "success", "", nil)
	return nil
}
