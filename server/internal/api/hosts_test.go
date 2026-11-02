package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"hub/server/internal/agentsrv"
	"hub/server/internal/api"
	"hub/server/internal/fixture"
)

// hostStore adds a fixed host list to the fixture, standing in for PostgreSQL.
type hostStore struct {
	*fixture.Store
}

func (hostStore) AgentHosts(context.Context, time.Time) ([]api.AgentHost, error) {
	return []api.AgentHost{{ID: "rpi5", Name: "Raspberry Pi", State: api.AgentNone}}, nil
}

func (hostStore) HostContainers(_ context.Context, id string) ([]api.Container, error) {
	if id != "rpi5" {
		return nil, api.ErrNotFound
	}
	return []api.Container{{ID: "0123456789ab", Name: "hawser", Labels: 4, MaskedLabels: 1}}, nil
}

// fakeAdmin records what it is asked, standing in for agentsrv.Service.
type fakeAdmin struct {
	calls []string
	by    []agentsrv.Actor
}

func (f *fakeAdmin) CreateEnrollment(_ context.Context, id, name string, by agentsrv.Actor) (agentsrv.Enrollment, error) {
	if id == "Bad Id" {
		return agentsrv.Enrollment{}, fmt.Errorf("%w host id", agentsrv.ErrInvalid)
	}
	f.calls, f.by = append(f.calls, "enroll "+id+" "+name), append(f.by, by)
	return agentsrv.Enrollment{HostID: id, Code: "one-time-code", ExpiresAt: time.Date(2026, 11, 2, 18, 15, 0, 0, time.UTC)}, nil
}

func (f *fakeAdmin) CancelEnrollment(_ context.Context, id string, by agentsrv.Actor) (int, error) {
	f.calls, f.by = append(f.calls, "cancel "+id), append(f.by, by)
	return 1, nil
}

func (f *fakeAdmin) Revoke(_ context.Context, id string, by agentsrv.Actor) (int, error) {
	f.calls, f.by = append(f.calls, "revoke "+id), append(f.by, by)
	return 2, nil
}

func (*fakeAdmin) Pin() string { return "sha256:00ff" }

func TestHostsWithoutAgentListener(t *testing.T) {
	e := newEnv(t, hostStore{fixture.New()})
	var res api.HostsResponse
	e.getJSON(t, "/api/v1/hosts", &res)
	if res.AgentsEnabled || res.Pin != "" || len(res.Hosts) != 1 {
		t.Fatalf("unexpected hosts: %+v", res)
	}
	// Nothing to enroll with: the write routes do not exist.
	if status, _ := e.do(t, e.user, http.MethodPost, "/api/v1/hosts/enrollments", `{"hostId":"rpi5","name":""}`, nil); status != http.StatusNotFound {
		t.Fatalf("enroll without listener: status %d", status)
	}
	if status, _ := e.do(t, e.user, http.MethodPost, "/api/v1/hosts/rpi5/revoke", "", nil); status != http.StatusNotFound {
		t.Fatalf("revoke without listener: status %d", status)
	}
}

func TestHostContainers(t *testing.T) {
	e := newEnv(t, hostStore{fixture.New()})
	var list []api.Container
	e.getJSON(t, "/api/v1/hosts/rpi5/containers", &list)
	if len(list) != 1 || list[0].MaskedLabels != 1 {
		t.Fatalf("containers %+v", list)
	}
	if status, _ := e.do(t, e.user, http.MethodGet, "/api/v1/hosts/nas/containers", "", nil); status != http.StatusNotFound {
		t.Fatalf("unknown host: status %d", status)
	}
	if status, _ := e.do(t, e.anon, http.MethodGet, "/api/v1/hosts/rpi5/containers", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("without session: status %d", status)
	}
}

