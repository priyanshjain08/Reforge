package recovery

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fake struct{ execs, verifies, rollbacks int }

func newEngine(mode Mode, f *fake, verifyErr, execErr error, cooldown time.Duration, now func() time.Time) *Engine {
	e := NewEngine(Config{Mode: mode, MaxRetries: 2, Cooldown: cooldown, Now: now})
	_ = e.Register(ActionDef{
		ID: "restart_container", Permission: "containers.restart", Risk: RiskLow,
		Validate: func(context.Context, Target) error { return nil },
		Execute:  func(context.Context, Target) error { f.execs++; return execErr },
		Verify:   func(context.Context, Target) error { f.verifies++; return verifyErr },
		Rollback: func(context.Context, Target) error { f.rollbacks++; return nil },
	})
	return e
}

func req(approved bool) Request {
	return Request{
		IncidentID: "inc-1", ActionID: "restart_container",
		Target:      Target{Kind: "container", Name: "payment"},
		Permissions: map[string]bool{"containers.restart": true},
		Authorized:  map[string]bool{"container/payment": true},
		Approved:    approved,
	}
}

func TestSuccessRequiresVerification(t *testing.T) {
	f := &fake{}
	r := newEngine(ModeAuto, f, nil, nil, 0, nil).Run(context.Background(), req(false))
	if r.State != StateSuccessful || f.execs != 1 || f.verifies != 1 {
		t.Fatalf("got %v execs=%d verifies=%d", r.State, f.execs, f.verifies)
	}
}

func TestFailedVerificationIsNotSuccess(t *testing.T) {
	f := &fake{}
	r := newEngine(ModeAuto, f, errors.New("still down"), nil, 0, nil).Run(context.Background(), req(false))
	if r.State == StateSuccessful {
		t.Fatal("unverified recovery must not be reported successful")
	}
	if r.State != StateRolledBack || f.rollbacks != 1 {
		t.Fatalf("expected rollback, got %v", r.State)
	}
}

func TestKillSwitchBlocksExecution(t *testing.T) {
	f := &fake{}
	e := newEngine(ModeAuto, f, nil, nil, 0, nil)
	e.SetKillSwitch(true)
	r := e.Run(context.Background(), req(true))
	if r.State != StateCancelled || !errors.Is(r.Err, ErrKillSwitch) || f.execs != 0 {
		t.Fatalf("kill switch failed: %v execs=%d", r.State, f.execs)
	}
}

func TestUnauthorizedTargetAndPermission(t *testing.T) {
	f := &fake{}
	e := newEngine(ModeAuto, f, nil, nil, 0, nil)
	bad := req(true)
	bad.Authorized = map[string]bool{}
	if r := e.Run(context.Background(), bad); !errors.Is(r.Err, ErrUnauthorized) || f.execs != 0 {
		t.Fatalf("unauthorized target executed: %v", r.State)
	}
	bad = req(true)
	bad.Permissions = map[string]bool{}
	if r := e.Run(context.Background(), bad); !errors.Is(r.Err, ErrUnauthorized) || f.execs != 0 {
		t.Fatalf("missing permission executed: %v", r.State)
	}
}

func TestUnknownActionRejected(t *testing.T) {
	r := newEngine(ModeAuto, &fake{}, nil, nil, 0, nil)
	q := req(true)
	q.ActionID = "rm_rf"
	if res := r.Run(context.Background(), q); !errors.Is(res.Err, ErrUnknownAction) {
		t.Fatalf("expected ErrUnknownAction, got %v", res.Err)
	}
}

func TestSimulateAndApprovalModes(t *testing.T) {
	f := &fake{}
	if r := newEngine(ModeSimulate, f, nil, nil, 0, nil).Run(context.Background(), req(true)); f.execs != 0 || r.State != StateCancelled {
		t.Fatal("simulate mode must not execute")
	}
	e := newEngine(ModeApproval, f, nil, nil, 0, nil)
	if r := e.Run(context.Background(), req(false)); r.State != StateAwaitingApproval || f.execs != 0 {
		t.Fatalf("expected awaiting approval, got %v", r.State)
	}
	if r := e.Run(context.Background(), req(true)); r.State != StateSuccessful {
		t.Fatalf("approved run should succeed, got %v", r.State)
	}
}

func TestCooldownAndRetryLimit(t *testing.T) {
	f := &fake{}
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newEngine(ModeAuto, f, nil, nil, time.Minute, func() time.Time { return clock })
	if r := e.Run(context.Background(), req(true)); r.State != StateSuccessful {
		t.Fatalf("first run: %v", r.State)
	}
	if r := e.Run(context.Background(), req(true)); !errors.Is(r.Err, ErrCooldown) {
		t.Fatalf("expected cooldown, got %v", r.Err)
	}
	clock = clock.Add(2 * time.Minute)
	if r := e.Run(context.Background(), req(true)); r.State != StateSuccessful {
		t.Fatalf("after cooldown: %v", r.State)
	}
	clock = clock.Add(2 * time.Minute)
	if r := e.Run(context.Background(), req(true)); r.State != StateEscalated || !errors.Is(r.Err, ErrRetryLimit) {
		t.Fatalf("expected escalation on retry limit, got %v %v", r.State, r.Err)
	}
}
