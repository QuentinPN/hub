// Package agentsrv is the server side of the Docker agents (AGENT-STUDY.md): a listener
// of its own, in mutual TLS, where agents enroll with a one-time code, renew their
// certificate and push what they see. Agents always open the connection; the server has
// no way to send them an order.
package agentsrv

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"sync"
	"time"

	"hub/server/internal/agentproto"
	"hub/server/internal/auth"
	"hub/server/internal/pki"
)

// EnrollmentTTL is how long a code stays usable.
const EnrollmentTTL = 15 * time.Minute

// MaxHostName is the longest display name a host may have.
const MaxHostName = 80

var (
	ErrNotFound = errors.New("not found")
	// ErrInvalid wraps a host id or name refused before anything is written.
	ErrInvalid = errors.New("invalid")
)

// Actor is who asked for an enrollment, a cancellation or a revocation: the signed-in user
// from the UI, or the command line on the server.
type Actor struct {
	UserID *int64
	IP     string
}

// CommandLine is the actor of the -enroll-host and -revoke-host flags.
var CommandLine = Actor{IP: "command line"}

// Enrollment is a code just issued. The code is shown once: only its hash is stored.
type Enrollment struct {
	HostID    string
	Code      string
	ExpiresAt time.Time
}

// CertRecord is an issued agent certificate as the database keeps it.
type CertRecord struct {
	Serial      string
	HostID      string
	Fingerprint []byte
	NotBefore   time.Time
	NotAfter    time.Time
	RevokedAt   *time.Time
}

// Repo is the storage the agent listener needs; pgstore implements it.
type Repo interface {
	// EnsureHost creates the host if it does not exist yet, named name (its id when empty).
	// An existing host is renamed only when name is not empty.
	EnsureHost(ctx context.Context, id, name string) error
	CreateEnrollment(ctx context.Context, hostID string, codeHash []byte, expires time.Time) error
	// CancelEnrollments deletes the unused codes of hostID and returns how many there were.
	CancelEnrollments(ctx context.Context, hostID string) (int, error)
	// ConsumeEnrollment marks the code used and returns its host. ErrNotFound when the code
	// is unknown, already used or expired.
	ConsumeEnrollment(ctx context.Context, codeHash []byte, now time.Time) (string, error)
	AddAgentCert(ctx context.Context, c CertRecord) error
	// AgentCert returns ErrNotFound for an unknown serial.
	AgentCert(ctx context.Context, serial string) (CertRecord, error)
	RevokeHostCerts(ctx context.Context, hostID string, now time.Time) (int, error)
	ReplaceContainers(ctx context.Context, hostID string, s agentproto.Snapshot, now time.Time) error
	ApplyContainerChanges(ctx context.Context, hostID string, c agentproto.Changes, now time.Time) error
	RecordHeartbeat(ctx context.Context, hostID string, h agentproto.Heartbeat, now time.Time) error
	RescanMinutes(ctx context.Context) (int, error)
	Audit(ctx context.Context, e auth.Event) error
}

type Service struct {
	ca      *pki.CA
	repo    Repo
	now     func() time.Time
	limiter *limiter
}

func New(ca *pki.CA, repo Repo) *Service {
	return &Service{ca: ca, repo: repo, now: time.Now, limiter: &limiter{failures: map[string][]time.Time{}}}
}

// CreateEnrollment issues a one-time code for hostID, creating the host if needed and
// naming it name when not empty. A host has at most one pending code: an older unused one
// stops working. Only the code's SHA-256 is stored.
func (s *Service) CreateEnrollment(ctx context.Context, hostID, name string, by Actor) (Enrollment, error) {
	if !pki.ValidHostID(hostID) {
		return Enrollment{}, fmt.Errorf("%w host id %q: lowercase letters, digits and dashes, 63 at most", ErrInvalid, hostID)
	}
	if len([]rune(name)) > MaxHostName {
		return Enrollment{}, fmt.Errorf("%w host name: %d characters at most", ErrInvalid, MaxHostName)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Enrollment{}, err
	}
	e := Enrollment{HostID: hostID, Code: base64.RawURLEncoding.EncodeToString(raw), ExpiresAt: s.now().Add(EnrollmentTTL)}
	if err := s.repo.EnsureHost(ctx, hostID, name); err != nil {
		return Enrollment{}, err
	}
	if _, err := s.repo.CancelEnrollments(ctx, hostID); err != nil {
		return Enrollment{}, err
	}
	if err := s.repo.CreateEnrollment(ctx, hostID, hashCode(e.Code), e.ExpiresAt); err != nil {
		return Enrollment{}, err
	}
	s.auditBy(ctx, "agent_enrollment_created", by, "host="+hostID)
	return e, nil
}

