package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hub/server/internal/api"
	"hub/server/internal/urlcheck"
)

func TestPosterRouteNeedsASessionAndAConnection(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n0000")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Header.Get("X-Emby-Token") != embyKey:
			w.WriteHeader(http.StatusUnauthorized)
		case strings.HasSuffix(r.URL.Path, "/Images/Primary"):
			_, _ = w.Write(png)
		case r.URL.Path == "/emby/Sessions":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/emby/Users/Query":
			_, _ = w.Write([]byte(`{"Items":[]}`))
		case r.URL.Path == "/emby/Items/Counts":
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	e, store, _ := widgetEnv(t, srv.URL)

	if status, _ := e.do(t, e.user, http.MethodGet, "/api/v1/posters/emby/abc", "", nil); status != http.StatusNotFound {
		t.Errorf("without a connection: %d", status)
	}
	var saved api.WidgetSaved
	e.sendJSON(t, http.MethodPut, "/api/v1/services/emby/connection", saveBody, &saved)
	if !saved.Saved {
		t.Fatalf("%+v", saved)
	}

	if status, _ := e.do(t, e.anon, http.MethodGet, "/api/v1/posters/emby/abc", "", nil); status != http.StatusUnauthorized {
		t.Errorf("without a session: %d", status)
	}
	res := e.raw(t, e.user, http.MethodGet, "/api/v1/posters/emby/abc", "", nil)
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || string(body) != string(png) {
		t.Fatalf("%d %q", res.StatusCode, body)
	}
	if res.Header.Get("Content-Type") != "image/png" || !strings.Contains(res.Header.Get("Cache-Control"), "private") {
		t.Errorf("%v", res.Header)
	}
	if status, _ := e.do(t, e.user, http.MethodGet, "/api/v1/posters/emby/a..b", "", nil); status != http.StatusNotFound {
		t.Errorf("bad reference: %d", status)
	}

	// A changed address could be anyone's: no key goes there, for a poster either.
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("request reached the new address: %s", r.URL)
	}))
	defer other.Close()
	if err := store.SaveServiceURL(context.Background(), "emby", other.URL, "", urlcheck.Result{Kind: urlcheck.KindOK, Status: 200}); err != nil {
		t.Fatal(err)
	}
	if status, _ := e.do(t, e.user, http.MethodGet, "/api/v1/posters/emby/abc", "", nil); status != http.StatusNotFound {
		t.Errorf("after an address change: %d", status)
	}
}
