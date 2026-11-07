package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"

	"hub/server/internal/agentproto"
)

const (
	HeartbeatEvery = 15 * time.Second
	defaultRescan  = 5 * time.Minute
	backoffMin     = time.Second
	backoffMax     = time.Minute
)

type Agent struct {
	ServerURL string
	StateDir  string
	Version   string
	Docker    *Docker
	now       func() time.Time
}

// Run keeps a session with the server until ctx ends. A broken session (server, network or
// Docker) is retried with a growing, jittered delay; each new session starts with a full
// snapshot, which covers whatever happened in between.
func (a *Agent) Run(ctx context.Context) error {
	if a.now == nil {
		a.now = time.Now
	}
	delay := backoffMin
	for {
		started := a.now()
		err := a.session(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if a.now().Sub(started) > time.Minute {
			delay = backoffMin
		}
		if errors.Is(err, ErrExpired) {
			// Nothing will fix this but a new enrollment; keep checking, slowly, in case
			// the files are replaced.
			delay = backoffMax
		}
		wait := delay/2 + rand.N(delay) // ±50 %
		var down *dockerDownError
		if errors.As(err, &down) {
			// The server heard that Docker is down: retry at the heartbeat pace, so the host
			// stays « Docker injoignable » and does not drift to « injoignable ».
			delay, wait = backoffMin, HeartbeatEvery
		}
		slog.Warn("session ended", "err", err, "retry_in", wait.Round(time.Millisecond))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		delay = min(delay*2, backoffMax)
	}
}

type session struct {
	*Agent
	id     *Identity
	client *http.Client
}

func (a *Agent) session(ctx context.Context) error {
	id, err := LoadIdentity(a.StateDir)
	if err != nil {
		return err
	}
	if a.now().After(id.Leaf.NotAfter) {
		return ErrExpired
	}
	s := &session{Agent: a, id: id, client: id.Client()}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	version, api, err := a.Docker.Version(ctx)
	if err != nil {
		return s.dockerDown(ctx, err)
	}
	// Events first, then the list: a change made in between shows up in both, never in neither.
	events, evErr := a.Docker.Events(ctx)
	if err := s.snapshot(ctx, version, api); err != nil {
		return err
	}
	slog.Info("connected", "server", a.ServerURL, "docker", version, "cert_expires", id.Leaf.NotAfter.Format(time.RFC3339))
	rescan, err := s.heartbeat(ctx, nil)
	if err != nil {
		return err
	}

	beat := time.NewTicker(HeartbeatEvery)
	defer beat.Stop()
	scan := time.NewTimer(rescan)
	defer scan.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				return s.dockerDown(ctx, <-evErr)
			}
			if err := s.event(ctx, ev); err != nil {
				return err
			}
		case <-beat.C:
			next, err := s.heartbeat(ctx, nil)
			if err != nil {
				return err
			}
			if next != rescan {
				rescan = next
				scan.Reset(rescan)
			}
			if s.id.DueForRenewal(a.now()) {
				if err := s.renew(ctx); err != nil {
					slog.Error("certificate renewal failed, will retry", "err", err)
				}
			}
		case <-scan.C:
			if err := s.snapshot(ctx, version, api); err != nil {
				return err
			}
			scan.Reset(rescan)
		}
	}
}

func (s *session) snapshot(ctx context.Context, version, api string) error {
	list, err := s.Docker.Containers(ctx)
	if err != nil {
		return err
	}
	return s.send(ctx, http.MethodPut, agentproto.PathSnapshot, agentproto.Snapshot{
		AgentVersion: s.Version, DockerVersion: version, DockerAPI: api, Containers: list,
	}, nil)
}

func (s *session) event(ctx context.Context, ev Event) error {
	if !relevant(ev.Action) || !agentproto.ValidContainerID(ev.ID) {
		return nil
	}
	var ch agentproto.Changes
	if ev.Action == "destroy" {
		ch.Removed = []string{ev.ID}
	} else {
		list, err := s.Docker.Containers(ctx, ev.ID)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			ch.Removed = []string{ev.ID}
		}
		ch.Upserted = list
	}
	if agentproto.HistoryAction(ev.Action) {
		e := agentproto.Event{ContainerID: ev.ID, Action: ev.Action, ExitCode: ev.ExitCode}
		if e.Validate() == nil {
			ch.Events = []agentproto.Event{e}
		}
	}
	return s.send(ctx, http.MethodPost, agentproto.PathContainers, ch, nil)
}

// dockerDownError is a session ended by Docker (or its proxy) while the server still
// answers: the server already knows, from the heartbeat sent on the way out.
type dockerDownError struct{ err error }

func (e *dockerDownError) Error() string { return "docker: " + e.err.Error() }
func (e *dockerDownError) Unwrap() error { return e.err }

// dockerDown tells the server that Docker does not answer. If the server cannot be told
// either, the session failed for that reason instead.
func (s *session) dockerDown(ctx context.Context, err error) error {
	if err == nil {
		err = errors.New("event stream closed")
	}
	if _, hbErr := s.heartbeat(ctx, err); hbErr != nil {
		return hbErr
	}
	return &dockerDownError{err: err}
}

// heartbeat reports whether Docker answers and returns the rescan interval.
func (s *session) heartbeat(ctx context.Context, dockerErr error) (time.Duration, error) {
	h := agentproto.Heartbeat{DockerOK: dockerErr == nil}
	if dockerErr != nil {
		h.Error = dockerErr.Error()
	}
	var resp agentproto.HeartbeatResponse
	if err := s.send(ctx, http.MethodPost, agentproto.PathHeartbeat, h, &resp); err != nil {
		return 0, err
	}
	if resp.RescanSeconds <= 0 {
		return defaultRescan, nil
	}
	return time.Duration(resp.RescanSeconds) * time.Second, nil
}

func (s *session) renew(ctx context.Context) error {
	id, err := Renew(ctx, s.id, s.ServerURL, s.StateDir)
	if err != nil {
		return err
	}
	s.id, s.client = id, id.Client()
	slog.Info("certificate renewed", "expires", id.Leaf.NotAfter.Format(time.RFC3339))
	return nil
}

func (s *session) send(ctx context.Context, method, path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, s.ServerURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return &StatusError{Code: resp.StatusCode, Body: buf.String()}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
