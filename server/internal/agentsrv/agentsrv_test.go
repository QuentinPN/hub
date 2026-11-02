package agentsrv_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"hub/server/internal/agentproto"
	"hub/server/internal/agentsrv"
	"hub/server/internal/db/dbtest"
	"hub/server/internal/fixture"
	"hub/server/internal/pgstore"
	"hub/server/internal/pki"
)

type env struct {
	t    *testing.T
	pool *pgxpool.Pool
	ca   *pki.CA
	svc  *agentsrv.Service
	srv  *httptest.Server
}

func setup(t *testing.T) *env {
	t.Helper()
	pool := dbtest.Pool(t)
	ctx := context.Background()
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	ca, err := pki.NewCA(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc := agentsrv.New(ca, store)
	srv := httptest.NewUnstartedServer(svc.Handler())
	if srv.TLS, err = svc.TLSConfig([]string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return &env{t: t, pool: pool, ca: ca, svc: svc, srv: srv}
}

// client trusts the CA and presents cert when it is not nil.
func (e *env) client(cert *tls.Certificate) *http.Client {
	cfg := &tls.Config{RootCAs: e.ca.Pool(), MinVersion: tls.VersionTLS13}
	if cert != nil {
		cfg.Certificates = []tls.Certificate{*cert}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
}

func (e *env) do(c *http.Client, method, path string, body any) (int, []byte) {
	e.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, e.srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	_, _ = out.ReadFrom(resp.Body)
	return resp.StatusCode, out.Bytes()
}

func newCSR(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		t.Fatal(err)
	}
	return key, der
}

// enroll runs the whole enrollment for hostID and returns the agent's TLS certificate.
func (e *env) enroll(hostID string) *tls.Certificate {
	e.t.Helper()
	code, err := e.svc.CreateEnrollment(context.Background(), hostID)
	if err != nil {
		e.t.Fatal(err)
	}
	key, csr := newCSR(e.t)
	status, body := e.do(e.client(nil), "POST", agentproto.PathEnroll, agentproto.EnrollRequest{Code: code, CSR: csr})
	if status != http.StatusOK {
		e.t.Fatalf("enroll: %d %s", status, body)
	}
	var resp agentproto.CertResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		e.t.Fatal(err)
	}
	return &tls.Certificate{Certificate: [][]byte{resp.Cert}, PrivateKey: key}
}

const cid = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestEnrollThenPushSnapshot(t *testing.T) {
	e := setup(t)
	cert := e.enroll("rpi5")
	agent := e.client(cert)

	snap := agentproto.Snapshot{AgentVersion: "dev", DockerVersion: "29.0.0", Containers: []agentproto.Container{{
		ID: cid, Name: "emby", Image: "emby/embyserver", State: "running", Status: "Up 2 hours",
		// The server must filter again even if the agent did not.
		Labels: map[string]string{
			"com.docker.compose.project":                    "media",
			"com.docker.compose.service":                    "emby",
			"traefik.http.middlewares.auth.basicauth.users": "admin:$apr1$secret-hash",
		},
		Ports: []agentproto.Port{{PrivatePort: 8096, PublicPort: 8096, Type: "tcp"}},
	}}}
	if status, body := e.do(agent, "PUT", agentproto.PathSnapshot, snap); status != http.StatusNoContent {
		t.Fatalf("snapshot: %d %s", status, body)
	}

	var project, labels string
	var redacted []string
	err := e.pool.QueryRow(context.Background(), `SELECT compose_project, labels::text, redacted_labels
		FROM containers WHERE host_id = 'rpi5' AND id = $1`, cid).Scan(&project, &labels, &redacted)
	if err != nil {
		t.Fatal(err)
	}
	if project != "media" || strings.Contains(labels, "secret-hash") || len(redacted) != 1 {
		t.Fatalf("project %q labels %s redacted %v", project, labels, redacted)
	}
	var dump string
	if err := e.pool.QueryRow(context.Background(), "SELECT string_agg(c::text, ' ') FROM containers c").Scan(&dump); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dump, "apr1") {
		t.Fatal("a filtered label value reached the database")
	}

	// An empty snapshot marks the container absent, without deleting it.
	snap.Containers = nil
	if status, _ := e.do(agent, "PUT", agentproto.PathSnapshot, snap); status != http.StatusNoContent {
		t.Fatal(status)
	}
	var removed bool
	_ = e.pool.QueryRow(context.Background(), "SELECT removed_at IS NOT NULL FROM containers WHERE id = $1", cid).Scan(&removed)
	if !removed {
		t.Fatal("container not marked absent")
	}

	status, body := e.do(agent, "POST", agentproto.PathHeartbeat, agentproto.Heartbeat{DockerOK: true})
	if status != http.StatusOK || !strings.Contains(string(body), `"rescan_seconds":300`) {
		t.Fatalf("heartbeat: %d %s", status, body)
	}
}