// CancelEnrollment makes the pending code of hostID unusable.
func (s *Service) CancelEnrollment(ctx context.Context, hostID string, by Actor) (int, error) {
	n, err := s.repo.CancelEnrollments(ctx, hostID)
	if err == nil && n > 0 {
		s.auditBy(ctx, "agent_enrollment_cancelled", by, "host="+hostID)
	}
	return n, err
}

// Revoke ends every certificate of hostID at once, and its pending code: the next request
// of its agent fails. Its containers stay in the inventory.
func (s *Service) Revoke(ctx context.Context, hostID string, by Actor) (int, error) {
	if _, err := s.repo.CancelEnrollments(ctx, hostID); err != nil {
		return 0, err
	}
	n, err := s.repo.RevokeHostCerts(ctx, hostID, s.now())
	if err == nil {
		s.auditBy(ctx, "agent_revoked", by, fmt.Sprintf("host=%s certs=%d", hostID, n))
	}
	return n, err
}

// TLSConfig serves a certificate for names (what agents dial: IPs or host names) and
// verifies client certificates against the CA when one is presented. Enrollment is the
// only route that works without one.
func (s *Service) TLSConfig(names []string) (*tls.Config, error) {
	cert, err := s.ca.ServerCertificate(names, s.now())
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.VerifyClientCertIfGiven,
		ClientCAs:    s.ca.Pool(),
	}, nil
}

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+agentproto.PathEnroll, s.enroll)
	mux.HandleFunc("POST "+agentproto.PathRenew, s.agentOnly(s.renew))
	mux.HandleFunc("PUT "+agentproto.PathSnapshot, s.agentOnly(s.snapshot))
	mux.HandleFunc("POST "+agentproto.PathContainers, s.agentOnly(s.changes))
	mux.HandleFunc("POST "+agentproto.PathHeartbeat, s.agentOnly(s.heartbeat))
	return mux
}

