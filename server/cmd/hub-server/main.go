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
	"hub/server/internal/icons"
	"hub/server/internal/pgstore"
	"hub/server/internal/secrets"
	"hub/server/internal/webui"
)

func main() {
	// Loopback by default: exposure goes through Caddy (ARCHITECTURE.md), never directly.
	addr := flag.String("addr", envOr("HUB_ADDR", "127.0.0.1:8080"), "listen address")
	newKey := flag.Bool("new-secret-key", false, "print a new line to append to the secret key file, then exit")
	reset := flag.Bool("reset-password", false, "set a new account password, end every session, then exit")
	flag.Parse()
	if *newKey {
		line, err := secrets.NewKeyLine(time.Now())
		if err != nil {
			slog.Error("new secret key", "err", err)
			os.Exit(1)
		}
		fmt.Println(line)
		return
	}
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

	// Before the secrets: a password reset must not create or rotate the key file.
	if *reset {
		svc, err := auth.New(store)
		if err == nil {
			err = resetPassword(ctx, svc)
		}
		if err != nil {
			slog.Error("reset password", "err", err)
			closeStore()
			os.Exit(1)
		}
		return
	}

	// No connector reads secrets yet; opening them now checks the key file at each start
	// and finishes any rotation.
	if _, err := openSecrets(ctx, os.Getenv("HUB_SECRET_KEY_FILE"), store); err != nil {
		slog.Error("startup", "err", err)
		os.Exit(1)
	}

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

	// Icons missing from the binary are downloaded once from selfh.st, then served from the
	// database. HUB_ICONS_REMOTE=off keeps the server off the network: bundled icons only.
	var iconFetcher icons.Fetcher
	if os.Getenv("HUB_ICONS_REMOTE") == "off" {
		slog.Info("icon downloads disabled (HUB_ICONS_REMOTE=off): bundled icons only")
	} else {
		iconFetcher = icons.NewHTTPFetcher(icons.RemoteBase)
	}
	iconSvc := icons.NewService(store, iconFetcher)

	ui := webui.New()
	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.NewHandler(store, authSvc, iconSvc, ui.Handler, ui.ScriptHashes),
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

// openSecrets loads the key file, creating it on the first start, then re-seals with the
// newest key every secret sealed with an older one.
func openSecrets(ctx context.Context, path string, repo secrets.Repo) (*secrets.Store, error) {
	if path == "" {
		return nil, errors.New("HUB_SECRET_KEY_FILE is required (keep it outside the database volume)")
	}
	kr, err := secrets.LoadKeyFile(path)
	if secrets.IsNotExist(err) {
		if kr, err = secrets.CreateKeyFile(path); err == nil {
			slog.Warn("created the secret key file: back it up apart from the database, without it the stored secrets are lost", "path", path)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("secret key file: %w", err)
	}
	store := secrets.NewStore(kr, repo)
	n, err := store.Rotate(ctx)
	if n > 0 {
		slog.Info("secrets re-sealed with the current key", "count", n, "key_id", kr.CurrentID())
	}
	if err != nil {
		// Only these secrets stay unreadable; the dashboard itself keeps working.
		slog.Error("some secrets cannot be opened with the key file", "err", err)
	}
	return store, nil
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