func TestEnrollCancelRevoke(t *testing.T) {
	admin := &fakeAdmin{}
	e := newEnvWithAgents(t, hostStore{fixture.New()}, &api.Agents{Admin: admin, ServerURL: "https://192.0.2.14:8443"})

	var hosts api.HostsResponse
	e.getJSON(t, "/api/v1/hosts", &hosts)
	if !hosts.AgentsEnabled || hosts.ServerURL != "https://192.0.2.14:8443" || hosts.Pin != "sha256:00ff" {
		t.Fatalf("hosts %+v", hosts)
	}

	refused := []struct {
		name, method, path, body string
		header                   map[string]string
		want                     int
	}{
		{"anonymous", http.MethodPost, "/api/v1/hosts/enrollments", `{"hostId":"rpi5","name":""}`, nil, http.StatusUnauthorized},
		{"cross-site", http.MethodPost, "/api/v1/hosts/enrollments", `{"hostId":"rpi5","name":""}`, map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"unknown field", http.MethodPost, "/api/v1/hosts/enrollments", `{"hostId":"rpi5","admin":true}`, nil, http.StatusBadRequest},
		{"invalid id", http.MethodPost, "/api/v1/hosts/enrollments", `{"hostId":"Bad Id","name":""}`, nil, http.StatusBadRequest},
		{"invalid path id", http.MethodPost, "/api/v1/hosts/Bad%20Id/revoke", "", nil, http.StatusNotFound},
		{"anonymous revoke", http.MethodPost, "/api/v1/hosts/rpi5/revoke", "", nil, http.StatusUnauthorized},
	}
	for _, c := range refused {
		client := e.user
		if strings.HasPrefix(c.name, "anonymous") {
			client = e.anon
		}
		if status, _ := e.do(t, client, c.method, c.path, c.body, c.header); status != c.want {
			t.Errorf("%s: status %d, want %d", c.name, status, c.want)
		}
	}
	if len(admin.calls) != 0 {
		t.Fatalf("a refused request reached the agents: %v", admin.calls)
	}

	status, body := e.do(t, e.user, http.MethodPost, "/api/v1/hosts/enrollments", `{"hostId":"rpi5","name":"Raspberry Pi"}`, nil)
	var enr api.EnrollmentResponse
	if status != http.StatusCreated || json.Unmarshal(body, &enr) != nil || enr.Code != "one-time-code" || enr.Pin != "sha256:00ff" || enr.ServerURL != hosts.ServerURL {
		t.Fatalf("enroll: %d %s", status, body)
	}
	if status, _ := e.do(t, e.user, http.MethodDelete, "/api/v1/hosts/rpi5/enrollment", "", nil); status != http.StatusNoContent {
		t.Fatalf("cancel: status %d", status)
	}
	status, body = e.do(t, e.user, http.MethodPost, "/api/v1/hosts/rpi5/revoke", "", nil)
	if status != http.StatusOK || strings.TrimSpace(string(body)) != `{"revoked":2}` {
		t.Fatalf("revoke: %d %s", status, body)
	}

	want := []string{"enroll rpi5 Raspberry Pi", "cancel rpi5", "revoke rpi5"}
	if strings.Join(admin.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls %v", admin.calls)
	}
	// The audit log names the signed-in user, not the command line.
	for _, by := range admin.by {
		if by.UserID == nil || by.IP == "" {
			t.Fatalf("actor %+v", by)
		}
	}
}

func TestAgentState(t *testing.T) {
	now := time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	later := ago(-24 * time.Hour)
	cases := []struct {
		name string
		f    api.AgentFacts
		want api.AgentState
	}{
		{"never enrolled", api.AgentFacts{}, api.AgentNone},
		{"code waiting", api.AgentFacts{EnrollmentExpiresAt: later}, api.AgentPending},
		{"connected", api.AgentFacts{CertExpiresAt: later, LastSeenAt: ago(10 * time.Second), DockerOK: true}, api.AgentConnected},
		{"docker down", api.AgentFacts{CertExpiresAt: later, LastSeenAt: ago(10 * time.Second)}, api.AgentDockerDown},
		{"silent", api.AgentFacts{CertExpiresAt: later, LastSeenAt: ago(api.AgentSeenWindow + time.Second), DockerOK: true}, api.AgentOffline},
		{"enrolled, never seen", api.AgentFacts{CertExpiresAt: later}, api.AgentOffline},
		{"new code for a connected host", api.AgentFacts{CertExpiresAt: later, LastSeenAt: ago(time.Second), DockerOK: true, EnrollmentExpiresAt: later}, api.AgentConnected},
		{"new code after revocation", api.AgentFacts{EverIssued: true, EverRevoked: true, EnrollmentExpiresAt: later}, api.AgentPending},
		{"revoked", api.AgentFacts{EverIssued: true, EverRevoked: true}, api.AgentRevoked},
		{"expired", api.AgentFacts{EverIssued: true}, api.AgentExpired},
	}
	for _, c := range cases {
		if got := c.f.State(now); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}
