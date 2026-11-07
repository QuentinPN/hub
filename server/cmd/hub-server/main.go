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
	"strings"
	"syscall"
	"time"
	// The image has no zone files: TZ (Europe/Paris) and the zone of « Général » need them
	// for the 03:00 backups.
	_ "time/tzdata"

	"hub/server/internal/agentsrv"
	"hub/server/internal/api"
	"hub/server/internal/auth"
	"hub/server/internal/backup"
	"hub/server/internal/db"
	"hub/server/internal/fixture"
	"hub/server/internal/icons"
	"hub/server/internal/notify"
	"hub/server/internal/pgstore"
	"hub/server/internal/pki"
	"hub/server/internal/secrets"
	"hub/server/internal/urlcheck"
	"hub/server/internal/webui"
	"hub/server/internal/widgets"
)

// version is set at build time (scripts/build.sh).
var version = "dev"

func main() {
	setupLogs(os.Getenv("HUB_LOG_FORMAT"), os.Getenv("HUB_LOG_LEVEL"))
	started := time.Now()
	// Loopback by default: exposure goes through Caddy (ARCHITECTURE.md), never directly.
	addr := flag.String("addr", envOr("HUB_ADDR", "127.0.0.1:8080"), "listen address")
	newKey := flag.Bool("new-secret-key", false, "print a new line to append to the secret key file, then exit")
	reset := flag.Bool("reset-password", false, "set a new account password, end every session, then exit")
	enrollHost := flag.String("enroll-host", "", "print a one-time code to enroll the agent of this host (created if new), then exit")
	revokeHost := flag.String("revoke-host", "", "revoke every certificate of this host's agent, then exit")
	health := flag.Bool("healthcheck", false, "exit 0 if the server listening on this machine answers, for the image's health check")
	restore := flag.String("restore", "", "restore this backup file into an empty database and fresh key files, then exit")
	verify := flag.String("verify-backup", "", "read this backup file to the end with its passphrase, writing nothing, then exit")
	flag.Parse()
	if *verify != "" {
		if err := verifyBackup(*verify); err != nil {
			slog.Error("verify backup", "err", err)
			os.Exit(1)
		}
		return
	}
	if *health {
		if err := checkHealth(*addr); err != nil {
			slog.Error("health check", "err", err)
			os.Exit(1)
		}
		return
	}
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

	keyFiles := map[string]string{
		backup.KeySecrets: os.Getenv("HUB_SECRET_KEY_FILE"),
		backup.KeyAgentCA: os.Getenv("HUB_AGENT_CA_FILE"),
	}
	// Before openStore, which would migrate the empty database and fill it.
	if *restore != "" {
		if err := restoreBackup(ctx, *restore, databaseURL, keyFiles); err != nil {
			slog.Error("restore", "err", err)
			os.Exit(1)
		}
		return
	}

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

	// The agents' CA is optional until a host is enrolled: without HUB_AGENT_CA_FILE the
	// agent listener stays off. Before the secrets, like the password reset: enrolling or
	// revoking an agent does not need the secret key file.
	agents, err := openAgents(os.Getenv("HUB_AGENT_CA_FILE"), store)
	if err != nil {
		slog.Error("startup", "err", err)
		os.Exit(1)
	}
	if *enrollHost != "" || *revokeHost != "" {
		if agents == nil {
			slog.Error("HUB_AGENT_CA_FILE is required to enroll or revoke an agent")
			closeStore()
			os.Exit(1)
		}
		if *enrollHost != "" {
			err = printEnrollment(ctx, agents, *enrollHost)
		} else {
			err = revokeAgent(ctx, agents, *revokeHost)
		}
		if err != nil {
			slog.Error("agent", "err", err)
			closeStore()
			os.Exit(1)
		}
		return
	}

	// Opening them checks the key file at each start and finishes any rotation.
	secretStore, err := openSecrets(ctx, os.Getenv("HUB_SECRET_KEY_FILE"), store)
	if err != nil {
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

	// The agents dial the first name their server certificate carries, on the agent port.
	agentAddr := envOr("HUB_AGENT_ADDR", "127.0.0.1:8443")
	agentNames := strings.Split(envOr("HUB_AGENT_NAMES", "localhost,127.0.0.1"), ",")
	var agentsUI *api.Agents
	// What « Général » shows of the .env: only what this process was really given.
	about := api.About{Origin: os.Getenv("HUB_ORIGIN"), Host: os.Getenv("HUB_HOST_NAME"), ServerTimeZone: os.Getenv("TZ")}
	if agents != nil {
		_, port, err := net.SplitHostPort(agentAddr)
		if err != nil {
			slog.Error("startup", "err", fmt.Errorf("HUB_AGENT_ADDR: %w", err))
			os.Exit(1)
		}
		agentsUI = &api.Agents{Admin: agents, ServerURL: "https://" + net.JoinHostPort(agentNames[0], port)}
		about.AgentURL = agentsUI.ServerURL
	}

	// Each address is requested once a minute, from here, to tell whether it still answers.
	go urlcheck.Watch(ctx, store, time.Minute)
	// Each connection is read at its own rate; this only looks for those due.
	bus := widgets.NewBus()
	go widgets.Run(ctx, store, secretStore, bus, 5*time.Second)
	// Backups go to a folder mounted from elsewhere (HUB_BACKUP_DIR), never created here.
	// HUB_BACKUP=off turns them off: nothing runs, the section says so.
	var backups api.Backups = backup.Off{}
	switch os.Getenv("HUB_BACKUP") {
	case "off":
		slog.Info("backups turned off (HUB_BACKUP=off)")
	case "", "on":
		svc := backup.NewService(ctx, store, store.Pool(), os.Getenv("HUB_BACKUP_DIR"), keyFiles, version)
		go svc.Watch(ctx, time.Minute)
		backups = svc
	default:
		slog.Error("startup", "err", errors.New("HUB_BACKUP must be on or off"))
		os.Exit(1)
	}

	// Alerts go out through ntfy and e-mail; the engine looks once a minute.
	var backupSrc notify.Backups
	if _, on := backups.(*backup.Service); on {
		backupSrc = store
	}
	notifier := notify.NewService(store, secretStore, backupSrc)
	go notifier.Watch(ctx, time.Minute)

	ui := webui.New()
	srv := &http.Server{
		Addr: *addr,
		Handler: api.NewHandler(store, authSvc, iconSvc, agentsUI, &api.Widgets{Keys: secretStore, Bus: bus},
			&api.Runtime{Version: version, StartedAt: started, Secrets: secretStore, Streams: widgets.OpenStreams, Backups: backups, About: about, Notifications: notifier}, ui.Handler, ui.ScriptHashes),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Open event streams must not hold the shutdown up.
	srv.RegisterOnShutdown(bus.Close)

	go func() {
		slog.Info("listening", "addr", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "err", err)
			os.Exit(1)
		}
	}()

	var agentSrv *http.Server
	if agents != nil {
		tlsCfg, err := agents.TLSConfig(agentNames)
		if err != nil {
			slog.Error("startup", "err", err)
			os.Exit(1)
		}
		agentSrv = &http.Server{
			Addr:              agentAddr,
			Handler:           agents.Handler(),
			TLSConfig:         tlsCfg,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		go func() {
			slog.Info("agent listener", "addr", agentAddr, "names", agentNames)
			if err := agentSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("agent listener stopped", "err", err)
				os.Exit(1)
			}
		}()
	}

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "err", err)
	}
	if agentSrv != nil {
		if err := agentSrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown", "err", err)
		}
	}
}

