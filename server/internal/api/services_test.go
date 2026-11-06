package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"hub/server/internal/domain"
	"hub/server/internal/fixture"
	"hub/server/internal/pgstore"
)

// sendJSON is do as the signed-in user, decoding the answer into out.
func (e *env) sendJSON(t *testing.T, method, path, body string, out any) (int, []byte) {
	t.Helper()
	status, data := e.do(t, e.user, method, path, body, nil)
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("%s %s: status %d, body %s: %v", method, path, status, data, err)
	}
	return status, data
}

func discoveredEnv(t *testing.T) *env {
	t.Helper()
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
		ID: strings.Repeat("a", 64), Name: "emby", Image: "linuxserver/emby", State: "running",
		Labels: map[string]string{agentproto.LabelComposeProject: "emby", agentproto.LabelComposeService: "emby"},
	}, {
		ID: strings.Repeat("b", 64), Name: "hawser", Image: "finsys/hawser", State: "running",
		Labels: map[string]string{agentproto.LabelComposeProject: "hawser", agentproto.LabelComposeService: "hawser"},
	}}}
	if err := store.ReplaceContainers(ctx, "mini", snap, time.Now()); err != nil {
		t.Fatal(err)
	}
	return newEnv(t, store)
}

func TestSetURLSavesWhatAnswers(t *testing.T) {
	e := discoveredEnv(t)
	target := httptest.NewServer(http.NotFoundHandler())
	defer target.Close()

	var res api.URLCheckResponse
	if status, _ := e.sendJSON(t, http.MethodPost, "/api/v1/services/emby/url", `{"url":"`+target.URL+`"}`, &res); status != http.StatusOK || !res.Saved || res.Result != "ok" {
		t.Fatalf("status %d, %+v", status, res)
	}
	var svc api.ServiceDetail
	e.getJSON(t, "/api/v1/services/emby", &svc)
	if svc.URL == nil || *svc.URL != target.URL || svc.Status != domain.StatusOnline || svc.URLCheck == nil || svc.URLCheck.CheckedAt == nil {
		t.Fatalf("service %+v, check %+v", svc.ServiceSummary, svc.URLCheck)
	}

	if status, _ := e.sendJSON(t, http.MethodDelete, "/api/v1/services/emby/url", "", &svc); status != http.StatusOK || svc.URL != nil {
		t.Fatalf("delete: status %d, %+v", status, svc.URL)
	}
}

func TestUnreachableNeedsForce(t *testing.T) {
	e := discoveredEnv(t)
	closed := httptest.NewServer(http.NotFoundHandler())
	url := closed.URL
	closed.Close()

	var res api.URLCheckResponse
	e.sendJSON(t, http.MethodPost, "/api/v1/services/emby/url", `{"url":"`+url+`"}`, &res)
	if res.Saved || res.Result != "unreachable" {
		t.Fatalf("%+v", res)
	}
	var svc api.ServiceDetail
	e.getJSON(t, "/api/v1/services/emby", &svc)
	if svc.URL != nil {
		t.Fatalf("saved anyway: %v", *svc.URL)
	}
	e.sendJSON(t, http.MethodPost, "/api/v1/services/emby/url", `{"url":"`+url+`","force":true}`, &res)
	if !res.Saved {
		t.Fatalf("force: %+v", res)
	}
	e.getJSON(t, "/api/v1/services/emby", &svc)
	if svc.URL == nil || svc.Status != domain.StatusDegraded || svc.Note != "adresse injoignable" {
		t.Fatalf("service %+v", svc.ServiceSummary)
	}
}

func TestCertificateOnlyPassesOnceAccepted(t *testing.T) {
	e := discoveredEnv(t)
	target := httptest.NewTLSServer(http.NotFoundHandler())
	defer target.Close()
	sum := sha256.Sum256(target.Certificate().Raw)
	fingerprint := hex.EncodeToString(sum[:])

	var res api.URLCheckResponse
	e.sendJSON(t, http.MethodPost, "/api/v1/services/emby/url", `{"url":"`+target.URL+`","force":true}`, &res)
	if res.Saved || res.Result != "cert" || res.CertSHA256 != fingerprint {
		t.Fatalf("force on a certificate: %+v", res)
	}
	e.sendJSON(t, http.MethodPost, "/api/v1/services/emby/url", `{"url":"`+target.URL+`","acceptCert":"`+strings.Repeat("0", 64)+`"}`, &res)
	if res.Saved {
		t.Fatalf("another fingerprint: %+v", res)
	}
	e.sendJSON(t, http.MethodPost, "/api/v1/services/emby/url", `{"url":"`+target.URL+`","acceptCert":"`+fingerprint+`"}`, &res)
	if !res.Saved || res.Result != "ok" {
		t.Fatalf("accepted: %+v", res)
	}
	var svc api.ServiceDetail
	e.getJSON(t, "/api/v1/services/emby", &svc)
	if !svc.URLCheck.CertPinned || svc.Status != domain.StatusOnline {
		t.Fatalf("check %+v, status %s", svc.URLCheck, svc.Status)
	}
}

