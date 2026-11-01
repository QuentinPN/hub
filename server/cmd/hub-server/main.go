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
	"hub/server/internal/db"
	"hub/server/internal/fixture"
	"hub/server/internal/pgstore"
	"hub/server/internal/webui"
)

func main() {
	// Loopback by default: exposure goes through Caddy (ARCHITECTURE.md), never directly.
	addr := flag.String("addr", envOr("HUB_ADDR", "127.0.0.1:8080"), "listen address")
	flag.Parse()
	// Read from the environment only, so the password never shows in the process list.
	databaseURL := os.Getenv("HUB_DATABASE_URL")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, closeStore, err := openStore(ctx, databaseURL)
	if err != nil {
		slog.Error("startup", "err", err)
		os.Exit(1)
	}
	defer closeStore()

	ui := webui.New()
	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.NewHandler(store, ui.Handler, ui.ScriptHashes),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

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

// openStore uses PostgreSQL when a database URL is set, and otherwise the read-only
// example data, which is enough to work on the UI.
func openStore(ctx context.Context, databaseURL string) (api.Store, func(), error) {
	live := fixture.New()
	if databaseURL == "" {
		slog.Warn("HUB_DATABASE_URL is not set: example data, settings are read-only")
		return live, func() {}, nil
	}
	pool, err := db.Open(ctx, databaseURL)
	if err != nil {
		return nil, nil, err
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, nil, err
	}
	store, err := pgstore.New(ctx, pool, live)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return store, pool.Close, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
