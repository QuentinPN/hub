package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hub/server/internal/agentproto"
	"hub/server/internal/api"
	"hub/server/internal/db/dbtest"
	"hub/server/internal/fixture"
	"hub/server/internal/pgstore"
	"hub/server/internal/secrets"
	"hub/server/internal/urlcheck"
	"hub/server/internal/widgets"
)

const embyKey = "une-cle-emby-de-test-7f2a"

func fakeEmbyServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != embyKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/emby/Sessions":
			_, _ = w.Write([]byte(`[{"DeviceName":"Salon","NowPlayingItem":{"Name":"Le film","RunTimeTicks":6000000000},"PlayState":{"PositionTicks":3000000000}}]`))
		case "/emby/Users/Query":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"u1","Name":"admin"}]}`))
		case "/emby/Users/u1/Items/Latest":
			_, _ = w.Write([]byte(`[]`))
		case "/emby/Items/Counts":
			_, _ = w.Write([]byte(`{"MovieCount":3}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// widgetEnv is a discovered Emby with its address, and the secrets behind the API.
func widgetEnv(t *testing.T, address string) (*env, *pgstore.Store, *secrets.Store) {
	t.Helper()
	ctx := context.Background()
	store, err := pgstore.New(ctx, dbtest.Pool(t), fixture.Empty())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureHost(ctx, "mini", "mini"); err != nil {
		t.Fatal(err)
	}
	snap := agentproto.Snapshot{Containers: []agentproto.Container{{
		ID: strings.Repeat("a", 64), Name: "emby", Image: "linuxserver/emby", State: "running",
		Labels: map[string]string{agentproto.LabelComposeProject: "emby", agentproto.LabelComposeService: "emby"},
	}}}
	if err := store.ReplaceContainers(ctx, "mini", snap, time.Now()); err != nil {
		t.Fatal(err)
	}
	if address != "" {
		if err := store.SaveServiceURL(ctx, "emby", address, "", urlcheck.Result{Kind: urlcheck.KindOK, Status: 200}); err != nil {
			t.Fatal(err)
		}
	}
	line, err := secrets.NewKeyLine(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	kr, err := secrets.ParseKeyring([]byte(line + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	keys := secrets.NewStore(kr, store)
	return newFullEnv(t, store, nil, &api.Widgets{Keys: keys, Bus: testBus}), store, keys
}

var testBus = widgets.NewBus()

const saveBody = `{"apiKey":"` + embyKey + `","widgets":["sessions","latest","library"],"refreshSeconds":30,"options":{"userId":"u1"}}`

func TestWidgetKeyIsTestedSavedSealedAndNeverSentBack(t *testing.T) {
	e, _, keys := widgetEnv(t, fakeEmbyServer(t).URL)

	var test api.WidgetTest
	e.sendJSON(t, http.MethodPost, "/api/v1/services/emby/connection/test", `{"apiKey":"`+embyKey+`"}`, &test)
	if test.Result != widgets.StateOK || test.Note != "1 lecture en cours" || len(test.Options) != 1 {
		t.Fatalf("%+v", test)
	}

	var saved api.WidgetSaved
	_, raw := e.sendJSON(t, http.MethodPut, "/api/v1/services/emby/connection", saveBody, &saved)
	if !saved.Saved || saved.Service == nil || saved.Service.Widget == nil || saved.Service.Widget.Connection == nil {
		t.Fatalf("%s", raw)
	}
	c := saved.Service.Widget.Connection
	if c.State != "ok" || c.KeyHint != "7f2a" || c.Options["userId"] != "u1" || len(c.Data["sessions"]) == 0 {
		t.Errorf("%+v", c)
	}
	if strings.Contains(string(raw), embyKey) {
		t.Error("the key came back to the browser")
	}
	v, err := keys.Get(context.Background(), widgets.KeyName("emby"))
	if err != nil || v.Reveal() != embyKey {
		t.Errorf("stored key: %v", err)
	}

	var home api.Home
	e.sendJSON(t, http.MethodGet, "/api/v1/home", "", &home)
	if home.SessionsSlug != "emby" || len(home.Sessions) != 1 || home.Sessions[0].Title != "Le film" {
		t.Errorf("home: %+v", home)
	}

	var rows []api.WidgetRow
	e.sendJSON(t, http.MethodGet, "/api/v1/widgets", "", &rows)
	if len(rows) != 1 || rows[0].Slug != "emby" || rows[0].State != "ok" {
		t.Errorf("rows: %+v", rows)
	}

	// The stored key serves again when no key is typed: settings change without retyping it.
	body := `{"apiKey":"","widgets":["sessions"],"refreshSeconds":60,"options":{}}`
	saved = api.WidgetSaved{}
	e.sendJSON(t, http.MethodPut, "/api/v1/services/emby/connection", body, &saved)
	if !saved.Saved || saved.Service.Widget.Connection.KeyHint != "7f2a" || saved.Service.Widget.Connection.RefreshSeconds != 60 {
		t.Errorf("%+v", saved)
	}

	var detail api.ServiceDetail
	e.sendJSON(t, http.MethodDelete, "/api/v1/services/emby/connection", "", &detail)
	if detail.Widget == nil || detail.Widget.Connection != nil {
		t.Errorf("%+v", detail.Widget)
	}
	if _, err := keys.Get(context.Background(), widgets.KeyName("emby")); err == nil {
		t.Error("the key outlived its connection")
	}
}

func TestRefusedKeyIsNeverKept(t *testing.T) {
	e, _, keys := widgetEnv(t, fakeEmbyServer(t).URL)
	var saved api.WidgetSaved
	body := strings.Replace(saveBody, embyKey, "mauvaise-cle-mauvaise", 1)
	e.sendJSON(t, http.MethodPut, "/api/v1/services/emby/connection", body, &saved)
	if saved.Saved || saved.Result != widgets.StateAuth {
		t.Errorf("%+v", saved)
	}
	if _, err := keys.Get(context.Background(), widgets.KeyName("emby")); err == nil {
		t.Error("a refused key was stored")
	}
}

func TestWidgetSaveRefusals(t *testing.T) {
	e, _, _ := widgetEnv(t, fakeEmbyServer(t).URL)
	for name, body := range map[string]string{
		"no key":         `{"apiKey":"","widgets":["sessions"],"refreshSeconds":30,"options":{}}`,
		"control":        `{"apiKey":"a\nb","widgets":["sessions"],"refreshSeconds":30,"options":{}}`,
		"bad rate":       `{"apiKey":"k","widgets":["sessions"],"refreshSeconds":1,"options":{}}`,
		"no widget":      `{"apiKey":"k","widgets":[],"refreshSeconds":30,"options":{}}`,
		"unknown widget": `{"apiKey":"k","widgets":["nope"],"refreshSeconds":30,"options":{}}`,
	} {
		status, _ := e.do(t, e.user, http.MethodPut, "/api/v1/services/emby/connection", body, nil)
		if status != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, status)
		}
	}
	if status, _ := e.do(t, e.user, http.MethodPut, "/api/v1/services/nope/connection", saveBody, nil); status != http.StatusNotFound {
		t.Errorf("unknown service: %d", status)
	}

	bare, _, _ := widgetEnv(t, "")
	if status, data := bare.do(t, bare.user, http.MethodPut, "/api/v1/services/emby/connection", saveBody, nil); status != http.StatusBadRequest || !strings.Contains(string(data), "adresse") {
		t.Errorf("no address: %d %s", status, data)
	}
}

// A changed address could be anyone's: the key must be typed again to go there.
func TestStoredKeyDoesNotFollowAChangedAddress(t *testing.T) {
	first := fakeEmbyServer(t)
	var seen []string
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("X-Emby-Token"))
	}))
	defer other.Close()
	e, store, _ := widgetEnv(t, first.URL)
	var saved api.WidgetSaved
	e.sendJSON(t, http.MethodPut, "/api/v1/services/emby/connection", saveBody, &saved)
	if !saved.Saved {
		t.Fatalf("%+v", saved)
	}

	ctx := context.Background()
	if err := store.SaveServiceURL(ctx, "emby", other.URL, "", urlcheck.Result{Kind: urlcheck.KindOK, Status: 200}); err != nil {
		t.Fatal(err)
	}
	due, err := store.DueConnections(ctx, time.Now().Add(time.Hour))
	if err != nil || len(due) != 0 {
		t.Errorf("due after an address change: %+v, %v", due, err)
	}
	var detail api.ServiceDetail
	e.sendJSON(t, http.MethodGet, "/api/v1/services/emby", "", &detail)
	c := detail.Widget.Connection
	if c.State != api.WidgetAddressChanged || len(c.Data) != 0 {
		t.Errorf("%+v", c)
	}
	status, _ := e.do(t, e.user, http.MethodPost, "/api/v1/services/emby/connection/test", `{"apiKey":""}`, nil)
	if status != http.StatusBadRequest {
		t.Errorf("stored key tested on the new address: %d", status)
	}
	if len(seen) != 0 {
		t.Errorf("the key reached the new address: %v", seen)
	}
}
