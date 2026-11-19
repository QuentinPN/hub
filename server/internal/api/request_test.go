package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"hub/server/internal/api"
	"hub/server/internal/fixture"
	"hub/server/internal/secrets"
	"hub/server/internal/widgets"
)

const hubKey = "ur_une-cle-de-test-request-0123"

// memKeys keeps the secrets in memory, unsealed: these tests are about the routes.
type memKeys struct {
	mu   sync.Mutex
	vals map[string]string
}

func (m *memKeys) Get(_ context.Context, name string) (secrets.Value, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vals[name]
	if !ok {
		return secrets.Value{}, secrets.ErrNotFound
	}
	return secrets.NewValue(v), nil
}

func (m *memKeys) Put(_ context.Context, name string, v secrets.Value) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vals[name] = v.Reveal()
	return nil
}

func (m *memKeys) Delete(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.vals[name]; !ok {
		return secrets.ErrNotFound
	}
	delete(m.vals, name)
	return nil
}

func (m *memKeys) has(name string) bool {
	_, err := m.Get(context.Background(), name)
	return err == nil
}

// connStore is the fixture with two connections: Request at hub, and Emby, which has no
// webhook, at emby.
type connStore struct {
	*fixture.Store
	keys    *memKeys
	targets map[string]api.WidgetTarget
	// saved is what the last save wrote.
	saved api.NewConnection
}

func (s *connStore) WidgetRows(context.Context) ([]api.WidgetRow, error) { return nil, nil }

func (s *connStore) WidgetTarget(_ context.Context, slug string) (api.WidgetTarget, error) {
	t, ok := s.targets[slug]
	if !ok {
		return api.WidgetTarget{}, api.ErrNotFound
	}
	return t, nil
}

func (s *connStore) SaveWidgetConnection(_ context.Context, _ string, c api.NewConnection) error {
	s.saved = c
	return nil
}
func (s *connStore) DeleteWidgetConnection(context.Context, string) error { return nil }

func (s *connStore) Service(_ context.Context, slug string) (api.ServiceDetail, error) {
	t, ok := s.targets[slug]
	if !ok {
		return api.ServiceDetail{}, api.ErrNotFound
	}
	conn := &api.WidgetConnection{State: "ok", KeyHint: "0123"}
	if t.Kind == "request" {
		conn.Webhook = &api.WebhookState{Configured: s.keys.has(widgets.WebhookName(slug))}
	}
	return api.ServiceDetail{ServiceSummary: api.ServiceSummary{Slug: slug},
		Widget: &api.WidgetInfo{Support: widgets.SupportReady, Connection: conn}}, nil
}

// fakeRequestAPI answers the actions like Request; the request id picks the answer.
// reached records what got to the fake service.
type reached struct {
	mu    sync.Mutex
	calls []string
}

func (r *reached) add(call string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

func (r *reached) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	got := r.calls
	r.calls = nil
	return got
}

