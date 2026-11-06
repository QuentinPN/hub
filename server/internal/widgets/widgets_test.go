package widgets_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"hub/server/internal/secrets"
	"hub/server/internal/widgets"
)

const testKey = "cle-de-test-0123"

const embySessionsJSON = `[
	{"Id":"1","DeviceName":"Salon","UserName":"secret-user","NowPlayingItem":{"Name":"Le film","Type":"Movie","RunTimeTicks":60000000000},
	 "PlayState":{"PositionTicks":15000000000,"IsPaused":false,"PlayMethod":"DirectPlay"}},
	{"Id":"2","DeviceName":"iPad","NowPlayingItem":{"Name":"Pilote","Type":"Episode","SeriesName":"Une série","ParentIndexNumber":2,"IndexNumber":5,"RunTimeTicks":12000000000},
	 "PlayState":{"PositionTicks":6000000000,"IsPaused":true,"PlayMethod":"Transcode"}},
	{"Id":"3","DeviceName":"Inactif","PlayState":{}}
]`

// fakeEmby answers the documented routes, and refuses a request without the key.
func fakeEmby(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != testKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if strings.Contains(r.URL.RawQuery, testKey) {
			t.Errorf("key in the URL: %s", r.URL)
		}
		switch r.URL.Path {
		case "/emby/Sessions":
			_, _ = w.Write([]byte(embySessionsJSON))
		case "/emby/Users/Query":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"u1","Name":"admin"}],"TotalRecordCount":1}`))
		case "/emby/Users/u1/Items/Latest":
			_, _ = w.Write([]byte(`[{"Name":"Un film","Type":"Movie","DateCreated":"2026-11-05T10:00:00Z"},
				{"Name":"Pilote","Type":"Episode","SeriesName":"Une série","ParentIndexNumber":2,"IndexNumber":5,"DateCreated":"2026-11-04T10:00:00Z"}]`))
		case "/emby/Items/Counts":
			_, _ = w.Write([]byte(`{"MovieCount":120,"SeriesCount":30,"EpisodeCount":900,"SongCount":7}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func target(srv *httptest.Server) widgets.Target {
	return widgets.Target{Kind: "emby", URL: srv.URL}
}

func TestEmbyProbeFindsAccountsAndCountsPlaybacks(t *testing.T) {
	p, state := widgets.TestKey(context.Background(), target(fakeEmby(t)), secrets.NewValue(testKey))
	if state != widgets.StateOK {
		t.Fatal(state)
	}
	if p.Note != "2 lectures en cours" || len(p.Options) != 1 || p.Options[0].Choices[0].Label != "admin" {
		t.Errorf("%+v", p)
	}
}

func TestEmbyReadKeepsOnlyWhatTheWidgetsShow(t *testing.T) {
	all := []string{widgets.EmbySessions, widgets.EmbyLatest, widgets.EmbyLibrary}
	data, state := widgets.Read(context.Background(), target(fakeEmby(t)), secrets.NewValue(testKey), map[string]string{"userId": "u1"}, all)
	if state != widgets.StateOK {
		t.Fatal(state)
	}
	var sessions []widgets.Session
	if err := json.Unmarshal(data[widgets.EmbySessions], &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("idle sessions kept: %+v", sessions)
	}
	if s := sessions[0]; s.Title != "Le film" || s.Device != "Salon" || s.Progress != 0.25 || s.DurationLabel != "75 min restantes" || s.Transcoding {
		t.Errorf("%+v", s)
	}
	if s := sessions[1]; s.Title != "Une série, S02E05" || s.Subtitle != "Pilote" || !s.Transcoding || s.DurationLabel != "en pause · 10 min restantes" {
		t.Errorf("%+v", s)
	}
	if strings.Contains(string(data[widgets.EmbySessions]), "secret-user") {
		t.Error("the account name reached the stored data")
	}
	var latest []widgets.MediaItem
	if err := json.Unmarshal(data[widgets.EmbyLatest], &latest); err != nil {
		t.Fatal(err)
	}
	if len(latest) != 2 || latest[0].Detail != "Film" || latest[1].Title != "Une série" || latest[1].Detail != "S02E05" {
		t.Errorf("%+v", latest)
	}
	var lib widgets.Library
	if err := json.Unmarshal(data[widgets.EmbyLibrary], &lib); err != nil {
		t.Fatal(err)
	}
	if lib != (widgets.Library{Movies: 120, Series: 30, Episodes: 900}) {
		t.Errorf("%+v", lib)
	}
}

func TestEmbyReadsOnlyWhatIsEnabled(t *testing.T) {
	data, state := widgets.Read(context.Background(), target(fakeEmby(t)), secrets.NewValue(testKey), nil, []string{widgets.EmbySessions, widgets.EmbyLatest})
	if state != widgets.StateOK {
		t.Fatal(state)
	}
	if _, ok := data[widgets.EmbyLibrary]; ok {
		t.Error("library read without being asked")
	}
	if _, ok := data[widgets.EmbyLatest]; ok {
		t.Error("latest read without an account")
	}
}