// setupLogs picks text (the default, for a person reading) or JSON (for a collector), and a
// level: debug, info (default), warn or error.
func setupLogs(format, level string) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil || level == "" {
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler = slog.NewTextHandler(os.Stderr, opts)
	if format == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(h))
}

// checkHealth asks /readyz on loopback, at the port of the listen address: the image has
// no shell or curl for its health check to use.
func checkHealth(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 3 * time.Second}
	// Ready, not only alive: the database answers and its migrations are applied.
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/readyz")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
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
	// No invented service in production: the catalogue stays empty until agents report.
	live := fixture.Empty()
	if os.Getenv("HUB_DEMO_DATA") == "on" {
		live = fixture.New()
	}
	store, err := pgstore.New(ctx, pool, live)
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

// openAgents loads the agents' CA, creating it on the first start. nil without a path.
func openAgents(path string, store *pgstore.Store) (*agentsrv.Service, error) {
	if path == "" {
		slog.Info("HUB_AGENT_CA_FILE not set: agent listener off")
		return nil, nil
	}
	ca, err := pki.LoadFile(path)
	if pki.IsNotExist(err) {
		if ca, err = pki.CreateFile(path, time.Now()); err == nil {
			slog.Warn("created the agents' CA: back it up apart from the database, without it every agent must be enrolled again", "path", path)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("agent CA file: %w", err)
	}
	return agentsrv.New(ca, store), nil
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
