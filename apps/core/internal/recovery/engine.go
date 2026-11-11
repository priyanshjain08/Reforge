// Package recovery implements the allowlisted, policy-gated remediation engine.
// It never executes arbitrary commands: only actions registered in the Registry can run,
// and only against explicitly authorized targets.
package recovery

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type State string

const (
	StatePending          State = "pending"
	StateAwaitingApproval State = "awaiting_approval"
	StateValidating       State = "validating"
	StateExecuting        State = "executing"
	StateVerifying        State = "verifying"
	StateSuccessful       State = "successful"
	StateFailed           State = "failed"
	StateRolledBack       State = "rolled_back"
	StateEscalated        State = "escalated"
	StateCancelled        State = "cancelled"
)

type Mode string

const (
	ModeSimulate Mode = "simulate" // default: evaluate only, execute nothing
	ModeApproval Mode = "approval" // execute only after explicit approval
	ModeAuto     Mode = "auto"     // execute automatically if policy + permissions allow
)

type Risk int

const (
	RiskLow Risk = iota
	RiskMedium
	RiskHigh
)

var (
	ErrKillSwitch     = errors.New("recovery kill switch is enabled")
	ErrUnknownAction  = errors.New("action is not in the allowlist")
	ErrUnauthorized   = errors.New("target or permission not authorized")
	ErrCooldown       = errors.New("action is in cooldown")
	ErrRetryLimit     = errors.New("retry limit exceeded")
	ErrNeedsApproval  = errors.New("approval required")
)

// Target is an explicitly registered, authorized object (container, managed process, ...).
type Target struct {
	Kind string // "container" | "process" | "application"
	Name string
}

func (t Target) key() string { return t.Kind + "/" + t.Name }

// ActionDef is an allowlisted action. Validate/Execute/Verify are required; Rollback is optional.
type ActionDef struct {
	ID         string
	Permission string
	Risk       Risk
	Validate   func(ctx context.Context, t Target) error
	Execute    func(ctx context.Context, t Target) error
	Verify     func(ctx context.Context, t Target) error
	Rollback   func(ctx context.Context, t Target) error
}

type Request struct {
	IncidentID  string
	ActionID    string
	Target      Target
	Permissions map[string]bool // permissions granted to the executing agent
	Authorized  map[string]bool // allowed target keys ("container/payment")
	Approved    bool
}

type Result struct {
	State    State
	Steps    []string
	Err      error
	Started  time.Time
	Finished time.Time
}

type AuditFunc func(event string, req Request, detail string)

type Engine struct {
	mode       Mode
	registry   map[string]ActionDef
	kill       atomic.Bool
	maxRetries int
	cooldown   time.Duration
	execTO     time.Duration
	now        func() time.Time
	audit      AuditFunc

	mu       sync.Mutex
	attempts map[string][]time.Time
}

type Config struct {
	Mode       Mode
	MaxRetries int
	Cooldown   time.Duration
	ExecTimeout time.Duration
	Now        func() time.Time
	Audit      AuditFunc
}

func NewEngine(cfg Config) *Engine {
	if cfg.Mode == "" {
		cfg.Mode = ModeSimulate
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.ExecTimeout <= 0 {
		cfg.ExecTimeout = 30 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Audit == nil {
		cfg.Audit = func(string, Request, string) {}
	}
	return &Engine{mode: cfg.Mode, registry: map[string]ActionDef{}, maxRetries: cfg.MaxRetries,
		cooldown: cfg.Cooldown, execTO: cfg.ExecTimeout, now: cfg.Now, audit: cfg.Audit,
		attempts: map[string][]time.Time{}}
}

func (e *Engine) Register(a ActionDef) error {
	if a.ID == "" || a.Validate == nil || a.Execute == nil || a.Verify == nil {
		return fmt.Errorf("action %q must define id, Validate, Execute and Verify", a.ID)
	}
	e.registry[a.ID] = a
	return nil
}

func (e *Engine) SetKillSwitch(on bool) { e.kill.Store(on) }
func (e *Engine) KillSwitch() bool      { return e.kill.Load() }

// Run drives one recovery through the lifecycle. Success is reported only if Verify passes.
func (e *Engine) Run(ctx context.Context, req Request) Result {
	res := Result{State: StatePending, Started: e.now()}
	finish := func(s State, err error, step string) Result {
		res.State, res.Err, res.Finished = s, err, e.now()
		res.Steps = append(res.Steps, step)
		e.audit("recovery."+string(s), req, errString(err))
		return res
	}

	if e.kill.Load() {
		return finish(StateCancelled, ErrKillSwitch, "kill switch enabled")
	}
	act, ok := e.registry[req.ActionID]
	if !ok {
		return finish(StateFailed, ErrUnknownAction, "action rejected")
	}
	res.State = StateValidating
	if !req.Permissions[act.Permission] || !req.Authorized[req.Target.key()] {
		return finish(StateFailed, ErrUnauthorized, "authorization failed")
	}
	if err := act.Validate(ctx, req.Target); err != nil {
		return finish(StateFailed, fmt.Errorf("validation: %w", err), "validation failed")
	}
	if err := e.checkLoopGuards(req); err != nil {
		st := StateFailed
		if errors.Is(err, ErrRetryLimit) {
			st = StateEscalated
		}
		return finish(st, err, "loop guard")
	}
	if e.mode == ModeSimulate {
		return finish(StateCancelled, nil, "simulation mode: no action executed")
	}
	if (e.mode == ModeApproval || act.Risk >= RiskHigh) && !req.Approved {
		res.State = StateAwaitingApproval
		res.Steps = append(res.Steps, "awaiting approval")
		e.audit("recovery.awaiting_approval", req, "")
		res.Err = ErrNeedsApproval
		return res
	}

	e.recordAttempt(req)
	res.State = StateExecuting
	xctx, cancel := context.WithTimeout(ctx, e.execTO)
	defer cancel()
	if err := act.Execute(xctx, req.Target); err != nil {
		return e.failOrRollback(ctx, act, req, &res, fmt.Errorf("execute: %w", err))
	}
	res.State = StateVerifying
	res.Steps = append(res.Steps, "executed")
	if err := act.Verify(xctx, req.Target); err != nil {
		return e.failOrRollback(ctx, act, req, &res, fmt.Errorf("verification: %w", err))
	}
	return finish(StateSuccessful, nil, "verified healthy")
}

func (e *Engine) failOrRollback(ctx context.Context, a ActionDef, req Request, res *Result, cause error) Result {
	res.Err, res.Finished = cause, e.now()
	if a.Rollback != nil {
		if err := a.Rollback(ctx, req.Target); err == nil {
			res.State = StateRolledBack
			res.Steps = append(res.Steps, "rolled back")
			e.audit("recovery.rolled_back", req, cause.Error())
			return *res
		}
	}
	res.State = StateFailed
	res.Steps = append(res.Steps, "failed")
	e.audit("recovery.failed", req, cause.Error())
	return *res
}

func (e *Engine) checkLoopGuards(req Request) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	k := req.ActionID + "|" + req.Target.key()
	now := e.now()
	hist := e.attempts[k]
	if len(hist) >= e.maxRetries {
		return ErrRetryLimit
	}
	if len(hist) > 0 && e.cooldown > 0 && now.Sub(hist[len(hist)-1]) < e.cooldown {
		return ErrCooldown
	}
	return nil
}

func (e *Engine) recordAttempt(req Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	k := req.ActionID + "|" + req.Target.key()
	e.attempts[k] = append(e.attempts[k], e.now())
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
