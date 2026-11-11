package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reforge/core/internal/core"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := core.ConfigFromEnv()
	if len(cfg.AdminToken) < 16 {
		log.Error("REFORGE_ADMIN_TOKEN must be set (16+ characters). Run scripts/setup.ps1 or copy .env.example to .env")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c := core.New(ctx, cfg)
	go c.Run()
	srv := &http.Server{Addr: cfg.Addr, Handler: core.NewServer(c).Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sh)
	}()
	log.Info("reforge core listening", "addr", cfg.Addr, "recovery_mode", cfg.RecoveryMode)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server error", "err", err)
		os.Exit(1)
	}
}
