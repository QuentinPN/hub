package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"hub/server/internal/api"
	"hub/server/internal/db/dbtest"
	"hub/server/internal/fixture"
	"hub/server/internal/pgstore"
)

func TestValidTimeZone(t *testing.T) {
	for zone, ok := range map[string]bool{
		"": true, "UTC": true, "Europe/Paris": true, "America/Argentina/Buenos_Aires": true, "Etc/GMT+2": true,
		"Local": false, "Mars/Olympus": false, "../etc/passwd": false, "Europe/": false, "/Europe/Paris": false,
		strings.Repeat("A", 65): false,
	} {
		if got := api.ValidTimeZone(zone) == ""; got != ok {
			t.Errorf("%q: valid %v, want %v", zone, got, ok)
		}
	}
}

func TestGeneralRoutes(t *testing.T) {
	store, err := pgstore.New(context.Background(), dbtest.Pool(t), fixture.Empty())
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, store)

	var state api.AuthState
	e.sendJSON(t, http.MethodGet, "/api/v1/auth/state", "", &state)
	if state.DisplayName != api.DefaultDisplayName || state.StartPage != api.StartHome || state.WidgetRefresh != api.RefreshNormal {
		t.Fatalf("signed in, defaults: %+v", state)
	}

	for name, body := range map[string]string{
		"empty name":    `{"displayName":"   ","startPage":"home","timeZone":"","widgetRefresh":"normal"}`,
		"long name":     `{"displayName":"` + strings.Repeat("é", 41) + `","startPage":"home","timeZone":"","widgetRefresh":"normal"}`,
		"control":       `{"displayName":"Hub\u0007","startPage":"home","timeZone":"","widgetRefresh":"normal"}`,
		"direction":     `{"displayName":"Hub\u202egnp.","startPage":"home","timeZone":"","widgetRefresh":"normal"}`,
		"invisible":     `{"displayName":"H\u200bub","startPage":"home","timeZone":"","widgetRefresh":"normal"}`,
		"unknown page":  `{"displayName":"Hub","startPage":"settings","timeZone":"","widgetRefresh":"normal"}`,
		"unknown zone":  `{"displayName":"Hub","startPage":"home","timeZone":"Mars/Olympus","widgetRefresh":"normal"}`,
		"unknown rate":  `{"displayName":"Hub","startPage":"home","timeZone":"","widgetRefresh":"fast"}`,
		"retention":     `{"displayName":"Hub","startPage":"home","timeZone":"","widgetRefresh":"normal","historyDays":45}`,
		"unknown field": `{"displayName":"Hub","startPage":"home","timeZone":"","widgetRefresh":"normal","version":"9"}`,
	} {
		if status, data := e.do(t, e.user, http.MethodPut, "/api/v1/settings/general", body, nil); status != http.StatusBadRequest {
			t.Errorf("%s: status %d %s", name, status, data)
		}
	}
	if status, _ := e.do(t, e.anon, http.MethodPut, "/api/v1/settings/general", `{}`, nil); status != http.StatusUnauthorized {
		t.Errorf("anonymous PUT: status %d", status)
	}
	if status, _ := e.do(t, e.anon, http.MethodGet, "/api/v1/settings/general", "", nil); status != http.StatusUnauthorized {
		t.Errorf("anonymous GET: status %d", status)
	}

	var got api.GeneralSettings
	status, _ := e.sendJSON(t, http.MethodPut, "/api/v1/settings/general",
		`{"displayName":"  Hub de Quentin ","startPage":"medias","timeZone":"Europe/Paris","widgetRefresh":"economy"}`, &got)
	// Without historyDays, the retention stays as it was.
	want := api.General{DisplayName: "Hub de Quentin", StartPage: api.StartMedias, TimeZone: "Europe/Paris", WidgetRefresh: api.RefreshEconomy, HistoryDays: 90}
	if status != http.StatusOK || got.General != want {
		t.Fatalf("status %d, %+v", status, got)
	}
	got = api.GeneralSettings{}
	e.getJSON(t, "/api/v1/settings/general", &got)
	if got.General != want {
		t.Errorf("GET after save: %+v", got)
	}
	status, _ = e.sendJSON(t, http.MethodPut, "/api/v1/settings/general",
		`{"displayName":"Hub de Quentin","startPage":"medias","timeZone":"Europe/Paris","widgetRefresh":"economy","historyDays":30}`, &got)
	if want.HistoryDays = 30; status != http.StatusOK || got.General != want {
		t.Errorf("retention: status %d, %+v", status, got)
	}

	// The login screen shows the name, and nothing else of the settings.
	status, data := e.do(t, e.anon, http.MethodGet, "/api/v1/auth/state", "", nil)
	if status != http.StatusOK || !strings.Contains(string(data), `"displayName":"Hub de Quentin"`) ||
		strings.Contains(string(data), "startPage") || strings.Contains(string(data), "widgetRefresh") {
		t.Errorf("anonymous state: %d %s", status, data)
	}
}
