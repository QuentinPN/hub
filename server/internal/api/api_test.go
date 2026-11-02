package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"hub/server/internal/api"
	"hub/server/internal/auth"
	"hub/server/internal/auth/authtest"
	"hub/server/internal/domain"
	"hub/server/internal/fixture"
	"hub/server/internal/icons"
)

const password = "correct horse battery"

type env struct {
	srv    *httptest.Server
	user   *http.Client // logged in, keeps cookies
	anon   *http.Client // no cookie
	repo   *authtest.Repo
	svc    *auth.Service
	secret string // the setup code, already used by user
}

// newEnv serves store over TLS (the session cookie is Secure) and creates the account.
func newEnv(t *testing.T, store api.Store) *env {
	t.Helper()
	repo := authtest.New()
	svc, err := auth.New(repo)
	if err != nil {
		t.Fatal(err)
	}
	code, err := svc.PrepareSetup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ui := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ui")) })
	srv := httptest.NewTLSServer(api.NewHandler(store, svc, icons.NewService(noCache{}, nil), ui, []string{"sha256-abc"}))
	t.Cleanup(srv.Close)

	e := &env{srv: srv, repo: repo, svc: svc, secret: code}
	e.anon = &http.Client{Transport: srv.Client().Transport}
	e.user = e.newClient(t)
	body := `{"code":"` + code + `","username":"quentin","password":"` + password + `"}`
	if status, _ := e.do(t, e.user, http.MethodPost, "/api/v1/auth/setup", body, nil); status != http.StatusNoContent {
		t.Fatalf("setup: status %d", status)
	}
	return e
}

func (e *env) newClient(t *testing.T) *http.Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: e.srv.Client().Transport, Jar: jar}
}

func (e *env) do(t *testing.T, c *http.Client, method, path, body string, header map[string]string) (int, []byte) {
	t.Helper()
	res := e.raw(t, c, method, path, body, header)
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, data
}

func (e *env) raw(t *testing.T, c *http.Client, method, path, body string, header map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (e *env) getJSON(t *testing.T, path string, v any) {
	t.Helper()
	status, data := e.do(t, e.user, http.MethodGet, path, "", nil)
	if status != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, status)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

func TestHome(t *testing.T) {
	e := newEnv(t, fixture.New())
	var home api.Home
	e.getJSON(t, "/api/v1/home", &home)
	if home.Summary.Total == 0 || len(home.Hosts) == 0 {
		t.Fatalf("empty home: %+v", home.Summary)
	}
}

// A service on an unreachable host must read "unknown", never "online" or "down".
func TestServiceOnUnreachableHostIsUnknown(t *testing.T) {
	e := newEnv(t, fixture.New())
	var svc api.ServiceDetail
	e.getJSON(t, "/api/v1/services/pihole", &svc)
	if svc.Status != domain.StatusUnknown {
		t.Fatalf("status = %q, want unknown", svc.Status)
	}
	if svc.LatencyMs != nil {
		t.Fatalf("latency should be absent for an unknown service")
	}
}

func TestUnknownServiceIs404(t *testing.T) {
	e := newEnv(t, fixture.New())
	if status, _ := e.do(t, e.user, http.MethodGet, "/api/v1/services/nope", "", nil); status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestSecurityHeaders(t *testing.T) {
	e := newEnv(t, fixture.New())
	res := e.raw(t, e.anon, http.MethodGet, "/", "", nil)
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
	e := newEnv(t, fixture.New())
	var list []api.ServiceSummary
	e.getJSON(t, "/api/v1/services", &list)
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
	e := newEnv(t, fixture.New())
	var s api.Settings
	e.getJSON(t, "/api/v1/settings", &s)
	if s.Discovery.InferURLFromPorts || s.Editable || len(s.Hosts) != 3 {
		t.Fatalf("unexpected settings: %+v", s)
	}
}

// Every data route answers 401 without a session; only the UI and the auth entry points
// stay reachable.
func TestAPIRequiresSession(t *testing.T) {
	e := newEnv(t, &writableStore{Store: fixture.New()})
	for _, path := range []string{"/api/v1/home", "/api/v1/services", "/api/v1/services/emby", "/api/v1/settings", "/api/v1/auth/sessions", "/api/v1/nope"} {
		if status, _ := e.do(t, e.anon, http.MethodGet, path, "", nil); status != http.StatusUnauthorized {
			t.Errorf("GET %s without session: status %d", path, status)
		}
	}
	if status, _ := e.do(t, e.anon, http.MethodPut, "/api/v1/settings/discovery", `{}`, nil); status != http.StatusUnauthorized {
		t.Errorf("PUT without session: status %d", status)
	}
	if status, _ := e.do(t, e.anon, http.MethodGet, "/", "", nil); status != http.StatusOK {
		t.Errorf("UI without session: status %d", status)
	}
	// A forged cookie is no better than none.
	forged := e.newClient(t)
	req, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/api/v1/home", nil)
	req.AddCookie(&http.Cookie{Name: api.SessionCookie, Value: strings.Repeat("A", 43)})
	res, err := forged.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("forged cookie: status %d", res.StatusCode)
	}

	var state api.AuthState
	_, data := e.do(t, e.anon, http.MethodGet, "/api/v1/auth/state", "", nil)
	if err := json.Unmarshal(data, &state); err != nil || state.Authenticated || state.SetupRequired {
		t.Fatalf("anonymous state %+v, err %v", state, err)
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	e := newEnv(t, fixture.New())
	res := e.raw(t, e.anon, http.MethodPost, "/api/v1/auth/login", `{"username":"quentin","password":"`+password+`"}`, nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("login: status %d", res.StatusCode)
	}
	cookies := res.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("%d cookies", len(cookies))
	}
	c := cookies[0]
	if c.Name != "__Host-hub_session" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" {
		t.Fatalf("cookie %+v", c)
	}
}

