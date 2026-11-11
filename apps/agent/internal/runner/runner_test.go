package runner

import (
	"context"
	"errors"
	"testing"

	"github.com/reforge/agent/internal/docker"
)

type fakeDocker struct {
	restarted []string
	err       error
}

func (f *fakeDocker) Restart(_ context.Context, n string) error {
	f.restarted = append(f.restarted, n)
	return f.err
}
func (f *fakeDocker) Inspect(_ context.Context, n string) (docker.Info, error) {
	return docker.Info{Name: n, State: "running"}, nil
}

func TestRejectsUnknownActionAndTarget(t *testing.T) {
	f := &fakeDocker{}
	targets := map[string]bool{"payment-service": true}
	if r := Execute(context.Background(), f, Command{Action: "exec", Target: "payment-service"}, targets); r.OK || len(f.restarted) != 0 {
		t.Fatal("unknown action must be rejected")
	}
	if r := Execute(context.Background(), f, Command{Action: "restart_container", Target: "database"}, targets); r.OK || len(f.restarted) != 0 {
		t.Fatal("non-allowlisted target must be rejected")
	}
}

func TestRestartAllowedTarget(t *testing.T) {
	f := &fakeDocker{}
	r := Execute(context.Background(), f, Command{Action: "restart_container", Target: "payment-service"}, map[string]bool{"payment-service": true})
	if !r.OK || r.ContainerState != "running" || len(f.restarted) != 1 {
		t.Fatalf("unexpected result %+v", r)
	}
}

func TestRestartFailureReported(t *testing.T) {
	f := &fakeDocker{err: errors.New("boom")}
	r := Execute(context.Background(), f, Command{Action: "restart_container", Target: "a"}, map[string]bool{"a": true})
	if r.OK || r.Error == "" {
		t.Fatal("failure must be reported")
	}
}
