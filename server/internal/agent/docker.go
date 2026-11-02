// Package agent is the Docker agent (AGENT-STUDY.md, option 1.b): it reads Docker through
// the filtering proxy, keeps only what the server needs, and pushes it over mutual TLS.
package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"hub/server/internal/agentproto"
)

// allowedPaths are the only Docker API calls the agent makes; the proxy enforces the same
// list. Anything else is refused here before it leaves the process.
var allowedPaths = map[string]bool{"/_ping": true, "/version": true, "/containers/json": true, "/events": true}

var ErrNotAllowed = errors.New("docker call not allowed by the agent")

// Docker is a read-only client of the Docker API over a unix socket (the proxy's).
type Docker struct {
	hc *http.Client
}

func NewDocker(socket string) *Docker {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
		MaxIdleConns: 2,
	}
	return &Docker{hc: &http.Client{Transport: tr}}
}

func (d *Docker) get(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	if !allowedPaths[path] {
		return nil, fmt.Errorf("%w: %s", ErrNotAllowed, path)
	}
	u := url.URL{Scheme: "http", Host: "docker", Path: path, RawQuery: query.Encode()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("docker %s: %s", path, resp.Status)
	}
	return resp, nil
}

func (d *Docker) getJSON(ctx context.Context, path string, query url.Values, v any) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := d.get(ctx, path, query)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(v)
}

// Version returns the engine version and its API version.
func (d *Docker) Version(ctx context.Context) (version, api string, err error) {
	var v struct{ Version, APIVersion string }
	err = d.getJSON(ctx, "/version", nil, &v)
	return v.Version, v.APIVersion, err
}

// summary is the part of GET /containers/json the agent reads. Command, HostConfig,
// Mounts and NetworkSettings are never decoded.
type summary struct {
	ID      string `json:"Id"`
	Names   []string
	Image   string
	State   string
	Status  string
	Created int64
	Labels  map[string]string
	Ports   []struct {
		IP          string
		PrivatePort int
		PublicPort  int
		Type        string
	}
}

func (s summary) container() agentproto.Container {
	c := agentproto.Container{ID: s.ID, Image: s.Image, State: s.State, Status: s.Status}
	if len(s.Names) > 0 {
		c.Name = strings.TrimPrefix(s.Names[0], "/")
	}
	if s.Created > 0 {
		c.Created = time.Unix(s.Created, 0).UTC()
	}
	c.Labels, c.RedactedLabels = agentproto.FilterLabels(s.Labels)
	for _, p := range s.Ports {
		c.Ports = append(c.Ports, agentproto.Port{IP: p.IP, PrivatePort: p.PrivatePort, PublicPort: p.PublicPort, Type: p.Type})
	}
	return c
}

// Containers lists every container, stopped ones included, already filtered. With ids,
// only those.
func (d *Docker) Containers(ctx context.Context, ids ...string) ([]agentproto.Container, error) {
	q := url.Values{"all": {"1"}}
	if len(ids) > 0 {
		f, _ := json.Marshal(map[string][]string{"id": ids})
		q.Set("filters", string(f))
	}
	var list []summary
	if err := d.getJSON(ctx, "/containers/json", q, &list); err != nil {
		return nil, err
	}
	out := make([]agentproto.Container, 0, len(list))
	for _, s := range list {
		out = append(out, s.container())
	}
	return out, nil
}

// Event is a container event: only its action and container id. Its attributes (which
// carry every label) are never read.
type Event struct {
	Action string
	ID     string
}

// Events streams container events until ctx ends or the stream breaks; the error channel
// then receives why, once.
func (d *Docker) Events(ctx context.Context) (<-chan Event, <-chan error) {
	events := make(chan Event)
	errc := make(chan error, 1)
	go func() {
		defer close(events)
		f, _ := json.Marshal(map[string][]string{"type": {"container"}})
		resp, err := d.get(ctx, "/events", url.Values{"filters": {string(f)}})
		if err != nil {
			errc <- err
			return
		}
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			var raw struct {
				Action string
				Actor  struct{ ID string }
			}
			if json.Unmarshal(sc.Bytes(), &raw) != nil {
				continue
			}
			select {
			case events <- Event{Action: raw.Action, ID: raw.Actor.ID}:
			case <-ctx.Done():
				errc <- ctx.Err()
				return
			}
		}
		err = sc.Err()
		if err == nil {
			err = errors.New("docker event stream closed")
		}
		errc <- err
	}()
	return events, errc
}

// relevant reports whether an event can change what the server keeps. exec_*, attach,
// top, resize and the like cannot.
func relevant(action string) bool {
	action, _, _ = strings.Cut(action, ":") // "health_status: healthy"
	switch action {
	case "create", "start", "restart", "stop", "die", "kill", "pause", "unpause", "rename", "update", "destroy", "health_status", "oom":
		return true
	}
	return false
}
