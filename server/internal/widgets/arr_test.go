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

// fakeArr answers Sonarr's and Radarr's documented routes, and refuses a request without the key.
func fakeArr(t *testing.T, calendar string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != testKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v3/queue/status":
			_, _ = w.Write([]byte(`{"totalCount":3,"count":3,"errors":false}`))
		case "/api/v3/calendar":
			if r.URL.Query().Get("start") == "" || r.URL.Query().Get("end") == "" {
				t.Errorf("calendar without a window: %s", r.URL)
			}
			_, _ = w.Write([]byte(calendar))
		case "/api/v3/queue":
			_, _ = w.Write([]byte(`{"page":1,"pageSize":100,"totalRecords":3,"records":[
				{"title":"nom-de-release-prive","status":"downloading","trackedDownloadStatus":"ok"},
				{"title":"autre","status":"queued","trackedDownloadStatus":"ok"},
				{"title":"encore","status":"downloading","trackedDownloadStatus":"warning"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func arrRead(t *testing.T, kind string, srv *httptest.Server) ([]widgets.Upcoming, widgets.Queue, string) {
	t.Helper()
	data, state := widgets.Read(context.Background(), widgets.Target{Kind: kind, URL: srv.URL}, secrets.NewValue(testKey), nil, []string{widgets.WidgetCalendar, widgets.WidgetQueue})
	if state != widgets.StateOK {
		t.Fatal(state)
	}
	var list []widgets.Upcoming
	if err := json.Unmarshal(data[widgets.WidgetCalendar], &list); err != nil {
		t.Fatal(err)
	}
	var q widgets.Queue
	if err := json.Unmarshal(data[widgets.WidgetQueue], &q); err != nil {
		t.Fatal(err)
	}
	return list, q, string(data[widgets.WidgetQueue])
}

func TestSonarrKeepsWhatIsStillToComeSoonestFirst(t *testing.T) {
	srv := fakeArr(t, `[
		{"title":"Second","seasonNumber":2,"episodeNumber":7,"airDateUtc":"2030-01-09T20:00:00Z","hasFile":false,"series":{"title":"Une série"}},
		{"title":"Premier","seasonNumber":2,"episodeNumber":6,"airDateUtc":"2030-01-08T20:00:00Z","hasFile":false,"series":{"title":"Une série"}},
		{"title":"Déjà là","seasonNumber":1,"episodeNumber":1,"airDateUtc":"2030-01-08T10:00:00Z","hasFile":true,"series":{"title":"Autre"}},
		{"title":"Sans date","seasonNumber":1,"episodeNumber":2,"hasFile":false,"series":{"title":"Autre"}}
	]`)
	list, q, raw := arrRead(t, "sonarr", srv)
	want := []widgets.Upcoming{
		{Title: "Une série", Detail: "S02E06 · Sonarr", Kind: "episode", At: "2030-01-08T20:00:00Z"},
		{Title: "Une série", Detail: "S02E07 · Sonarr", Kind: "episode", At: "2030-01-09T20:00:00Z"},
	}
	if fmt.Sprint(list) != fmt.Sprint(want) {
		t.Errorf("%+v", list)
	}
	if q != (widgets.Queue{Total: 3, Downloading: 2, Problems: 1}) {
		t.Errorf("%+v", q)
	}
	if strings.Contains(raw, "nom-de-release-prive") {
		t.Error("a release name reached the queue's counts")
	}
}

func TestQueueItemsNameWhatIsFetched(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("includeSeries") != "true" || r.URL.Query().Get("includeEpisode") != "true" {
			t.Errorf("queue asked without its series: %s", r.URL)
		}
		_, _ = w.Write([]byte(`{"totalRecords":2,"records":[
			{"title":"nom.de.release","status":"downloading","trackedDownloadStatus":"ok","series":{"title":"Une série"},"episode":{"seasonNumber":2,"episodeNumber":6}},
			{"title":"nom.seul","status":"queued","trackedDownloadStatus":"warning"}]}`))
	}))
	defer srv.Close()
	data, state := widgets.Read(context.Background(), widgets.Target{Kind: "sonarr", URL: srv.URL}, secrets.NewValue(testKey), nil, []string{widgets.WidgetQueue})
	if state != widgets.StateOK {
		t.Fatal(state)
	}
	var items []widgets.QueueItem
	if err := json.Unmarshal(data[widgets.DataQueueItems], &items); err != nil {
		t.Fatal(err)
	}
	want := []widgets.QueueItem{
		{Title: "Une série", Detail: "S02E06 · Sonarr", Status: "downloading"},
		{Title: "nom.seul", Detail: "Sonarr", Status: "queued", Problem: true},
	}
	if fmt.Sprint(items) != fmt.Sprint(want) {
		t.Errorf("%+v", items)
	}
}

func TestRadarrPicksTheNextDateOfAFilm(t *testing.T) {
	past := time.Now().AddDate(0, -2, 0).UTC().Format(time.RFC3339)
	soon := time.Now().AddDate(0, 0, 3).UTC().Format(time.RFC3339)
	later := time.Now().AddDate(0, 0, 9).UTC().Format(time.RFC3339)
	srv := fakeArr(t, fmt.Sprintf(`[
		{"title":"Déjà sorti en salle","inCinemas":%q,"digitalRelease":%q,"physicalRelease":%q,"hasFile":false},
		{"title":"En salle bientôt","inCinemas":%q,"hasFile":false},
		{"title":"Déjà là","digitalRelease":%q,"hasFile":true},
		{"title":"Date passée","inCinemas":%q,"hasFile":false}
	]`, past, later, soon, soon, soon, past))
	list, _, _ := arrRead(t, "radarr", srv)
	if len(list) != 2 {
		t.Fatalf("%+v", list)
	}
	for _, u := range list {
		if u.Title == "Déjà sorti en salle" && u.Detail != "En physique · Radarr" {
			t.Errorf("next date: %+v", u)
		}
		if u.Kind != "movie" || u.At != soon {
			t.Errorf("%+v", u)
		}
	}
}

func TestArrProbeUsesTheHeaderKey(t *testing.T) {
	srv := fakeArr(t, "[]")
	for _, kind := range []string{"sonarr", "radarr"} {
		p, state := widgets.TestKey(context.Background(), widgets.Target{Kind: kind, URL: srv.URL}, secrets.NewValue(testKey))
		if state != widgets.StateOK || p.Note != "3 éléments en file d’attente" {
			t.Errorf("%s: %s %+v", kind, state, p)
		}
		if _, state := widgets.TestKey(context.Background(), widgets.Target{Kind: kind, URL: srv.URL}, secrets.NewValue("mauvaise")); state != widgets.StateAuth {
			t.Errorf("%s: wrong key gave %s", kind, state)
		}
	}
}

func TestQueueTextCountsInTheRightUnit(t *testing.T) {
	for _, c := range []struct {
		kind           string
		q              widgets.Queue
		headline, note string
	}{
		{"sonarr", widgets.Queue{}, "File vide", "Rien à récupérer"},
		{"sonarr", widgets.Queue{Total: 2, Downloading: 1}, "2 épisodes", "1 en cours · 1 en attente"},
		{"radarr", widgets.Queue{Total: 1, Downloading: 1}, "1 film", "1 en cours · 0 en attente"},
		{"radarr", widgets.Queue{Total: 3, Problems: 1}, "3 films", "0 en cours · 3 en attente · 1 à vérifier"},
	} {
		h, n := widgets.QueueText(c.kind, c.q)
		if h != c.headline || n != c.note {
			t.Errorf("%+v: %q, %q", c, h, n)
		}
	}
}