func (s *Service) enroll(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	now := s.now()
	if s.limiter.blocked(ip, now) {
		writeError(w, http.StatusTooManyRequests, "too many failed enrollments")
		return
	}
	var req agentproto.EnrollRequest
	if !decode(w, r, &req) {
		return
	}
	fail := func(status int, reason string) {
		s.limiter.fail(ip, now)
		s.audit(r.Context(), "agent_enrollment_failed", ip, reason)
		writeError(w, status, reason)
	}
	// Check the request before burning the code on it.
	if err := pki.CheckCSR(req.CSR); err != nil {
		fail(http.StatusBadRequest, "invalid certificate request")
		return
	}
	hostID, err := s.repo.ConsumeEnrollment(r.Context(), hashCode(req.Code), now)
	if errors.Is(err, ErrNotFound) {
		fail(http.StatusForbidden, "unknown, used or expired enrollment code")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	cert, err := s.issue(r.Context(), req.CSR, hostID, now)
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.audit(r.Context(), "agent_enrolled", ip, fmt.Sprintf("host=%s serial=%s hostname=%.64q", hostID, pki.Serial(cert), req.Hostname))
	writeJSON(w, agentproto.CertResponse{Cert: cert.Raw})
}

func (s *Service) renew(w http.ResponseWriter, r *http.Request, a agent) {
	var req agentproto.RenewRequest
	if !decode(w, r, &req) {
		return
	}
	now := s.now()
	// Not before half the lifetime: an agent stuck in a loop must not mint certificates.
	if now.Before(a.cert.NotBefore.Add(a.cert.NotAfter.Sub(a.cert.NotBefore) / 2)) {
		writeError(w, http.StatusConflict, "too early to renew")
		return
	}
	if err := pki.CheckCSR(req.CSR); err != nil {
		writeError(w, http.StatusBadRequest, "invalid certificate request")
		return
	}
	cert, err := s.issue(r.Context(), req.CSR, a.hostID, now)
	if err != nil {
		internalError(w, r, err)
		return
	}
	s.audit(r.Context(), "agent_renewed", clientIP(r), fmt.Sprintf("host=%s serial=%s", a.hostID, pki.Serial(cert)))
	writeJSON(w, agentproto.CertResponse{Cert: cert.Raw})
}

func (s *Service) issue(ctx context.Context, csr []byte, hostID string, now time.Time) (*x509.Certificate, error) {
	cert, err := s.ca.SignAgent(csr, hostID, now)
	if err != nil {
		return nil, err
	}
	fp := sha256.Sum256(cert.Raw)
	err = s.repo.AddAgentCert(ctx, CertRecord{
		Serial: pki.Serial(cert), HostID: hostID, Fingerprint: fp[:], NotBefore: cert.NotBefore, NotAfter: cert.NotAfter,
	})
	return cert, err
}

func (s *Service) snapshot(w http.ResponseWriter, r *http.Request, a agent) {
	var snap agentproto.Snapshot
	if !decode(w, r, &snap) {
		return
	}
	if len(snap.Containers) > agentproto.MaxContainers || len(snap.AgentVersion) > 64 || len(snap.DockerVersion) > 64 || len(snap.DockerAPI) > 16 {
		writeError(w, http.StatusBadRequest, "snapshot over the limits")
		return
	}
	if !sanitizeAll(w, snap.Containers) {
		return
	}
	if err := s.repo.ReplaceContainers(r.Context(), a.hostID, snap, s.now()); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) changes(w http.ResponseWriter, r *http.Request, a agent) {
	var c agentproto.Changes
	if !decode(w, r, &c) {
		return
	}
	if len(c.Upserted)+len(c.Removed) > agentproto.MaxContainers {
		writeError(w, http.StatusBadRequest, "too many changes")
		return
	}
	if !sanitizeAll(w, c.Upserted) {
		return
	}
	for _, id := range c.Removed {
		if !agentproto.ValidContainerID(id) {
			writeError(w, http.StatusBadRequest, "invalid container id")
			return
		}
	}
	if len(c.Events) > agentproto.MaxEvents {
		writeError(w, http.StatusBadRequest, "too many events")
		return
	}
	for _, e := range c.Events {
		if e.Validate() != nil {
			writeError(w, http.StatusBadRequest, "invalid event")
			return
		}
	}
	if err := s.repo.ApplyContainerChanges(r.Context(), a.hostID, c, s.now()); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) heartbeat(w http.ResponseWriter, r *http.Request, a agent) {
	var h agentproto.Heartbeat
	if !decode(w, r, &h) {
		return
	}
	if len(h.Error) > 512 {
		h.Error = h.Error[:512]
	}
	if err := s.repo.RecordHeartbeat(r.Context(), a.hostID, h, s.now()); err != nil {
		internalError(w, r, err)
		return
	}
	minutes, err := s.repo.RescanMinutes(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, agentproto.HeartbeatResponse{RescanSeconds: minutes * 60})
}

func sanitizeAll(w http.ResponseWriter, cs []agentproto.Container) bool {
	for i := range cs {
		if err := cs[i].Sanitize(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return false
		}
	}
	return true
}

// agent is the caller of a route behind agentOnly.
type agent struct {
	hostID string
	cert   *x509.Certificate
}

// agentOnly lets through a verified agent certificate that the database knows, for its
// own host, and not revoked. The host always comes from the certificate, never from the
// payload: an agent can only speak for its own host.
func (s *Service) agentOnly(next func(http.ResponseWriter, *http.Request, agent)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.VerifiedChains[0]) == 0 {
			writeError(w, http.StatusUnauthorized, "client certificate required")
			return
		}
		cert := r.TLS.VerifiedChains[0][0]
		hostID, err := pki.AgentHostID(cert)
		if err != nil {
			writeError(w, http.StatusForbidden, "not an agent certificate")
			return
		}
		rec, err := s.repo.AgentCert(r.Context(), pki.Serial(cert))
		fp := sha256.Sum256(cert.Raw)
		switch {
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusForbidden, "unknown certificate")
			return
		case err != nil:
			internalError(w, r, err)
			return
		case rec.RevokedAt != nil:
			slog.Warn("revoked agent certificate", "host", hostID, "serial", rec.Serial, "ip", clientIP(r))
			writeError(w, http.StatusForbidden, "certificate revoked")
			return
		case rec.HostID != hostID || string(rec.Fingerprint) != string(fp[:]):
			writeError(w, http.StatusForbidden, "certificate does not match its record")
			return
		}
		next(w, r, agent{hostID: hostID, cert: cert})
	}
}

func (s *Service) audit(ctx context.Context, kind, ip, detail string) {
	s.auditBy(ctx, kind, Actor{IP: ip}, detail)
}

func (s *Service) auditBy(ctx context.Context, kind string, by Actor, detail string) {
	if err := s.repo.Audit(ctx, auth.Event{Kind: kind, UserID: by.UserID, IP: by.IP, Detail: detail}); err != nil {
		slog.Error("audit", "kind", kind, "err", err)
	}
}

func hashCode(code string) []byte {
	sum := sha256.Sum256([]byte(code))
	return sum[:]
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "expected application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, agentproto.MaxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("agent request", "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// limiter blocks an IP after 5 failed enrollments in 10 minutes. The codes cannot be
// guessed (256 bits); this only keeps a misbehaving client from flooding the audit log.
type limiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

const (
	limitMax    = 5
	limitWindow = 10 * time.Minute
)

func (l *limiter) blocked(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(ip, now)) >= limitMax
}

func (l *limiter) fail(ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[ip] = append(l.recent(ip, now), now)
}

func (l *limiter) recent(ip string, now time.Time) []time.Time {
	list := l.failures[ip]
	for len(list) > 0 && now.Sub(list[0]) >= limitWindow {
		list = list[1:]
	}
	if len(list) == 0 {
		delete(l.failures, ip)
	}
	return list
}

// Pin is the CA pin an agent checks on its first contact.
func (s *Service) Pin() string { return s.ca.Pin() }
