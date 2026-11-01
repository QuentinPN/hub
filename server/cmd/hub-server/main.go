// Command hub-server serves the HomeLab dashboard: JSON API and embedded web UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hub/server/internal/api"
	"hub/server/internal/auth"
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

	authSvc, err := auth.New(store)
	if err != nil {
		slog.Error("startup", "err", err)
		os.Exit(1)
	}
	if err := enablePasskeys(authSvc, os.Getenv("HUB_ORIGIN")); err != nil {
		slog.Error("startup", "err", err)
		os.Exit(1)
	}
	// The setup code goes to the logs only: reading them already requires access to the
	// server. A new code is issued at each start until the account exists.
	if code, err := authSvc.PrepareSetup(ctx); err != nil {
		slog.Error("startup", "err", err)
		os.Exit(1)
	} else if code != "" {
		slog.Warn("no account yet: open the dashboard and create it with this one-time code", "setup_code", code)
	}

	ui := webui.New()
	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.NewHandler(store, authSvc, ui.Handler, ui.ScriptHashes),
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

// openStore connects, migrates and returns the PostgreSQL store. The database is required:
// the account and sessions live there, and without them nothing is reachable.
func openStore(ctx context.Context, databaseURL string) (*pgstore.Store, func(), error) {
	if databaseURL == "" {
		return nil, nil, errors.New("HUB_DATABASE_URL is required")
	}
	pool, err := db.Open(ctx, databaseURL)
	if err != nil {
		return nil, nil, err
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, nil, err
	}
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return store, pool.Close, nil
}

// enablePasskeys binds passkeys to the dashboard's public origin (HUB_ORIGIN, e.g.
// https://hub.example). Without it only localhost works, for development: a passkey
// needs a host name, never an IP address, and cannot move to another name later.
func enablePasskeys(svc *auth.Service, origin string) error {
	if origin == "" {
		slog.Warn("HUB_ORIGIN not set: passkeys only work on http://localhost")
		return svc.EnablePasskeys("localhost", []string{"http://localhost:5173", "http://localhost:8080"})
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.Path != "" && u.Path != "/" || net.ParseIP(u.Hostname()) != nil {
		return fmt.Errorf("HUB_ORIGIN must be an https:// URL with a host name, got %q", origin)
	}
	return svc.EnablePasskeys(u.Hostname(), []string{u.Scheme + "://" + u.Host})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
