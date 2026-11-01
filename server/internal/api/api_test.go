package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hub/server/internal/api"
	"hub/server/internal/domain"
	"hub/server/internal/fixture"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	ui := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ui")) })
	srv := httptest.NewServer(api.NewHandler(fixture.New(), ui, []string{"sha256-abc"}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHome(t *testing.T) {
	srv := newServer(t)
	res, err := http.Get(srv.URL + "/api/v1/home")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var home api.Home
	if err := json.NewDecoder(res.Body).Decode(&home); err != nil {
		t.Fatal(err)
	}
	if home.Summary.Total == 0 || len(home.Hosts) == 0 {
		t.Fatalf("empty home: %+v", home.Summary)
	}
}

// A service on an unreachable host must read "unknown", never "online" or "down".
func TestServiceOnUnreachableHostIsUnknown(t *testing.T) {
	srv := newServer(t)
	res, err := http.Get(srv.URL + "/api/v1/services/pihole")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var svc api.ServiceDetail
	if err := json.NewDecoder(res.Body).Decode(&svc); err != nil {
		t.Fatal(err)
	}
	if svc.Status != domain.StatusUnknown {
		t.Fatalf("status = %q, want unknown", svc.Status)
	}
	if svc.LatencyMs != nil {
		t.Fatalf("latency should be absent for an unknown service")
	}
}

func TestUnknownServiceIs404(t *testing.T) {
	srv := newServer(t)
	res, err := http.Get(srv.URL + "/api/v1/services/nope")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv := newServer(t)
	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' 'sha256-abc'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("unexpected CSP: %s", csp)
	}
	if res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}
}

func TestServicesListsWholeInventory(t *testing.T) {
	srv := newServer(t)
	res, err := http.Get(srv.URL + "/api/v1/services")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var list []api.ServiceSummary
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	counts := map[domain.Status]int{}
	for _, s := range list {
		counts[s.Status]++
		if s.HostID == "rpi5" && s.Status != domain.StatusUnknown {
			t.Errorf("%s on unreachable rpi5 has status %q", s.Name, s.Status)
		}
	}
	if len(list) != 34 || counts[domain.StatusConfigRequired] != 1 || counts[domain.StatusUnknown] != 3 {
		t.Fatalf("got %d services, counts %v", len(list), counts)
	}
}

func TestSettingsNeverInferURLFromPorts(t *testing.T) {
	srv := newServer(t)
	res, err := http.Get(srv.URL + "/api/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var s api.Settings
	if err := json.NewDecoder(res.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	if s.Discovery.InferURLFromPorts || s.Editable || len(s.Hosts) != 3 {
		t.Fatalf("unexpected settings: %+v", s)
	}
}