func TestLoginLogoutAndLimits(t *testing.T) {
	e := newEnv(t, fixture.New())

	// The setup code is spent.
	again := `{"code":"` + e.secret + `","username":"autre","password":"` + password + `"}`
	if status, _ := e.do(t, e.anon, http.MethodPost, "/api/v1/auth/setup", again, nil); status != http.StatusForbidden {
		t.Fatalf("second setup: status %d", status)
	}
	// Login CSRF: another site cannot log the browser into this account.
	login := `{"username":"quentin","password":"` + password + `"}`
	if status, _ := e.do(t, e.anon, http.MethodPost, "/api/v1/auth/login", login, map[string]string{"Sec-Fetch-Site": "cross-site"}); status != http.StatusForbidden {
		t.Fatalf("cross-site login: status %d", status)
	}

	if status, _ := e.do(t, e.user, http.MethodPost, "/api/v1/auth/logout", "", nil); status != http.StatusNoContent {
		t.Fatalf("logout: status %d", status)
	}
	if status, _ := e.do(t, e.user, http.MethodGet, "/api/v1/home", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("after logout: status %d", status)
	}

	wrong := `{"username":"quentin","password":"wrong password!"}`
	for i := 0; i < 5; i++ {
		if status, _ := e.do(t, e.user, http.MethodPost, "/api/v1/auth/login", wrong, nil); status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d", i, status)
		}
	}
	res := e.raw(t, e.user, http.MethodPost, "/api/v1/auth/login", login, nil)
	res.Body.Close()
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") == "" {
		t.Fatalf("locked out: status %d, Retry-After %q", res.StatusCode, res.Header.Get("Retry-After"))
	}
}

func TestSessionsListAndRevoke(t *testing.T) {
	e := newEnv(t, fixture.New())
	other := e.newClient(t)
	login := `{"username":"quentin","password":"` + password + `"}`
	if status, _ := e.do(t, other, http.MethodPost, "/api/v1/auth/login", login, nil); status != http.StatusNoContent {
		t.Fatalf("login: status %d", status)
	}

	var list []api.SessionInfo
	e.getJSON(t, "/api/v1/auth/sessions", &list)
	if len(list) != 2 {
		t.Fatalf("%d sessions", len(list))
	}
	i := slices.IndexFunc(list, func(s api.SessionInfo) bool { return !s.Current })
	if i < 0 || !slices.ContainsFunc(list, func(s api.SessionInfo) bool { return s.Current }) {
		t.Fatalf("current flags wrong: %+v", list)
	}

	path := "/api/v1/auth/sessions/" + jsonNumber(list[i].ID)
	if status, _ := e.do(t, e.user, http.MethodDelete, path, "", nil); status != http.StatusNoContent {
		t.Fatalf("revoke: status %d", status)
	}
	if status, _ := e.do(t, other, http.MethodGet, "/api/v1/home", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("revoked session still works: status %d", status)
	}
	if status, _ := e.do(t, e.user, http.MethodDelete, "/api/v1/auth/sessions/999", "", nil); status != http.StatusNotFound {
		t.Fatalf("unknown session: status %d", status)
	}
}

