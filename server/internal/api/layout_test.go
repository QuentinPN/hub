package api_test

import (
	"net/http"
	"testing"

	"hub/server/internal/api"
)

func TestLayoutRoutesRefuseWhatTheyDoNotKnow(t *testing.T) {
	e, _, _ := widgetEnv(t, "")

	var l api.HomeLayout
	e.sendJSON(t, http.MethodGet, "/api/v1/home-layout", "", &l)
	if len(l.Rows) != len(api.DefaultRows) {
		t.Fatalf("%+v", l)
	}
	for name, body := range map[string]string{
		"unknown row": `{"rows":[{"id":"nope","visible":true,"excluded":[]}]}`,
		"twice":       `{"rows":[{"id":"hosts","visible":true,"excluded":[]},{"id":"hosts","visible":false,"excluded":[]}]}`,
		"empty id":    `{"rows":[{"id":"hosts","visible":true,"excluded":[""]}]}`,
	} {
		if status, _ := e.do(t, e.user, http.MethodPut, "/api/v1/home-layout", body, nil); status != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, status)
		}
	}

	e.sendJSON(t, http.MethodPut, "/api/v1/home-layout", `{"rows":[{"id":"hosts","visible":false,"excluded":[]}]}`, &l)
	if l.Rows[1].ID != api.RowHosts || l.Rows[1].Visible {
		t.Errorf("%+v", l.Rows)
	}
	e.sendJSON(t, http.MethodDelete, "/api/v1/home-layout", "", &l)
	if l.Rows[1].ID != api.RowFavorites {
		t.Errorf("%+v", l.Rows)
	}
}
