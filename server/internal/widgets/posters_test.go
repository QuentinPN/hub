package widgets_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hub/server/internal/secrets"
	"hub/server/internal/widgets"
)

var pngBytes = []byte("\x89PNG\r\n\x1a\n0000")

func TestPostersComeOnlyFromTheImageRoutes(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		if r.Header.Get("X-Api-Key") != testKey && r.Header.Get("X-Emby-Token") != testKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/emby/Items/abc/Images/Primary", "/emby/Items/abc/Images/Backdrop", "/api/v3/mediacover/12/poster-500.jpg", "/api/images/w342/a_B-9.jpg":
			_, _ = w.Write(pngBytes)
		case "/api/v3/mediacover/13/poster-500.jpg":
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	key := secrets.NewValue(testKey)
	ctx := context.Background()

	p, ok := widgets.FetchPoster(ctx, widgets.Target{Kind: "emby", URL: srv.URL}, key, "abc")
	if !ok || p.ContentType != "image/png" {
		t.Errorf("emby: %v %q", ok, p.ContentType)
	}
	if _, ok := widgets.FetchPoster(ctx, widgets.Target{Kind: "emby", URL: srv.URL}, key, "abc.backdrop"); !ok || asked[len(asked)-1] != "/emby/Items/abc/Images/Backdrop" {
		t.Errorf("emby backdrop: %v %v", ok, asked)
	}
	if _, ok := widgets.FetchPoster(ctx, widgets.Target{Kind: "sonarr", URL: srv.URL}, key, "12"); !ok {
		t.Error("sonarr poster refused")
	}
	if _, ok := widgets.FetchPoster(ctx, widgets.Target{Kind: "request", URL: srv.URL}, key, "a_B-9.jpg"); !ok {
		t.Error("request poster refused")
	}
	if _, ok := widgets.FetchPoster(ctx, widgets.Target{Kind: "radarr", URL: srv.URL}, key, "13"); ok {
		t.Error("an SVG passed for a poster")
	}
	if _, ok := widgets.FetchPoster(ctx, widgets.Target{Kind: "radarr", URL: srv.URL}, key, "99"); ok {
		t.Error("a missing poster passed")
	}

	before := len(asked)
	for _, c := range []struct{ kind, ref string }{
		{"emby", "../../System/Info"},
		{"emby", "a/b"},
		{"emby", "abc.logo"},
		{"emby", "abc.backdrop.backdrop"},
		{"emby", ".backdrop"},
		{"sonarr", "12/../../queue"},
		{"radarr", "abc"},
		{"qbittorrent", "12"},
		{"seerr", "12"}, // a connector Hub no longer has
		{"request", "../x.jpg"},
		{"request", "..%2fx.jpg"},
		{"request", "a/b.jpg"},
		{"request", "/abc.jpg"},
		{"request", "abc"},
		{"request", "abc.svg"},
		{"request", "abc.jpg.png/x"},
		{"request", "abc.jpg?x=1"},
		{"request", "http:abc.jpg"},
		{"request", `a\b.jpg`},
		{"request", strings.Repeat("a", 101) + ".jpg"},
	} {
		if _, ok := widgets.FetchPoster(ctx, widgets.Target{Kind: c.kind, URL: srv.URL}, key, c.ref); ok {
			t.Errorf("%s %q passed", c.kind, c.ref)
		}
	}
	if len(asked) != before {
		t.Errorf("a refused reference reached the service: %v", asked[before:])
	}
}

