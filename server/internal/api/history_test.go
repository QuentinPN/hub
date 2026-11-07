package api_test

import (
	"net/http"
	"testing"

	"hub/server/internal/api"
)

func TestHistoryRoute(t *testing.T) {
	e := discoveredEnv(t)

	if status, _ := e.do(t, e.anon, http.MethodGet, "/api/v1/services/emby/history", "", nil); status != http.StatusUnauthorized {
		t.Errorf("anonymous: status %d", status)
	}
	for path, want := range map[string]int{
		"/api/v1/services/nope/history":              http.StatusNotFound,
		"/api/v1/services/emby/history?kinds=crash":  http.StatusBadRequest,
		"/api/v1/services/emby/history?limit=0":      http.StatusBadRequest,
		"/api/v1/services/emby/history?limit=101":    http.StatusBadRequest,
		"/api/v1/services/emby/history?offset=-1":    http.StatusBadRequest,
		"/api/v1/services/emby/history?offset=x":     http.StatusBadRequest,
		"/api/v1/services/emby/history?kinds=outage": http.StatusOK,
	} {
		if status, data := e.do(t, e.user, http.MethodGet, path, "", nil); status != want {
			t.Errorf("%s: status %d, want %d: %s", path, status, want, data)
		}
	}

	var h api.ServiceHistory
	e.getJSON(t, "/api/v1/services/emby/history", &h)
	if h.Total != 0 || len(h.Events) != 0 || len(h.Days) != api.HistoryBarDays || h.RetentionDays != 90 || h.Limit != 25 || h.Since.IsZero() {
		t.Fatalf("empty history: %+v", h)
	}

	// Edits made in Hub land in the history, with the audit kind as their action.
	var svc api.ServiceDetail
	if status, _ := e.sendJSON(t, http.MethodPatch, "/api/v1/services/emby", `{"name":"Cinéma","category":"Salon"}`, &svc); status != http.StatusOK {
		t.Fatalf("edit: status %d", status)
	}
	if status, _ := e.sendJSON(t, http.MethodPost, "/api/v1/services/emby/reset", "", &svc); status != http.StatusOK {
		t.Fatalf("reset: status %d", status)
	}
	e.getJSON(t, "/api/v1/services/emby/history?kinds=edit,alert&limit=1", &h)
	if h.Total != 2 || len(h.Events) != 1 || h.Events[0].Action != "service_reset" || h.Counts[api.HistoryEdit] != 2 {
		t.Fatalf("edits: total %d, %+v", h.Total, h.Events)
	}
	e.getJSON(t, "/api/v1/services/emby/history?kinds=edit&offset=1", &h)
	if len(h.Events) != 1 || h.Events[0].Action != "service_updated" || h.Offset != 1 {
		t.Fatalf("second page: %+v", h.Events)
	}
}
