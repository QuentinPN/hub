package widgets_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hub/server/internal/secrets"
	"hub/server/internal/widgets"
)

func fakeJSON(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != testKey {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		body, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestProwlarrCountsFailingEnabledIndexers(t *testing.T) {
	later := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	srv := fakeJSON(t, map[string]string{
		"/api/v1/indexer": `[{"id":1,"name":"nom-prive","enable":true},{"id":2,"enable":true},{"id":3,"enable":true},{"id":4,"enable":false}]`,
		"/api/v1/indexerstatus": fmt.Sprintf(`[{"indexerId":1,"disabledTill":%q},{"indexerId":2,"disabledTill":%q},{"indexerId":4,"disabledTill":%q}]`,
			later, past, later),
	})
	where := widgets.Target{Kind: "prowlarr", URL: srv.URL}
	p, state := widgets.TestKey(context.Background(), where, secrets.NewValue(testKey))
	if state != widgets.StateOK || p.Note != "3 indexeurs" {
		t.Errorf("%s %+v", state, p)
	}
	data, state := widgets.Read(context.Background(), where, secrets.NewValue(testKey), nil, []string{widgets.WidgetIndexers})
	if state != widgets.StateOK {
		t.Fatal(state)
	}
	var got widgets.Indexers
	if err := json.Unmarshal(data[widgets.WidgetIndexers], &got); err != nil {
		t.Fatal(err)
	}
	if got != (widgets.Indexers{Enabled: 3, Failing: 1}) {
		t.Errorf("%+v", got)
	}
	cards, err := widgets.CardsOf("prowlarr", data, []string{widgets.WidgetIndexers})
	if err != nil || len(cards) != 1 || cards[0].Note != "1 en panne" {
		t.Errorf("%+v %v", cards, err)
	}
	if strings.Contains(string(data[widgets.WidgetIndexers]), "nom-prive") {
		t.Error("an indexer name reached the stored data")
	}
}

func TestCardsSayWhenAllIsWell(t *testing.T) {
	data := map[string]json.RawMessage{
		widgets.WidgetRequests: json.RawMessage(`{"pendingApprovals":0,"mineActive":0,"openIssues":0}`),
		widgets.WidgetIndexers: json.RawMessage(`{"enabled":1,"failing":0}`),
	}
	cards, err := widgets.CardsOf("x", data, []string{widgets.WidgetRequests, widgets.WidgetIndexers})
	if err != nil || len(cards) != 2 {
		t.Fatalf("%+v %v", cards, err)
	}
	if cards[0].Headline != "Aucune demande en attente" || cards[0].Note != "Aucun signalement" || cards[1].Headline != "1 indexeur" || cards[1].Note != "Tous répondent" {
		t.Errorf("%+v", cards)
	}
	// Not enabled: no card, even with data left over.
	if cards, _ := widgets.CardsOf("x", data, nil); len(cards) != 0 {
		t.Errorf("%+v", cards)
	}
}
