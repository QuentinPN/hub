package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hub/server/internal/agentproto"
	"hub/server/internal/agentsrv"
	"hub/server/internal/api"
	"hub/server/internal/db/dbtest"
	"hub/server/internal/fixture"
	"hub/server/internal/pgstore"
	"hub/server/internal/secrets"
	"hub/server/internal/urlcheck"
	"hub/server/internal/widgets"

	"crypto/sha256"
)

func TestBeszelConnectionSignsInAndMapsHosts(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/collections/users/auth-with-password":
			var body struct{ Identity, Password string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Identity != "hub@local" || body.Password != "un mot de passe" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"token":"t"}`))
		case "/api/collections/systems/records":
			if r.Header.Get("Authorization") != "t" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"items":[{"name":"MINI","status":"up","info":{"cpu":10,"mp":20,"dp":30}},{"name":"autre","status":"up","info":{"cpu":1,"mp":2,"dp":3}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fake.Close()

	ctx := context.Background()
	store, err := pgstore.New(ctx, dbtest.Pool(t), fixture.Empty())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureHost(ctx, "mini", "mini"); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("mini"))
	if err := store.AddAgentCert(ctx, agentsrv.CertRecord{Serial: "a1", HostID: "mini", Fingerprint: sum[:],
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	snap := agentproto.Snapshot{Containers: []agentproto.Container{{
		ID: strings.Repeat("c", 64), Name: "beszel", Image: "henrygd/beszel", State: "running",
		Labels: map[string]string{agentproto.LabelComposeProject: "beszel", agentproto.LabelComposeService: "beszel"},
	}}}
	if err := store.ReplaceContainers(ctx, "mini", snap, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveServiceURL(ctx, "beszel", fake.URL, "", urlcheck.Result{Kind: urlcheck.KindOK, Status: 200}); err != nil {
		t.Fatal(err)
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
	e := newFullEnv(t, store, nil, &api.Widgets{Keys: keys, Bus: testBus})

	// An identity alone is not enough, and neither is a password alone.
	for _, body := range []string{`{"apiKey":"","identity":"hub@local"}`, `{"apiKey":"un mot de passe","identity":""}`} {
		if status, _ := e.do(t, e.user, http.MethodPost, "/api/v1/services/beszel/connection/test", body, nil); status != http.StatusBadRequest {
			t.Errorf("%s: %d", body, status)
		}
	}
	var test api.WidgetTest
	e.sendJSON(t, http.MethodPost, "/api/v1/services/beszel/connection/test", `{"apiKey":"faux","identity":"hub@local"}`, &test)
	if test.Result != widgets.StateAuth {
		t.Errorf("%+v", test)
	}
	e.sendJSON(t, http.MethodPost, "/api/v1/services/beszel/connection/test", `{"apiKey":"un mot de passe","identity":"hub@local"}`, &test)
	if test.Result != widgets.StateOK || len(test.Options) != 1 {
		t.Fatalf("%+v", test)
	}
	o := test.Options[0]
	if o.ID != "host:mini" || o.Choices[0].Value != "MINI" {
		t.Errorf("the matching system comes first: %+v", o)
	}

	var saved api.WidgetSaved
	_, raw := e.sendJSON(t, http.MethodPut, "/api/v1/services/beszel/connection",
		`{"apiKey":"un mot de passe","identity":"hub@local","widgets":["systems"],"refreshSeconds":30,"options":{"host:mini":"MINI"}}`, &saved)
	if !saved.Saved || saved.Service.Widget.Connection.KeyHint != "hub@local" {
		t.Fatalf("%s", raw)
	}
	if strings.Contains(string(raw), "un mot de passe") {
		t.Error("the password came back")
	}

	var infra api.InfraPage
	e.sendJSON(t, http.MethodGet, "/api/v1/sections/infra", "", &infra)
	if !infra.HasMetrics || len(infra.Hosts) != 1 || len(infra.Hosts[0].Metrics) != 3 || infra.Hosts[0].Metrics[0] != (api.HostMetric{Label: "CPU", Percent: 10}) {
		t.Errorf("%+v", infra)
	}
	var home api.Home
	e.sendJSON(t, http.MethodGet, "/api/v1/home", "", &home)
	if len(home.Hosts) != 1 || len(home.Hosts[0].Metrics) != 3 {
		t.Errorf("%+v", home.Hosts)
	}

	// The stored password serves again without being retyped.
	body := `{"apiKey":"","identity":"","widgets":["systems"],"refreshSeconds":60,"options":{"host:mini":""}}`
	saved = api.WidgetSaved{}
	e.sendJSON(t, http.MethodPut, "/api/v1/services/beszel/connection", body, &saved)
	if !saved.Saved || saved.Service.Widget.Connection.KeyHint != "hub@local" {
		t.Errorf("%+v", saved)
	}
	e.sendJSON(t, http.MethodGet, "/api/v1/sections/infra", "", &infra)
	if len(infra.Hosts[0].Metrics) != 0 {
		t.Errorf("host left without a system: %+v", infra.Hosts[0])
	}
}
