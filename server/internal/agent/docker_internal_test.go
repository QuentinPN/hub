package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"hub/server/internal/agentproto"
)

func TestGetRefusesPathsOutsideTheList(t *testing.T) {
	d := NewDocker("/nonexistent.sock")
	for _, p := range []string{"/containers/abc/json", "/containers/abc/logs", "/containers/abc/archive", "/info", "/v1.47/containers/json", "/images/json"} {
		if _, err := d.get(context.Background(), p, nil); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("%s: %v", p, err)
		}
	}
}

func TestRelevant(t *testing.T) {
	for a, want := range map[string]bool{"start": true, "destroy": true, "health_status: healthy": true, "exec_create: sh": false, "attach": false, "top": false} {
		if relevant(a) != want {
			t.Errorf("%s: %v", a, !want)
		}
	}
}

// Docker gone, server up: the server is told, and the session ends with an error Run
// retries at the heartbeat pace. Server gone too: that is the error.
func TestDockerDownIsReported(t *testing.T) {
	var got agentproto.Heartbeat
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"rescan_seconds":300}`))
	}))
	defer srv.Close()
	s := &session{Agent: &Agent{ServerURL: srv.URL}, client: srv.Client()}

	err := s.dockerDown(context.Background(), errors.New("no such file or directory"))
	var down *dockerDownError
	if !errors.As(err, &down) || got.DockerOK || got.Error != "no such file or directory" {
		t.Fatalf("err %v, heartbeat %+v", err, got)
	}

	fail = true
	err = s.dockerDown(context.Background(), errors.New("no such file or directory"))
	var status *StatusError
	if errors.As(err, &down) || !errors.As(err, &status) {
		t.Fatalf("server down too: %v", err)
	}
}
