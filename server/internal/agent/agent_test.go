package agent_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"hub/server/internal/agent"
	"hub/server/internal/agentsrv"
	"hub/server/internal/db/dbtest"
	"hub/server/internal/fixture"
	"hub/server/internal/pgstore"
	"hub/server/internal/pki"
)

const cid = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// fakeDocker answers like the Docker API on a unix socket and records every path asked.
type fakeDocker struct {
	mu    sync.Mutex
	paths []string
}

func (f *fakeDocker) start(t *testing.T) string {
	t.Helper()
	// Unix socket paths are short (108 bytes): not under t.TempDir() on every OS.
	dir, err := os.MkdirTemp("", "hub")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	srv := &httptest.Server{Listener: ln, Config: &http.Server{Handler: http.HandlerFunc(f.serve)}}
	srv.Start()
	t.Cleanup(srv.Close)
	return sock
}

func (f *fakeDocker) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.paths = append(f.paths, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	switch r.URL.Path {
	case "/version":
		fmt.Fprint(w, `{"Version":"29.0.0","ApiVersion":"1.52"}`)
	case "/containers/json":
		// What Docker really sends, secrets included: the agent must drop them.
		fmt.Fprintf(w, `[{"Id":%q,"Names":["/emby"],"Image":"emby/embyserver","Command":"/init --password=hunter2",
			"State":"running","Status":"Up 2 hours","Created":1793600000,
			"Labels":{"com.docker.compose.project":"media","traefik.http.middlewares.a.basicauth.users":"admin:$apr1$x"},
			"Ports":[{"PrivatePort":8096,"PublicPort":8096,"Type":"tcp"}]}]`, cid)
	case "/events":
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	default:
		http.Error(w, "unexpected", http.StatusTeapot)
	}
}

func TestDockerClientRefusesOtherCalls(t *testing.T) {
	f := &fakeDocker{}
	d := agent.NewDocker(f.start(t))
	list, err := d.Containers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c := list[0]
	if c.Name != "emby" || c.Labels["com.docker.compose.project"] != "media" {
		t.Fatalf("%+v", c)
	}
	if _, ok := c.Labels["traefik.http.middlewares.a.basicauth.users"]; ok || len(c.RedactedLabels) != 1 {
		t.Fatalf("label not filtered: %+v", c)
	}
	if strings.Contains(fmt.Sprintf("%+v", c), "hunter2") {
		t.Fatal("the command line was kept")
	}
	for _, p := range f.paths {
		if !strings.HasPrefix(p, "GET ") {
			t.Fatalf("non-GET call %s", p)
		}
	}
}

func TestEnrollRefusesWrongPin(t *testing.T) {
	ca, _ := pki.NewCA(time.Now())
	other, _ := pki.NewCA(time.Now())
	svc := agentsrv.New(ca, nil)
	srv := httptest.NewUnstartedServer(svc.Handler())
	srv.TLS, _ = svc.TLSConfig([]string{"127.0.0.1"})
	srv.StartTLS()
	defer srv.Close()
	err := agent.Enroll(context.Background(), srv.URL, other.Pin(), "code", "h", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "HUB_CA_PIN") {
		t.Fatalf("enrolled against an unpinned CA: %v", err)
	}
}

func TestAgentEndToEnd(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := pki.NewCA(time.Now())
	svc := agentsrv.New(ca, store)
	srv := httptest.NewUnstartedServer(svc.Handler())
	srv.TLS, _ = svc.TLSConfig([]string{"127.0.0.1"})
	srv.StartTLS()
	defer srv.Close()

	enr, err := svc.CreateEnrollment(ctx, "rpi5", "", agentsrv.CommandLine)
	if err != nil {
		t.Fatal(err)
	}
	code := enr.Code
	dir := t.TempDir()
	if err := agent.Enroll(ctx, srv.URL, ca.Pin(), code, "rpi5", dir); err != nil {
		t.Fatal(err)
	}
	if !agent.HasIdentity(dir) {
		t.Fatal("no identity saved")
	}

	f := &fakeDocker{}
	a := &agent.Agent{ServerURL: srv.URL, StateDir: dir, Version: "test", Docker: agent.NewDocker(f.start(t))}
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	deadline := time.Now().Add(10 * time.Second)
	var labels string
	for time.Now().Before(deadline) {
		err = pool.QueryRow(ctx, "SELECT labels::text FROM containers WHERE host_id = 'rpi5' AND id = $1", cid).Scan(&labels)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("snapshot never arrived: %v", err)
	}
	var dump string
	_ = pool.QueryRow(ctx, "SELECT string_agg(c::text, ' ') FROM containers c").Scan(&dump)
	if strings.Contains(dump, "hunter2") || strings.Contains(dump, "apr1") {
		t.Fatalf("secret in the database: %s", dump)
	}
	var agentVersion string
	_ = pool.QueryRow(ctx, "SELECT agent_version FROM hosts WHERE id = 'rpi5'").Scan(&agentVersion)
	if agentVersion != "test" {
		t.Fatalf("agent version %q", agentVersion)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not stop")
	}
}
