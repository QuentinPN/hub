package agent

import (
	"context"
	"errors"
	"testing"
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
