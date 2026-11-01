package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"hub/server/internal/api"
	"hub/server/internal/auth/authtest"
	"hub/server/internal/fixture"
)

const passkeyOrigin = "https://hub.test"

func TestPasskeyRoutesWhenDisabled(t *testing.T) {
	e := newEnv(t, fixture.New())
	var state api.AuthState
	e.getJSON(t, "/api/v1/auth/state", &state)
	if state.Passkeys {
		t.Fatal("passkeys announced without EnablePasskeys")
	}
	if status, _ := e.do(t, e.anon, http.MethodPost, "/api/v1/auth/passkey/login/begin", "", nil); status != http.StatusNotFound {
		t.Fatalf("login begin: status %d", status)
	}
	if status, _ := e.do(t, e.user, http.MethodGet, "/api/v1/auth/passkeys", "", nil); status != http.StatusNotFound {
		t.Fatalf("list: status %d", status)
	}
}

func TestPasskeyRoutes(t *testing.T) {
	e := newEnv(t, fixture.New())
	if err := e.svc.EnablePasskeys("hub.test", []string{passkeyOrigin}); err != nil {
		t.Fatal(err)
	}
	var state api.AuthState
	e.getJSON(t, "/api/v1/auth/state", &state)
	if !state.Passkeys {
		t.Fatal("passkeys not announced")
	}

	// Management needs a session; login is public.
	for _, path := range []string{"/api/v1/auth/passkeys/begin", "/api/v1/auth/passkeys/finish"} {
		if status, _ := e.do(t, e.anon, http.MethodPost, path, `{}`, nil); status != http.StatusUnauthorized {
			t.Errorf("POST %s without session: status %d", path, status)
		}
	}
	if status, _ := e.do(t, e.anon, http.MethodGet, "/api/v1/auth/passkeys", "", nil); status != http.StatusUnauthorized {
		t.Errorf("list without session: status %d", status)
	}

	// A wrong password is 403, so the UI does not take it for an expired session.
	if status, _ := e.do(t, e.user, http.MethodPost, "/api/v1/auth/passkeys/begin", `{"password":"wrong password!"}`, nil); status != http.StatusForbidden {
		t.Fatalf("wrong password: status %d", status)
	}
	status, options := e.do(t, e.user, http.MethodPost, "/api/v1/auth/passkeys/begin", `{"password":"`+password+`"}`, nil)
	if status != http.StatusOK {
		t.Fatalf("begin: status %d", status)
	}
	key := authtest.NewSoftKey(t, passkeyOrigin)
	finish := func(name string, credential []byte) (int, []byte) {
		body, _ := json.Marshal(map[string]any{"name": name, "credential": json.RawMessage(credential)})
		return e.do(t, e.user, http.MethodPost, "/api/v1/auth/passkeys/finish", string(body), nil)
	}
	status, data := finish("Windows Hello", key.Create(t, options))
	if status != http.StatusCreated {
		t.Fatalf("finish: status %d %s", status, data)
	}
	var created api.PasskeyInfo
	if err := json.Unmarshal(data, &created); err != nil || created.Name != "Windows Hello" || !created.BackedUp {
		t.Fatalf("created %+v, err %v", created, err)
	}
	// The prompt is single use.
	if status, _ := finish("encore", key.Create(t, options)); status != http.StatusBadRequest {
		t.Fatalf("replayed finish: status %d", status)
	}

	// Sign in on a new browser with the passkey alone.
	browser := e.newClient(t)
	status, options = e.do(t, browser, http.MethodPost, "/api/v1/auth/passkey/login/begin", "", nil)
	if status != http.StatusOK {
		t.Fatalf("login begin: status %d", status)
	}
	if status, _ := e.do(t, browser, http.MethodPost, "/api/v1/auth/passkey/login/finish", string(key.Get(t, options)), nil); status != http.StatusNoContent {
		t.Fatalf("login finish: status %d", status)
	}
	if status, _ := e.do(t, browser, http.MethodGet, "/api/v1/home", "", nil); status != http.StatusOK {
		t.Fatalf("after passkey login: status %d", status)
	}
	// A replayed answer opens nothing.
	if status, _ := e.do(t, e.anon, http.MethodPost, "/api/v1/auth/passkey/login/finish", string(key.Get(t, options)), nil); status != http.StatusUnauthorized {
		t.Fatalf("replayed login: status %d", status)
	}

	var list []api.PasskeyInfo
	e.getJSON(t, "/api/v1/auth/passkeys", &list)
	if len(list) != 1 || list[0].ID != created.ID || list[0].LastUsedAt == nil {
		t.Fatalf("list %+v", list)
	}
	path := "/api/v1/auth/passkeys/" + jsonNumber(created.ID)
	if status, _ := e.do(t, e.user, http.MethodDelete, path, "", nil); status != http.StatusNoContent {
		t.Fatalf("delete: status %d", status)
	}
	if status, _ := e.do(t, e.user, http.MethodDelete, path, "", nil); status != http.StatusNotFound {
		t.Fatalf("second delete: status %d", status)
	}
}