func TestEnrollmentCodeIsSingleUse(t *testing.T) {
	e := setup(t)
	code, err := e.svc.CreateEnrollment(context.Background(), "qnap-ts464")
	if err != nil {
		t.Fatal(err)
	}
	_, csr := newCSR(t)
	req := agentproto.EnrollRequest{Code: code, CSR: csr}
	if status, _ := e.do(e.client(nil), "POST", agentproto.PathEnroll, req); status != http.StatusOK {
		t.Fatal(status)
	}
	if status, _ := e.do(e.client(nil), "POST", agentproto.PathEnroll, req); status != http.StatusForbidden {
		t.Fatalf("second use: %d", status)
	}
	// A bad request does not burn a valid code.
	code, _ = e.svc.CreateEnrollment(context.Background(), "qnap-ts464")
	if status, _ := e.do(e.client(nil), "POST", agentproto.PathEnroll, agentproto.EnrollRequest{Code: code, CSR: []byte("junk")}); status != http.StatusBadRequest {
		t.Fatal(status)
	}
	if status, _ := e.do(e.client(nil), "POST", agentproto.PathEnroll, agentproto.EnrollRequest{Code: code, CSR: csr}); status != http.StatusOK {
		t.Fatalf("code burned by a bad request: %d", status)
	}
}

func TestAgentRoutesNeedAKnownCertificate(t *testing.T) {
	e := setup(t)
	hb := agentproto.Heartbeat{DockerOK: true}
	if status, _ := e.do(e.client(nil), "POST", agentproto.PathHeartbeat, hb); status != http.StatusUnauthorized {
		t.Fatalf("no certificate: %d", status)
	}

	// Signed by the CA but never recorded (e.g. a database restored from an older backup).
	key, csr := newCSR(t)
	stray, err := e.ca.SignAgent(csr, "rpi5", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	strayClient := e.client(&tls.Certificate{Certificate: [][]byte{stray.Raw}, PrivateKey: key})
	if status, _ := e.do(strayClient, "POST", agentproto.PathHeartbeat, hb); status != http.StatusForbidden {
		t.Fatalf("unknown certificate: %d", status)
	}

	// Signed by another CA: the handshake itself fails.
	other, _ := pki.NewCA(time.Now())
	foreign, _ := other.SignAgent(csr, "rpi5", time.Now())
	req, _ := http.NewRequest("POST", e.srv.URL+agentproto.PathHeartbeat, strings.NewReader("{}"))
	if _, err := e.client(&tls.Certificate{Certificate: [][]byte{foreign.Raw}, PrivateKey: key}).Do(req); err == nil {
		t.Fatal("a certificate from another CA was accepted")
	}

	agent := e.client(e.enroll("rpi5"))
	if status, _ := e.do(agent, "POST", agentproto.PathHeartbeat, hb); status != http.StatusOK {
		t.Fatal(status)
	}
	if n, err := e.svc.Revoke(context.Background(), "rpi5"); err != nil || n != 1 {
		t.Fatalf("revoke: %d %v", n, err)
	}
	if status, _ := e.do(agent, "POST", agentproto.PathHeartbeat, hb); status != http.StatusForbidden {
		t.Fatalf("revoked: %d", status)
	}
}

func TestRenewNotTooEarly(t *testing.T) {
	e := setup(t)
	agent := e.client(e.enroll("rpi5"))
	_, csr := newCSR(t)
	if status, _ := e.do(agent, "POST", agentproto.PathRenew, agentproto.RenewRequest{CSR: csr}); status != http.StatusConflict {
		t.Fatalf("renew at once: %d", status)
	}
}

func TestRejectsBadPayloads(t *testing.T) {
	e := setup(t)
	agent := e.client(e.enroll("rpi5"))
	bad := []any{
		agentproto.Snapshot{Containers: []agentproto.Container{{ID: "not-an-id"}}},
		map[string]any{"containers": []any{}, "env": []string{"A=B"}},
	}
	for i, body := range bad {
		if status, _ := e.do(agent, "PUT", agentproto.PathSnapshot, body); status != http.StatusBadRequest {
			t.Errorf("case %d: %d", i, status)
		}
	}
	if status, _ := e.do(agent, "POST", agentproto.PathContainers, agentproto.Changes{Removed: []string{"x"}}); status != http.StatusBadRequest {
		t.Errorf("bad removed id: %d", status)
	}
}
