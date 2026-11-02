package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"hub/server/internal/agentsrv"
	"hub/server/internal/pki"
)

// HostReader is implemented by stores that know the hosts and what their agents report.
// Without it, the host routes are not registered.
type HostReader interface {
	AgentHosts(ctx context.Context, now time.Time) ([]AgentHost, error)
	// HostContainers returns ErrNotFound for an unknown host.
	HostContainers(ctx context.Context, hostID string) ([]Container, error)
}

// AgentAdmin enrolls and revokes agents; *agentsrv.Service implements it.
type AgentAdmin interface {
	CreateEnrollment(ctx context.Context, hostID, name string, by agentsrv.Actor) (agentsrv.Enrollment, error)
	CancelEnrollment(ctx context.Context, hostID string, by agentsrv.Actor) (int, error)
	Revoke(ctx context.Context, hostID string, by agentsrv.Actor) (int, error)
	Pin() string
}

// Agents is the agent listener as the UI needs it. nil when HUB_AGENT_CA_FILE is not set:
// hosts are listed, but nothing can be enrolled.
type Agents struct {
	Admin AgentAdmin
	// ServerURL is what an agent dials, written into the instructions shown at enrollment.
	ServerURL string
}

// AgentState sums up a host's agent for the UI. The server decides it, so every screen
// agrees on when an agent counts as gone.
type AgentState string

const (
	AgentNone       AgentState = "none"        // never enrolled
	AgentPending    AgentState = "pending"     // a code is waiting for its agent
	AgentConnected  AgentState = "connected"   // seen recently, Docker answers
	AgentDockerDown AgentState = "docker_down" // seen recently, Docker (or its proxy) does not answer
	AgentOffline    AgentState = "offline"     // valid certificate, not seen recently
	AgentExpired    AgentState = "expired"     // its certificate ran out without being renewed
	AgentRevoked    AgentState = "revoked"
)

// AgentSeenWindow is how long an agent counts as connected after its last request: three
// heartbeats (agentproto: one every 15 s).
const AgentSeenWindow = 45 * time.Second

// AgentFacts is what the database knows of a host's agent, before it is summed up.
type AgentFacts struct {
	LastSeenAt          *time.Time
	DockerOK            bool
	CertExpiresAt       *time.Time // latest valid, unrevoked certificate
	EnrollmentExpiresAt *time.Time // pending code
	EverIssued          bool
	EverRevoked         bool
}

// State applies the rules in order: a working certificate wins over a pending code, so
// enrolling again a connected host keeps it shown as connected until the new agent comes.
func (f AgentFacts) State(now time.Time) AgentState {
	switch {
	case f.CertExpiresAt != nil:
		if f.LastSeenAt == nil || now.Sub(*f.LastSeenAt) > AgentSeenWindow {
			return AgentOffline
		}
		if !f.DockerOK {
			return AgentDockerDown
		}
		return AgentConnected
	case f.EnrollmentExpiresAt != nil:
		return AgentPending
	case f.EverRevoked:
		return AgentRevoked
	case f.EverIssued:
		return AgentExpired
	}
	return AgentNone
}

func registerHosts(mux *http.ServeMux, hosts HostReader, agents *Agents, audit func(*http.Request) agentsrv.Actor) {
	mux.HandleFunc("GET /api/v1/hosts", func(w http.ResponseWriter, r *http.Request) {
		list, err := hosts.AgentHosts(r.Context(), time.Now())
		res := HostsResponse{Hosts: list}
		if agents != nil {
			res.AgentsEnabled, res.ServerURL, res.Pin = true, agents.ServerURL, agents.Admin.Pin()
		}
		respond(w, r)(res, err)
	})
	mux.HandleFunc("GET /api/v1/hosts/{id}/containers", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(hosts.HostContainers(r.Context(), r.PathValue("id")))
	})
	if agents == nil {
		return
	}
	mux.HandleFunc("POST /api/v1/hosts/enrollments", func(w http.ResponseWriter, r *http.Request) {
		var req EnrollmentRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		e, err := agents.Admin.CreateEnrollment(r.Context(), req.HostID, req.Name, audit(r))
		if errors.Is(err, agentsrv.ErrInvalid) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, EnrollmentResponse{
			HostID: e.HostID, Code: e.Code, ExpiresAt: e.ExpiresAt, Pin: agents.Admin.Pin(), ServerURL: agents.ServerURL,
		})
	})
	mux.HandleFunc("DELETE /api/v1/hosts/{id}/enrollment", func(w http.ResponseWriter, r *http.Request) {
		id, ok := hostID(w, r)
		if !ok {
			return
		}
		if _, err := agents.Admin.CancelEnrollment(r.Context(), id, audit(r)); err != nil {
			internalError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/hosts/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
		id, ok := hostID(w, r)
		if !ok {
			return
		}
		n, err := agents.Admin.Revoke(r.Context(), id, audit(r))
		if err != nil {
			internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"revoked": n})
	})
}

func hostID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !pki.ValidHostID(id) {
		writeError(w, http.StatusNotFound, "not found")
		return "", false
	}
	return id, true
}
