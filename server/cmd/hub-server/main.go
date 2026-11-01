// Command hub-server serves the HomeLab dashboard: JSON API and embedded web UI.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hub/server/internal/api"
	"hub/server/internal/fixture"
	"hub/server/internal/webui"
)

func main() {
	// Loopback by default: exposure goes through Caddy (ARCHITECTURE.md), never directly.
	addr := flag.String("addr", envOr("HUB_ADDR", "127.0.0.1:8080"), "listen address")
	flag.Parse()

	ui := webui.New()
	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.NewHandler(fixture.New(), ui.Handler, ui.ScriptHashes),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("listening", "addr", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "err", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