func jsonNumber(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// writableStore records the settings it is given, standing in for PostgreSQL.
type writableStore struct {
	*fixture.Store
	saved *api.Discovery
}

func (s *writableStore) UpdateDiscovery(_ context.Context, d api.Discovery) error {
	s.saved = &d
	return nil
}

func TestUpdateDiscovery(t *testing.T) {
	store := &writableStore{Store: fixture.New()}
	e := newEnv(t, store)
	valid := `{"followEvents":true,"rescanMinutes":15,"onRemoval":"absent_only","archiveAfterDays":7,"confirmBelowPercent":90,"inferUrlFromPorts":false}`
	put := func(body string, header map[string]string) int {
		status, _ := e.do(t, e.user, http.MethodPut, "/api/v1/settings/discovery", body, header)
		return status
	}

	cases := []struct {
		name   string
		body   string
		header map[string]string
		want   int
	}{
		{"cross-site", valid, map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"foreign origin", valid, map[string]string{"Origin": "http://evil.example"}, http.StatusForbidden},
		{"wrong content type", valid, map[string]string{"Content-Type": "text/plain"}, http.StatusUnsupportedMediaType},
		{"unknown field", `{"rescanMinutes":5,"admin":true}`, nil, http.StatusBadRequest},
		{"infer URL from ports", strings.Replace(valid, `"inferUrlFromPorts":false`, `"inferUrlFromPorts":true`, 1), nil, http.StatusBadRequest},
		{"out of range", strings.Replace(valid, `"rescanMinutes":15`, `"rescanMinutes":0`, 1), nil, http.StatusBadRequest},
	}
	for _, c := range cases {
		if got := put(c.body, c.header); got != c.want {
			t.Errorf("%s: status %d, want %d", c.name, got, c.want)
		}
	}
	if store.saved != nil {
		t.Fatal("a rejected request reached the store")
	}

	if got := put(valid, map[string]string{"Sec-Fetch-Site": "same-origin"}); got != http.StatusOK {
		t.Fatalf("valid request: status %d", got)
	}
	if store.saved == nil || store.saved.RescanMinutes != 15 {
		t.Fatalf("saved = %+v", store.saved)
	}
	if !slices.Contains(e.repo.Kinds(), "settings_updated") {
		t.Fatalf("no audit event: %v", e.repo.Kinds())
	}
}

func TestReadOnlyStoreHasNoWriteRoute(t *testing.T) {
	e := newEnv(t, fixture.New())
	if status, _ := e.do(t, e.user, http.MethodPut, "/api/v1/settings/discovery", `{}`, nil); status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", status)
	}
}

// noCache is an empty icon cache: the API tests only see the bundled icons.
type noCache struct{}

func (noCache) CachedIcon(context.Context, string) (icons.CachedIcon, error) {
	return icons.CachedIcon{}, icons.ErrNotFound
}
func (noCache) PutCachedIcon(context.Context, icons.CachedIcon) error { return nil }

func TestIcons(t *testing.T) {
	e := newEnv(t, fixture.New())

	if status, _ := e.do(t, e.anon, http.MethodGet, "/api/v1/icons/emby", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("anonymous: status %d", status)
	}
	res := e.raw(t, e.user, http.MethodGet, "/api/v1/icons/emby", "", nil)
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/svg+xml" || !strings.Contains(string(body), "<svg") {
		t.Fatalf("emby: %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") || !strings.Contains(csp, "default-src 'none'") {
		t.Fatalf("icon CSP: %q", csp)
	}
	for _, ref := range []string{"unknown-service", "..%2Fdb", "EMBY"} {
		if status, _ := e.do(t, e.user, http.MethodGet, "/api/v1/icons/"+ref, "", nil); status != http.StatusNotFound {
			t.Errorf("%s: status %d", ref, status)
		}
	}
}

func TestChangePassword(t *testing.T) {
	e := newEnv(t, fixture.New())
	other := e.newClient(t)
	login := `{"username":"quentin","password":"` + password + `"}`
	if status, _ := e.do(t, other, http.MethodPost, "/api/v1/auth/login", login, nil); status != http.StatusNoContent {
		t.Fatalf("login: status %d", status)
	}
	const next = "a brand new passphrase"
	change := func(c *http.Client, current, newPassword string, header map[string]string) int {
		status, _ := e.do(t, c, http.MethodPut, "/api/v1/auth/password", `{"current":"`+current+`","new":"`+newPassword+`"}`, header)
		return status
	}

	if got := change(e.anon, password, next, nil); got != http.StatusUnauthorized {
		t.Fatalf("without a session: status %d", got)
	}
	if got := change(e.user, password, next, map[string]string{"Sec-Fetch-Site": "cross-site"}); got != http.StatusForbidden {
		t.Fatalf("cross-site: status %d", got)
	}
	// 403, not 401: the UI must not take a wrong password for an expired session.
	if got := change(e.user, "wrong password!", next, nil); got != http.StatusForbidden {
		t.Fatalf("wrong current password: status %d", got)
	}
	if got := change(e.user, password, "short", nil); got != http.StatusBadRequest {
		t.Fatalf("short new password: status %d", got)
	}
	if got := change(e.user, password, next, nil); got != http.StatusNoContent {
		t.Fatalf("change: status %d", got)
	}

	if status, _ := e.do(t, e.user, http.MethodGet, "/api/v1/home", "", nil); status != http.StatusOK {
		t.Fatalf("current session after the change: status %d", status)
	}
	if status, _ := e.do(t, other, http.MethodGet, "/api/v1/home", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("other session after the change: status %d", status)
	}
	if status, _ := e.do(t, e.newClient(t), http.MethodPost, "/api/v1/auth/login", `{"username":"quentin","password":"`+next+`"}`, nil); status != http.StatusNoContent {
		t.Fatalf("login with the new password: status %d", status)
	}
}