func TestServiceWritesAreChecked(t *testing.T) {
	e := discoveredEnv(t)
	cases := []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/api/v1/services/emby/url", `{"url":"ftp://x.fr"}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/services/emby/url", `{"url":"http://u:p@x.fr"}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/services/emby/url", `{"url":"http://x.fr","acceptCert":"nope"}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/services/nope/url", `{"url":"http://127.0.0.1:1"}`, http.StatusNotFound},
		{http.MethodPatch, "/api/v1/services/emby", `{"name":" ","category":"X"}`, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/services/emby", `{"name":"E","category":""}`, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/services/emby", `{"name":"E","category":"X","icon":"../x"}`, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/services/emby", `{"name":"E","category":"X","links":["nope"]}`, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/services/nope", `{"name":"E","category":"X"}`, http.StatusNotFound},
		{http.MethodPut, "/api/v1/services/nope/favorite", `{"favorite":true}`, http.StatusNotFound},
		{http.MethodPost, "/api/v1/hidden/show", `{"hostId":"mini","key":"nope"}`, http.StatusNotFound},
	}
	for _, c := range cases {
		if status, _ := e.do(t, e.user, c.method, c.path, c.body, nil); status != c.want {
			t.Errorf("%s %s %s: status %d, want %d", c.method, c.path, c.body, status, c.want)
		}
	}
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/services/emby/url", `{"url":"http://x.fr"}`},
		{http.MethodPatch, "/api/v1/services/emby", `{"name":"E","category":"X"}`},
		{http.MethodPut, "/api/v1/services/emby/favorite", `{"favorite":true}`},
		{http.MethodGet, "/api/v1/hidden", ""},
	} {
		if status, _ := e.do(t, e.anon, c.method, c.path, c.body, nil); status != http.StatusUnauthorized {
			t.Errorf("%s %s without a session: status %d", c.method, c.path, status)
		}
	}
}

func TestEditFavoriteAndHiddenRoutes(t *testing.T) {
	e := discoveredEnv(t)
	var svc api.ServiceDetail
	if status, _ := e.sendJSON(t, http.MethodPut, "/api/v1/services/emby/favorite", `{"favorite":true}`, &svc); status != http.StatusOK || !svc.Favorite {
		t.Fatalf("favorite: status %d, %+v", status, svc.ServiceSummary)
	}
	if status, _ := e.sendJSON(t, http.MethodPatch, "/api/v1/services/emby", `{"name":"Cinéma","category":"Salon","icon":"jellyfin","hidden":false,"links":[]}`, &svc); status != http.StatusOK || svc.Name != "Cinéma" || *svc.Icon != "jellyfin" {
		t.Fatalf("edit: status %d, %+v", status, svc.ServiceSummary)
	}
	if status, _ := e.sendJSON(t, http.MethodPost, "/api/v1/services/emby/reset", "", &svc); status != http.StatusOK || svc.Name != "Emby" || !svc.Favorite {
		t.Fatalf("reset: status %d, %+v", status, svc.ServiceSummary)
	}
	var hidden []api.HiddenGroup
	e.getJSON(t, "/api/v1/hidden", &hidden)
	if len(hidden) != 1 || hidden[0].Key != "hawser" {
		t.Fatalf("hidden %+v", hidden)
	}
	if status, _ := e.sendJSON(t, http.MethodPost, "/api/v1/hidden/show", `{"hostId":"mini","key":"hawser"}`, &svc); status != http.StatusOK || svc.Slug == "" {
		t.Fatalf("show: status %d, %+v", status, svc.ServiceSummary)
	}
}

// The example store has no corrections to keep: the routes do not exist.
func TestWritesAbsentWithoutEditor(t *testing.T) {
	e := newEnv(t, fixture.New())
	if status, _ := e.do(t, e.user, http.MethodPut, "/api/v1/services/emby/favorite", `{"favorite":true}`, nil); status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", status)
	}
}