func fakeRequestAPI(t *testing.T, seen *reached) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != hubKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		seen.add(r.Method + " " + r.URL.Path + " " + string(body))
		refuse := func(status int, msg string) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
		}
		switch r.URL.Path {
		case "/api/auth/me":
			_, _ = w.Write([]byte(`{"viewer":{"id":"u1"},"apiKeyScope":"read"}`))
		case "/api/requests/counts":
			_, _ = w.Write([]byte(`{"pendingApprovals":1,"mineActive":0}`))
		case "/api/issues/counts":
			_, _ = w.Write([]byte(`{"open":0}`))
		case "/api/requests/403/approve":
			refuse(http.StatusForbidden, "Droit requis")
		case "/api/requests/409/approve":
			refuse(http.StatusConflict, "Demande déjà traitée")
		case "/api/requests/401/approve":
			refuse(http.StatusUnauthorized, "Clé inconnue")
		case "/api/requests/500/approve":
			refuse(http.StatusInternalServerError, "boom")
		case "/api/requests/12/approve", "/api/requests/12/decline":
			_, _ = w.Write([]byte(`{"id":12}`))
		case "/api/requests":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":20}`))
		case "/api/search":
			_, _ = w.Write([]byte(`{"page":1,"totalPages":1,"people":[],"results":[{"tmdbId":603,"type":"movie","title":"Matrix","year":1999,"posterPath":"/abc.jpg","status":"pending"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type wakes struct {
	mu    sync.Mutex
	slugs []string
}

func (w *wakes) wake(slug string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.slugs = append(w.slugs, slug)
}

func (w *wakes) take() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	got := w.slugs
	w.slugs = nil
	return got
}

func actionsEnv(t *testing.T, origin string) (*env, *memKeys, *wakes, *reached) {
	t.Helper()
	e, _, keys, woken, seen := actionsStore(t, origin)
	return e, keys, woken, seen
}

func actionsStore(t *testing.T, origin string) (*env, *connStore, *memKeys, *wakes, *reached) {
	t.Helper()
	seen := &reached{}
	address := fakeRequestAPI(t, seen).URL
	keys := &memKeys{vals: map[string]string{widgets.KeyName("hub"): hubKey, widgets.KeyName("emby"): "une-cle-emby"}}
	store := &connStore{Store: fixture.New(), keys: keys, targets: map[string]api.WidgetTarget{
		"hub": {Support: widgets.SupportReady, Kind: "request", URL: address, ConnectedURL: address},
		"emby": {Support: widgets.SupportReady, Kind: "emby", URL: address, ConnectedURL: address},
		// Made with Seerr's connector, which Hub no longer has.
		"seerr": {Support: widgets.SupportNone, Kind: "seerr", URL: address, ConnectedURL: address},
		// Saved, then moved: the key stays home.
		"moved": {Support: widgets.SupportReady, Kind: "request", URL: "http://ailleurs.invalid", ConnectedURL: address},
	}}
	keys.vals[widgets.KeyName("moved")] = hubKey
	keys.vals[widgets.KeyName("seerr")] = "une-cle-seerr"
	woken := &wakes{}
	e := newFullEnv(t, store, nil, &api.Widgets{Keys: keys, Bus: widgets.NewBus(), Wake: woken.wake, Origin: origin})
	return e, store, keys, woken, seen
}

// The scope of a key is read when it is saved, and kept with the connection.
func TestSavingARequestConnectionKeepsTheKeyScope(t *testing.T) {
	e, store, _, _, _ := actionsStore(t, "")
	body := `{"apiKey":"` + hubKey + `","widgets":["pending"],"refreshSeconds":60}`
	if status, data := e.do(t, e.user, http.MethodPut, "/api/v1/services/hub/connection", body, nil); status != http.StatusOK {
		t.Fatalf("%d %s", status, data)
	}
	if c := store.saved.CanAct; c == nil || *c {
		t.Errorf("saved canAct: %v", c)
	}
}

func TestRequestActionsReachRequestAndWakeTheConnection(t *testing.T) {
	e, _, woken, seen := actionsEnv(t, "")
	post := func(path, body string) (int, string) {
		status, data := e.do(t, e.user, http.MethodPost, path, body, nil)
		return status, string(data)
	}

	if status, _ := post("/api/v1/services/hub/requests/12/approve", ""); status != http.StatusNoContent {
		t.Fatalf("approve: %d", status)
	}
	if status, _ := post("/api/v1/services/hub/requests/12/decline", `{"reason":"  Déjà là  "}`); status != http.StatusNoContent {
		t.Fatalf("decline: %d", status)
	}
	if status, _ := post("/api/v1/services/hub/requests", `{"mediaType":"tv","tmdbId":99}`); status != http.StatusCreated {
		t.Fatalf("request: %d", status)
	}
	if got := woken.take(); len(got) != 3 || got[0] != "hub" {
		t.Errorf("wakes: %v", got)
	}
	want := []string{
		"POST /api/requests/12/approve {}",
		`POST /api/requests/12/decline {"reason":"Déjà là"}`,
		`POST /api/requests {"seasons":"all","tmdbId":99,"type":"tv"}`,
	}
	if got := seen.take(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("reached:\n%s", strings.Join(got, "\n"))
	}

	var found struct{ Items []widgets.Found }
	if status, _ := e.sendJSON(t, http.MethodGet, "/api/v1/services/hub/search?q=%20matrix%20", "", &found); status != http.StatusOK ||
		len(found.Items) != 1 || found.Items[0].Poster != "abc.jpg" || *found.Items[0].Status != "pending" {
		t.Errorf("search: %d %+v", status, found)
	}
	if got := woken.take(); len(got) != 0 {
		t.Errorf("a search woke the connection: %v", got)
	}

	// Refusals: the service's words, never a 401 that would sign the user out.
	for _, c := range []struct {
		path   string
		status int
		says   string
	}{
		{"/api/v1/services/hub/requests/403/approve", http.StatusForbidden, "La clé Request n'a pas le droit de faire cette action."},
		{"/api/v1/services/hub/requests/409/approve", http.StatusConflict, "Demande déjà traitée"},
		{"/api/v1/services/hub/requests/401/approve", http.StatusBadGateway, "reconnecte le service"},
		{"/api/v1/services/hub/requests/500/approve", http.StatusBadGateway, "ne répond pas"},
	} {
		status, data := post(c.path, "")
		if status != c.status || !strings.Contains(data, c.says) || strings.Contains(data, "boom") {
			t.Errorf("%s: %d %s", c.path, status, data)
		}
	}
	if got := woken.take(); len(got) != 0 {
		t.Errorf("a refused write woke the connection: %v", got)
	}

	// Checked here, before anything reaches the service.
	seen.take()
	for _, c := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPost, "/api/v1/services/hub/requests/abc/approve", "", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/services/hub/requests/0/approve", "", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/services/hub/requests/99999999999/approve", "", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/services/hub/requests/12/decline", `{"reason":"   "}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/services/hub/requests/12/decline", `{"reason":"` + strings.Repeat("é", 501) + `"}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/services/hub/requests/12/decline", `{"reason":"x","extra":1}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/services/hub/requests", `{"mediaType":"person","tmdbId":1}`, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/services/hub/requests", `{"mediaType":"movie","tmdbId":0}`, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/services/hub/search?q=m", "", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/services/hub/search?q=" + strings.Repeat("m", 101), "", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/services/emby/requests/12/approve", "", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/services/moved/requests/12/approve", "", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/services/nope/requests/12/approve", "", http.StatusBadRequest},
	} {
		if status, data := e.do(t, e.user, c.method, c.path, c.body, nil); status != c.status {
			t.Errorf("%s %s: %d %s", c.method, c.path, status, data)
		}
	}
	if status, _ := e.do(t, e.anon, http.MethodPost, "/api/v1/services/hub/requests/12/approve", "", nil); status != http.StatusUnauthorized {
		t.Errorf("without a session: %d", status)
	}
	if status, _ := e.do(t, e.user, http.MethodPost, "/api/v1/services/hub/requests/12/approve", "", map[string]string{"Origin": "https://evil.example"}); status != http.StatusForbidden {
		t.Errorf("cross-origin: %d", status)
	}
	if got := seen.take(); len(got) != 0 {
		t.Errorf("a refused request reached Request: %v", got)
	}
}

func TestWebhookWakesOnlyWithItsSecret(t *testing.T) {
	e, keys, woken, _ := actionsEnv(t, "https://hub.example/")
	hook := func(slug, auth, contentType, body string, extra map[string]string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/api/v1/hooks/"+slug, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		for k, v := range extra {
			req.Header.Set(k, v)
		}
		res, err := e.anon.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		data, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(data)
	}
	const payload = `{"type":"approved","title":"Demande approuvée","message":"Matrix","media":null,"request":{"id":12,"status":"approved"},"user":null}`

	// Before any secret, nothing gets in.
	refusedStatus, refused := hook("hub", "anything", "application/json", payload, nil)
	if refusedStatus != http.StatusUnauthorized {
		t.Fatalf("no secret yet: %d", refusedStatus)
	}

	var made api.WebhookSecret
	if status, _ := e.sendJSON(t, http.MethodPost, "/api/v1/services/hub/connection/webhook-secret", "", &made); status != http.StatusOK ||
		made.URL != "https://hub.example/api/v1/hooks/hub" || len(made.Secret) != 43 {
		t.Fatalf("%d %+v", status, made)
	}
	if !keys.has(widgets.WebhookName("hub")) {
		t.Fatal("secret not stored")
	}

	// The connection only says a secret exists.
	status, data := e.do(t, e.user, http.MethodGet, "/api/v1/services/hub/connection", "", nil)
	if status != http.StatusOK || !strings.Contains(string(data), `"webhook":{"configured":true}`) || strings.Contains(string(data), made.Secret) {
		t.Errorf("GET connection: %d %s", status, data)
	}

	// A server sends it: no session, no Origin, any JSON content type, even a foreign site's headers.
	if status, body := hook("hub", made.Secret, "text/plain", payload, map[string]string{"Origin": "https://request.lan", "Sec-Fetch-Site": "cross-site"}); status != http.StatusNoContent {
		t.Fatalf("good secret: %d %s", status, body)
	}
	if got := woken.take(); len(got) != 1 || got[0] != "hub" {
		t.Errorf("wakes: %v", got)
	}

	for name, c := range map[string]struct{ slug, auth string }{
		"wrong secret":     {"hub", made.Secret + "x"},
		"no secret":        {"hub", ""},
		"bearer prefix":    {"hub", "Bearer " + made.Secret},
		"unknown slug":     {"nope", made.Secret},
		"another's secret": {"emby", made.Secret},
	} {
		status, body := hook(c.slug, c.auth, "application/json", payload, nil)
		if status != refusedStatus || body != refused {
			t.Errorf("%s: %d %s, want the same refusal as always", name, status, body)
		}
	}
	// A connector without a webhook refuses alike, even with a secret planted under its name.
	if err := keys.Put(context.Background(), widgets.WebhookName("emby"), secrets.NewValue("planted")); err != nil {
		t.Fatal(err)
	}
	if status, body := hook("emby", "planted", "application/json", payload, nil); status != refusedStatus || body != refused {
		t.Errorf("no webhook for emby: %d %s", status, body)
	}
	if status, _ := e.do(t, e.user, http.MethodPost, "/api/v1/services/emby/connection/webhook-secret", "", nil); status != http.StatusBadRequest {
		t.Errorf("emby webhook secret: %d", status)
	}

	if status, _ := hook("hub", made.Secret, "application/json", `{"pad":"`+strings.Repeat("a", 64<<10)+`"}`, nil); status != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized: %d", status)
	}
	if status, _ := hook("hub", made.Secret, "application/json", "pas du JSON", nil); status != http.StatusBadRequest {
		t.Errorf("not JSON: %d", status)
	}
	if got := woken.take(); len(got) != 0 {
		t.Errorf("a refused webhook woke the connection: %v", got)
	}
	if status, _ := hook("hub/extra", made.Secret, "application/json", payload, nil); status != http.StatusUnauthorized {
		t.Errorf("deeper path: %d", status)
	}

	// Refusals pile up per IP: past the limit even the right secret waits, then passes again.
	now := time.Now()
	e.svc.SetClock(func() time.Time { return now })
	for i := 0; i < 25; i++ {
		hook("hub", "wrong", "application/json", payload, nil)
	}
	if status, _ := hook("hub", made.Secret, "application/json", payload, nil); status != http.StatusTooManyRequests {
		t.Errorf("burst of refusals: %d, want 429", status)
	}
	now = now.Add(2 * time.Minute)
	if status, _ := hook("hub", made.Secret, "application/json", payload, nil); status != http.StatusNoContent {
		t.Errorf("after the window: %d", status)
	}

	// Rotating replaces it at once.
	var again api.WebhookSecret
	e.sendJSON(t, http.MethodPost, "/api/v1/services/hub/connection/webhook-secret", "", &again)
	if again.Secret == made.Secret {
		t.Fatal("same secret twice")
	}
	if status, _ := hook("hub", made.Secret, "application/json", payload, nil); status != http.StatusUnauthorized {
		t.Errorf("old secret after rotation: %d", status)
	}

	// Removing the connection erases both secrets.
	if status, _ := e.do(t, e.user, http.MethodDelete, "/api/v1/services/hub/connection", "", nil); status != http.StatusOK {
		t.Fatalf("delete: %d", status)
	}
	if keys.has(widgets.WebhookName("hub")) || keys.has(widgets.KeyName("hub")) {
		t.Error("a secret outlived its connection")
	}
	if status, _ := hook("hub", again.Secret, "application/json", payload, nil); status != http.StatusUnauthorized {
		t.Errorf("after removal: %d", status)
	}
}

// Without HUB_ORIGIN, the address is the one the browser used.
func TestWebhookURLFollowsTheRequestWithoutAnOrigin(t *testing.T) {
	e, _, _, _ := actionsEnv(t, "")
	var made api.WebhookSecret
	e.sendJSON(t, http.MethodPost, "/api/v1/services/hub/connection/webhook-secret", "", &made)
	if made.URL != e.srv.URL+"/api/v1/hooks/hub" {
		t.Errorf("%s", made.URL)
	}
	raw, _ := json.Marshal(made)
	if !strings.Contains(string(raw), `"url"`) || !strings.Contains(string(raw), `"secret"`) {
		t.Errorf("%s", raw)
	}
}

// A connection made with a connector Hub no longer has can do nothing but be removed.
func TestConnectionOfAConnectorNoLongerInHubCanOnlyBeRemoved(t *testing.T) {
	e, keys, woken, seen := actionsEnv(t, "")
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/services/seerr/requests/12/approve", ""},
		{http.MethodGet, "/api/v1/services/seerr/search?q=matrix", ""},
		{http.MethodPost, "/api/v1/services/seerr/connection/webhook-secret", ""},
		{http.MethodPost, "/api/v1/services/seerr/connection/test", `{"apiKey":""}`},
		{http.MethodPut, "/api/v1/services/seerr/connection", `{"apiKey":"","widgets":["requests"],"refreshSeconds":60,"options":{}}`},
	} {
		if status, data := e.do(t, e.user, c.method, c.path, c.body, nil); status != http.StatusBadRequest {
			t.Errorf("%s %s: %d %s", c.method, c.path, status, data)
		}
	}
	if status, _ := e.do(t, e.user, http.MethodGet, "/api/v1/posters/seerr/abc.jpg", "", nil); status != http.StatusNotFound {
		t.Errorf("poster: %d", status)
	}
	if got := seen.take(); len(got) != 0 {
		t.Errorf("Seerr's key went out: %v", got)
	}
	if got := woken.take(); len(got) != 0 {
		t.Errorf("woke: %v", got)
	}
	if status, _ := e.do(t, e.user, http.MethodDelete, "/api/v1/services/seerr/connection", "", nil); status != http.StatusOK {
		t.Fatalf("delete: %d", status)
	}
	if keys.has(widgets.KeyName("seerr")) {
		t.Error("the key outlived its connection")
	}
}
