// Package runner validates and executes recovery commands received from Reforge Core.
// The agent re-checks every command locally: the core is not blindly trusted.
package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/reforge/agent/internal/docker"
)

type Command struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Target string `json:"target"`
}

type Result struct {
	OK             bool   `json:"ok"`
	Error          string `json:"error"`
	ContainerState string `json:"container_state"`
}

// Allowed is the agent-side allowlist: only restart_container, only for locally configured targets.
func Allowed(cmd Command, targets map[string]bool) error {
	if cmd.Action != "restart_container" {
		return fmt.Errorf("action %q is not permitted on this agent", cmd.Action)
	}
	if !targets[cmd.Target] {
		return fmt.Errorf("target %q is not in this agent's allowlist", cmd.Target)
	}
	return nil
}

type Docker interface {
	Restart(ctx context.Context, name string) error
	Inspect(ctx context.Context, name string) (docker.Info, error)
}

func Execute(ctx context.Context, d Docker, cmd Command, targets map[string]bool) Result {
	if err := Allowed(cmd, targets); err != nil {
		return Result{Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	if err := d.Restart(ctx, cmd.Target); err != nil {
		return Result{Error: err.Error()}
	}
	info, err := d.Inspect(ctx, cmd.Target)
	if err != nil {
		return Result{OK: true, Error: "restarted but inspect failed: " + err.Error()}
	}
	return Result{OK: true, ContainerState: info.State}
}
