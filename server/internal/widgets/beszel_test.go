package widgets_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"hub/server/internal/secrets"
	"hub/server/internal/widgets"
)

// fakeBeszel answers as PocketBase does: a token for the right password, 400 for a wrong one.
type fakeBeszel struct {
	logins atomic.Int32
	token  atomic.Value
}

func (f *fakeBeszel) server(t *testing.T) *httptest.Server {
	t.Helper()
	f.token.Store("jeton-1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/collections/users/auth-with-password":
			f.logins.Add(1)
			var body struct{ Identity, Password string }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Identity != "hub@local" || body.Password != "secret" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": f.token.Load().(string)})
		case "/api/collections/systems/records":
			if r.Header.Get("Authorization") != f.token.Load().(string) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if !strings.Contains(r.URL.Query().Get("fields"), "info") {
				t.Errorf("fields: %s", r.URL)
			}
			_, _ = w.Write([]byte(`{"page":1,"items":[
				{"name":"mini-pc","status":"up","info":{"cpu":12.4,"mp":40.6,"dp":61}},
				{"name":"qnap","status":"down","info":{"cpu":0,"mp":0,"dp":0}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestBeszelSignsInOnceAndReadsItsSystems(t *testing.T) {
	f := &fakeBeszel{}
	where := widgets.Target{Kind: "beszel", URL: f.server(t).URL}
	account := secrets.NewValue(widgets.JoinCredentials("hub@local", "secret"))

	p, state := widgets.TestKey(context.Background(), where, account)
	if state != widgets.StateOK || p.Note != "2 systèmes visibles" || strings.Join(p.Systems, ",") != "mini-pc,qnap" {
		t.Errorf("%s %+v", state, p)
	}
	data, state := widgets.Read(context.Background(), where, account, nil, []string{widgets.WidgetSystems})
	if state != widgets.StateOK {
		t.Fatal(state)
	}
	if f.logins.Load() != 1 {
		t.Errorf("signed in %d times", f.logins.Load())
	}
	var list []widgets.System
	if err := json.Unmarshal(data[widgets.WidgetSystems], &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0] != (widgets.System{Name: "mini-pc", Status: "up", CPU: 12, Memory: 41, Disk: 61}) {
		t.Errorf("%+v", list)
	}

	// An expired token signs in again, once.
	f.token.Store("jeton-2")
	if _, state := widgets.Read(context.Background(), where, account, nil, []string{widgets.WidgetSystems}); state != widgets.StateOK {
		t.Errorf("after expiry: %s", state)
	}
	if f.logins.Load() != 2 {
		t.Errorf("signed in %d times", f.logins.Load())
	}
}

func TestBeszelRefusesAWrongPassword(t *testing.T) {
	f := &fakeBeszel{}
	where := widgets.Target{Kind: "beszel", URL: f.server(t).URL}
	_, state := widgets.TestKey(context.Background(), where, secrets.NewValue(widgets.JoinCredentials("hub@local", "faux")))
	if state != widgets.StateAuth {
		t.Errorf("%s", state)
	}
}

func TestBeszelFormatChangeIsNotGuessed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "auth-with-password") {
			_, _ = w.Write([]byte(`{"token":"t"}`))
			return
		}
		_, _ = w.Write([]byte(`{"systems":[]}`))
	}))
	defer srv.Close()
	_, state := widgets.TestKey(context.Background(), widgets.Target{Kind: "beszel", URL: srv.URL}, secrets.NewValue(widgets.JoinCredentials("a", "b")))
	if state != widgets.StateUnexpected {
		t.Errorf("%s", state)
	}
}

func TestMatchSystemIgnoresCaseAndPunctuation(t *testing.T) {
	systems := []string{"Mini PC", "qnap-nas"}
	if got := widgets.MatchSystem(systems, "mini-pc", "mini-PC"); got != "Mini PC" {
		t.Errorf("%q", got)
	}
	if got := widgets.MatchSystem(systems, "raspberry-pi"); got != "" {
		t.Errorf("%q", got)
	}
}