func TestFailuresMapToStates(t *testing.T) {
	statusOf := func(status int, body string) widgets.State {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		defer srv.Close()
		_, state := widgets.Read(context.Background(), target(srv), secrets.NewValue(testKey), nil, []string{widgets.EmbySessions})
		return state
	}
	for _, c := range []struct {
		status int
		body   string
		want   widgets.State
	}{
		{401, "", widgets.StateAuth},
		{403, "", widgets.StateAuth},
		{502, "", widgets.StateUnreachable},
		{404, "", widgets.StateUnexpected},
		{200, "<html>", widgets.StateUnexpected},
		{200, `{"not":"a list"}`, widgets.StateUnexpected},
	} {
		if got := statusOf(c.status, c.body); got != c.want {
			t.Errorf("%d %q: %s, want %s", c.status, c.body, got, c.want)
		}
	}

	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	if _, state := widgets.TestKey(context.Background(), target(closed), secrets.NewValue(testKey)); state != widgets.StateUnreachable {
		t.Errorf("closed port: %s", state)
	}
}

func TestSelfSignedCertificateIsRefusedUnlessPinned(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()
	_, state := widgets.Read(context.Background(), target(srv), secrets.NewValue(testKey), nil, []string{widgets.EmbySessions})
	if state != widgets.StateCert {
		t.Errorf("unpinned: %s", state)
	}
}

func TestRedirectIsNotFollowedWithTheKey(t *testing.T) {
	var leaked sync.Map
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		leaked.Store("token", r.Header.Get("X-Emby-Token"))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.RedirectHandler(other.URL, http.StatusFound))
	defer srv.Close()
	if _, state := widgets.Read(context.Background(), target(srv), secrets.NewValue(testKey), nil, []string{widgets.EmbySessions}); state != widgets.StateUnexpected {
		t.Errorf("redirect: %s", state)
	}
	if _, ok := leaked.Load("token"); ok {
		t.Error("the key followed the redirect")
	}
}

func TestNextDelaySpacesFailuresOut(t *testing.T) {
	for _, c := range []struct {
		failures int
		want     time.Duration
	}{{0, 30 * time.Second}, {1, time.Minute}, {2, 2 * time.Minute}, {10, 10 * time.Minute}} {
		if got := widgets.NextDelay(30*time.Second, c.failures); got != c.want {
			t.Errorf("%d failures: %s", c.failures, got)
		}
	}
	if got := widgets.NextDelay(time.Hour, 3); got != time.Hour {
		t.Errorf("a slow rate is not shortened: %s", got)
	}
}

func TestKeyHintNeverShowsAShortKey(t *testing.T) {
	if widgets.KeyHint("abc") != "" || widgets.KeyHint("0123456789abcdef") != "cdef" {
		t.Error(widgets.KeyHint("0123456789abcdef"))
	}
}

// memSource hands out each connection once and collects what is read.
type memSource struct {
	mu    sync.Mutex
	due   []widgets.Due
	live  []widgets.Due
	reads map[string]widgets.State
	done  chan string
}

func (m *memSource) LiveConnections(context.Context) ([]widgets.Due, error) { return m.live, nil }

func (m *memSource) DueConnections(context.Context, time.Time) ([]widgets.Due, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.due
	m.due = nil
	return d, nil
}

func (m *memSource) RecordRead(_ context.Context, d widgets.Due, _ map[string]json.RawMessage, s widgets.State, _ time.Time) error {
	m.mu.Lock()
	m.reads[d.Slug] = s
	m.mu.Unlock()
	m.done <- d.Slug
	return nil
}

type memKeys struct{}

func (memKeys) Get(_ context.Context, name string) (secrets.Value, error) {
	if name == widgets.KeyName("nokey") {
		return secrets.Value{}, secrets.ErrNotFound
	}
	return secrets.NewValue(testKey), nil
}

func TestRunIsolatesAHangingServiceFromTheOthers(t *testing.T) {
	release := make(chan struct{})
	hanging := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer hanging.Close()
	defer close(release)
	healthy := fakeEmby(t)
	src := &memSource{reads: map[string]widgets.State{}, done: make(chan string, 4), due: []widgets.Due{
		{Slug: "hang", Target: target(hanging), Enabled: []string{widgets.EmbySessions}},
		{Slug: "good", Target: target(healthy), Enabled: []string{widgets.EmbySessions}},
		{Slug: "nokey", Target: target(healthy), Enabled: []string{widgets.EmbySessions}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go widgets.Run(ctx, src, memKeys{}, widgets.NewBus(), 10*time.Millisecond)

	seen := map[string]bool{}
	timeout := time.After(3 * time.Second)
	for !seen["good"] || !seen["nokey"] {
		select {
		case slug := <-src.done:
			seen[slug] = true
		case <-timeout:
			t.Fatalf("a hanging service held the others up: %v", seen)
		}
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.reads["good"] != widgets.StateOK || src.reads["nokey"] != widgets.StateAuth {
		t.Errorf("%v", src.reads)
	}
}
