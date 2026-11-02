// Command hub-agent reads a host's containers through the filtering Docker proxy and
// pushes them to the Hub server over mutual TLS (AGENT-STUDY.md, option 1.b). It opens
// no port and runs no Docker command.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"hub/server/internal/agent"
)

// version is set at build time (-ldflags "-X main.version=…").
var version = "dev"

func main() {
	serverURL := strings.TrimRight(os.Getenv("HUB_SERVER_URL"), "/")
	stateDir := envOr("HUB_STATE_DIR", "/var/lib/hub-agent")
	socket := envOr("HUB_DOCKER_SOCKET", "/run/hub-proxy/docker.sock")
	if !strings.HasPrefix(serverURL, "https://") {
		fatal(errors.New("HUB_SERVER_URL must be an https:// URL, e.g. https://192.0.2.14:8443"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !agent.HasIdentity(stateDir) {
		if err := enroll(ctx, serverURL, stateDir); err != nil {
			fatal(err)
		}
		slog.Info("enrolled", "server", serverURL)
	}

	a := &agent.Agent{ServerURL: serverURL, StateDir: stateDir, Version: version, Docker: agent.NewDocker(socket)}
	slog.Info("hub-agent starting", "version", version, "docker_socket", socket)
	if err := a.Run(ctx); err != nil {
		fatal(err)
	}
}

// enroll reads the one-time code from a file (a Compose secret), never from an environment
// variable that docker inspect would show.
func enroll(ctx context.Context, serverURL, stateDir string) error {
	codeFile, pin := os.Getenv("HUB_ENROLL_CODE_FILE"), os.Getenv("HUB_CA_PIN")
	if codeFile == "" || !strings.HasPrefix(pin, "sha256:") {
		return errors.New("not enrolled yet: set HUB_ENROLL_CODE_FILE and HUB_CA_PIN (from hub-server -enroll-host)")
	}
	code, err := os.ReadFile(codeFile)
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	return agent.Enroll(ctx, serverURL, pin, strings.TrimSpace(string(code)), hostname, stateDir)
}

func fatal(err error) {
	slog.Error("hub-agent", "err", err)
	os.Exit(1)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
