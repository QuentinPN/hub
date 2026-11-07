package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"hub/server/internal/api"
	"hub/server/internal/fixture"
)

// containerStore adds one discovered service's containers to the fixture.
type containerStore struct {
	*fixture.Store
}

func (containerStore) ServiceContainers(_ context.Context, slug string, _ time.Time) (api.ServiceContainers, error) {
	if slug != "tz" {
		return api.ServiceContainers{}, api.ErrNotFound
	}
	return api.ServiceContainers{HostID: "mini", HostState: api.AgentConnected,
		Containers: []api.ServiceContainer{{ID: "0123456789ab", Name: "tz", Role: "main", Labels: 4, MaskedLabels: 1}},
		Removed:    []api.ServiceContainer{}}, nil
}

func TestServiceContainersRoute(t *testing.T) {
	e := newEnv(t, containerStore{fixture.New()})
	var res api.ServiceContainers
	e.getJSON(t, "/api/v1/services/tz/containers", &res)
	if res.HostID != "mini" || len(res.Containers) != 1 || res.Containers[0].MaskedLabels != 1 {
		t.Fatalf("containers %+v", res)
	}
	if status, _ := e.do(t, e.user, http.MethodGet, "/api/v1/services/nope/containers", "", nil); status != http.StatusNotFound {
		t.Fatalf("unknown service: status %d", status)
	}
	if status, _ := e.do(t, e.anon, http.MethodGet, "/api/v1/services/tz/containers", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("without session: status %d", status)
	}
	// Read only: no route acts on a container.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		if status, _ := e.do(t, e.user, method, "/api/v1/services/tz/containers", "", nil); status != http.StatusMethodNotAllowed && status != http.StatusNotFound {
			t.Errorf("%s: status %d", method, status)
		}
	}
}
