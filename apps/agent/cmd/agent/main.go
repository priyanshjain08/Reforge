package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/reforge/agent/internal/collector"
	"github.com/reforge/agent/internal/docker"
	"github.com/reforge/agent/internal/runner"
)

const version = "0.1.0"

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

type api struct {
	base, id, secret string
	hc               *http.Client
}

func (a *api) do(method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, a.base+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-ID", a.id)
	req.Header.Set("X-Agent-Secret", a.secret)
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return &httpErr{resp.StatusCode}
	}
	if out == nil {
		return nil
	}
	var env struct{ Data json.RawMessage }
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env); err != nil {
		return err
	}
	return json.Unmarshal(env.Data, out)
}

type httpErr struct{ code int }

func (e *httpErr) Error() string { return "core returned HTTP " + http.StatusText(e.code) }

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	a := &api{base: strings.TrimRight(env("REFORGE_CORE_URL", "http://localhost:8080"), "/"),
		id: os.Getenv("REFORGE_AGENT_ID"), secret: os.Getenv("REFORGE_AGENT_SECRET"), hc: &http.Client{Timeout: 10 * time.Second}}
	if a.id == "" || a.secret == "" {
		log.Error("REFORGE_AGENT_ID and REFORGE_AGENT_SECRET are required")
		os.Exit(1)
	}
	targets := map[string]bool{}
	var targetList []string
	for _, t := range strings.Split(os.Getenv("REFORGE_AGENT_TARGETS"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			targets[t] = true
			targetList = append(targetList, t)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dk := docker.New(env("REFORGE_DOCKER_SOCKET", "/var/run/docker.sock"))
	dockerOK := dk.Ping(ctx) == nil
	caps := []string{"host-metrics"}
	if dockerOK {
		caps = append(caps, "docker")
	} else {
		log.Warn("docker socket not reachable: container monitoring and recovery disabled")
	}
	host, _ := os.Hostname()
	col := collector.New()

	// command poller: only explicitly allowlisted commands run
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				var cmds []runner.Command
				if err := a.do("GET", "/api/v1/agent/commands", nil, &cmds); err != nil {
					continue
				}
				for _, cmd := range cmds {
					log.Info("command received", "id", cmd.ID, "action", cmd.Action, "target", cmd.Target)
					res := runner.Result{Error: "docker unavailable"}
					if dockerOK {
						res = runner.Execute(ctx, dk, cmd, targets)
					}
					if err := a.do("POST", "/api/v1/agent/commands/"+cmd.ID+"/result", res, nil); err != nil {
						log.Error("could not report result", "err", err)
					}
					log.Info("command finished", "id", cmd.ID, "ok", res.OK, "error", res.Error)
				}
			}
		}
	}()

	interval, _ := time.ParseDuration(env("REFORGE_AGENT_INTERVAL", "5s"))
	if interval < time.Second {
		interval = 5 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		s := col.Collect()
		containers := []map[string]any{}
		if dockerOK {
			for _, name := range targetList {
				cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				info, err := dk.Inspect(cctx, name)
				cancel()
				if err != nil {
					continue
				}
				containers = append(containers, map[string]any{"name": info.Name, "state": info.State, "status": info.Status,
					"health": info.Health, "restarts": info.Restarts, "exit_code": info.ExitCode})
			}
		}
		payload := map[string]any{"hostname": host, "os": runtime.GOOS + "/" + runtime.GOARCH, "version": version,
			"capabilities": caps, "metrics": s.Metrics, "unsupported": s.Unsupported, "containers": containers}
		if err := a.do("POST", "/api/v1/agent/telemetry", payload, nil); err != nil {
			log.Warn("telemetry failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