func TestReadsCarryPosterReferences(t *testing.T) {
	sonarrSrv := fakeArr(t, `[{"title":"E","seriesId":12,"seasonNumber":1,"episodeNumber":1,"airDateUtc":"2030-01-08T20:00:00Z","hasFile":false,"series":{"title":"S"}}]`)
	data, state := widgets.Read(context.Background(), widgets.Target{Kind: "sonarr", URL: sonarrSrv.URL}, secrets.NewValue(testKey), nil, []string{widgets.WidgetCalendar})
	if state != widgets.StateOK {
		t.Fatal(state)
	}
	var upcoming []widgets.Upcoming
	if err := json.Unmarshal(data[widgets.WidgetCalendar], &upcoming); err != nil {
		t.Fatal(err)
	}
	if len(upcoming) != 1 || upcoming[0].Poster != "12" {
		t.Errorf("sonarr: %+v", upcoming)
	}

	soon := time.Now().AddDate(0, 0, 3).UTC().Format(time.RFC3339)
	radarrSrv := fakeArr(t, `[{"id":34,"title":"F","inCinemas":"`+soon+`","hasFile":false}]`)
	data, state = widgets.Read(context.Background(), widgets.Target{Kind: "radarr", URL: radarrSrv.URL}, secrets.NewValue(testKey), nil, []string{widgets.WidgetCalendar})
	if state != widgets.StateOK {
		t.Fatal(state)
	}
	if err := json.Unmarshal(data[widgets.WidgetCalendar], &upcoming); err != nil {
		t.Fatal(err)
	}
	if len(upcoming) != 1 || upcoming[0].Poster != "34" {
		t.Errorf("radarr: %+v", upcoming)
	}

	embySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"Id":"film","Name":"F","Type":"Movie"},{"Id":"ep","Name":"E","Type":"Episode","SeriesId":"serie","SeriesName":"S"}]`))
	}))
	defer embySrv.Close()
	data, state = widgets.Read(context.Background(), widgets.Target{Kind: "emby", URL: embySrv.URL}, secrets.NewValue(testKey), map[string]string{"userId": "u"}, []string{widgets.EmbyLatest})
	if state != widgets.StateOK {
		t.Fatal(state)
	}
	var latest []widgets.MediaItem
	if err := json.Unmarshal(data[widgets.EmbyLatest], &latest); err != nil {
		t.Fatal(err)
	}
	// An episode shows its series' poster: its own image is a landscape still.
	if len(latest) != 2 || latest[0].Poster != "film" || latest[1].Poster != "serie" {
		t.Errorf("emby: %+v", latest)
	}
}

func TestSessionsCarryTheImagesOfWhatPlays(t *testing.T) {
	embySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"DeviceName":"Salon","NowPlayingItem":{"Id":"film","Name":"F","Type":"Movie","ImageTags":{"Primary":"t"},"BackdropImageTags":["t"]}},
			{"DeviceName":"iPad","NowPlayingItem":{"Id":"ep","Name":"E","Type":"Episode","SeriesId":"serie","SeriesName":"S","SeriesPrimaryImageTag":"t",
			 "ImageTags":{"Primary":"t"},"ParentBackdropItemId":"serie","ParentBackdropImageTags":["t"]}},
			{"DeviceName":"TV","NowPlayingItem":{"Id":"nu","Name":"N","Type":"Movie","ImageTags":{}}},
			{"DeviceName":"PC","NowPlayingItem":{"Id":"ep2","Name":"E2","Type":"Episode","SeriesId":"s2","SeriesName":"S2","BackdropImageTags":["t"]}}
		]`))
	}))
	defer embySrv.Close()
	data, state := widgets.Read(context.Background(), widgets.Target{Kind: "emby", URL: embySrv.URL}, secrets.NewValue(testKey), nil, []string{widgets.EmbySessions})
	if state != widgets.StateOK {
		t.Fatal(state)
	}
	var sessions []widgets.Session
	if err := json.Unmarshal(data[widgets.EmbySessions], &sessions); err != nil {
		t.Fatal(err)
	}
	want := [][2]string{
		{"film", "film.backdrop"},
		// An episode shows its series, not its own still.
		{"serie", "serie.backdrop"},
		// Nothing to show: the hero keeps its colors.
		{"", ""},
		// Without a series image, an episode's own backdrop is better than none.
		{"", "ep2.backdrop"},
	}
	if len(sessions) != len(want) {
		t.Fatalf("%+v", sessions)
	}
	for i, w := range want {
		if sessions[i].Poster != w[0] || sessions[i].Backdrop != w[1] {
			t.Errorf("%d: %q %q, want %q", i, sessions[i].Poster, sessions[i].Backdrop, w)
		}
	}
}

func TestProwlarrWakesOnItsIndexerMessages(t *testing.T) {
	h := &fakeHub{}
	src := runHub(t, h, "prowlarr")
	h.send <- hubMessage("version", "command")
	noRead(t, src, 300*time.Millisecond)
	h.send <- hubMessage("indexerstatus")
	waitRead(t, src)
	h.send <- hubMessage("health")
	waitRead(t, src)
}
