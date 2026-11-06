package pgstore_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"hub/server/internal/api"
	"hub/server/internal/urlcheck"
	"hub/server/internal/widgets"
)

func (e *discoveryEnv) connected(t *testing.T, c api.NewConnection) {
	t.Helper()
	if err := e.store.SaveServiceURL(e.ctx, "emby", "http://emby", "", urlcheck.Result{Kind: urlcheck.KindOK, Status: 200}); err != nil {
		t.Fatal(err)
	}
	if err := e.store.SaveWidgetConnection(e.ctx, "emby", c); err != nil {
		t.Fatal(err)
	}
}

func newConnection(at time.Time) api.NewConnection {
	return api.NewConnection{Kind: "emby", URL: "http://emby", KeyHint: "7f2a", Options: map[string]string{}, Enabled: []string{"sessions"},
		RefreshSeconds: 30, State: widgets.StateOK, At: at,
		Data: map[string]json.RawMessage{"sessions": json.RawMessage(`[{"title":"Le film"}]`)}}
}

func (e *discoveryEnv) widget(t *testing.T) *api.WidgetConnection {
	t.Helper()
	d, err := e.store.Service(e.ctx, "emby")
	if err != nil {
		t.Fatal(err)
	}
	return d.Widget.Connection
}

func TestConnectionsBecomeDueAtTheirRateAndBackOffOnFailure(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini", e.container("emby", "emby", "linuxserver/emby", "running", 8096))
	now := time.Now()
	e.connected(t, newConnection(now))

	due := func(at time.Time) []widgets.Due {
		t.Helper()
		list, err := e.store.DueConnections(e.ctx, at)
		if err != nil {
			t.Fatal(err)
		}
		return list
	}
	if len(due(now.Add(29*time.Second))) != 0 {
		t.Error("due before its rate")
	}
	list := due(now.Add(30 * time.Second))
	if len(list) != 1 || list[0].URL != "http://emby" || list[0].Kind != "emby" {
		t.Fatalf("%+v", list)
	}

	// A failure keeps the data, dated, and doubles the wait.
	if err := e.store.RecordRead(e.ctx, list[0], nil, widgets.StateUnreachable, now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	c := e.widget(t)
	if c.State != "unreachable" || len(c.Data["sessions"]) == 0 || c.FailingSince == nil || c.DataAt == nil {
		t.Errorf("%+v", c)
	}
	if len(due(now.Add(89*time.Second))) != 0 || len(due(now.Add(90*time.Second))) != 1 {
		t.Error("failure did not space the next read")
	}

	// A good read clears the failure.
	fresh := map[string]json.RawMessage{"sessions": json.RawMessage(`[]`)}
	if err := e.store.RecordRead(e.ctx, list[0], fresh, widgets.StateOK, now.Add(90*time.Second)); err != nil {
		t.Fatal(err)
	}
	if c := e.widget(t); c.State != "ok" || c.FailingSince != nil || string(c.Data["sessions"]) != "[]" {
		t.Errorf("%+v", c)
	}

	// A refused key is not retried and shows nothing.
	if err := e.store.RecordRead(e.ctx, list[0], nil, widgets.StateAuth, now.Add(100*time.Second)); err != nil {
		t.Fatal(err)
	}
	if c := e.widget(t); c.State != "auth" || len(c.Data) != 0 {
		t.Errorf("%+v", c)
	}
	if len(due(now.Add(time.Hour))) != 0 {
		t.Error("a refused key is retried")
	}
}

func TestReadBegunBeforeASaveIsIgnored(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini", e.container("emby", "emby", "linuxserver/emby", "running", 8096))
	now := time.Now()
	e.connected(t, newConnection(now))
	list, err := e.store.DueConnections(e.ctx, now.Add(time.Minute))
	if err != nil || len(list) != 1 {
		t.Fatalf("%+v %v", list, err)
	}
	if err := e.store.SaveWidgetConnection(e.ctx, "emby", newConnection(now)); err != nil {
		t.Fatal(err)
	}
	if err := e.store.RecordRead(e.ctx, list[0], nil, widgets.StateAuth, now); err != nil {
		t.Fatal(err)
	}
	if c := e.widget(t); c.State != "ok" {
		t.Errorf("the old key's refusal landed on the new one: %s", c.State)
	}
}

func TestStoppedServiceIsNotAskedAndLosesItsSessions(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini", e.container("emby", "emby", "linuxserver/emby", "running", 8096))
	now := time.Now()
	e.connected(t, newConnection(now))
	home, err := e.store.Home(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(home.Sessions) != 1 || home.SessionsSlug != "emby" {
		t.Errorf("%+v", home.Sessions)
	}

	e.snapshot(t, "mini", e.container("emby", "emby", "linuxserver/emby", "exited", 8096))
	if c := e.widget(t); c.State != api.WidgetStopped || len(c.Data) != 0 {
		t.Errorf("%+v", c)
	}
	list, err := e.store.DueConnections(e.ctx, now.Add(time.Hour))
	if err != nil || len(list) != 0 {
		t.Errorf("a stopped service is asked: %+v %v", list, err)
	}
	home, err = e.store.Home(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(home.Sessions) != 0 {
		t.Errorf("sessions of a stopped service: %+v", home.Sessions)
	}
}

func TestRowsListWhatHubKnowsFirstTheActionable(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini",
		e.container("dockhand", "dockhand", "fnsys/dockhand", "running", 3000),
		e.container("prowlarr", "prowlarr", "linuxserver/prowlarr", "running", 9696),
		e.container("beszel", "beszel", "henrygd/beszel", "running", 8090),
		e.container("sonarr", "sonarr", "linuxserver/sonarr", "running", 8989),
		e.container("emby", "emby", "linuxserver/emby", "running", 8096),
		e.container("autre", "autre", "exemple/autre", "running", 80))
	rows, err := e.store.WidgetRows(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Slug+":"+string(r.Support))
	}
	if want := []string{"emby:ready", "prowlarr:ready", "sonarr:ready", "beszel:planned", "dockhand:link"}; !slices.Equal(got, want) {
		t.Errorf("%v, want %v", got, want)
	}
}

func TestHomeGathersUpcomingAndQueues(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini",
		e.container("sonarr", "sonarr", "linuxserver/sonarr", "running", 8989),
		e.container("radarr", "radarr", "linuxserver/radarr", "running", 7878),
		e.container("emby", "emby", "linuxserver/emby", "running", 8096))
	now := time.Now()
	connect := func(slug, kind, calendar, queue string) {
		t.Helper()
		address := "http://" + slug
		if err := e.store.SaveServiceURL(e.ctx, slug, address, "", urlcheck.Result{Kind: urlcheck.KindOK, Status: 200}); err != nil {
			t.Fatal(err)
		}
		err := e.store.SaveWidgetConnection(e.ctx, slug, api.NewConnection{Kind: kind, URL: address, KeyHint: "abcd", Options: map[string]string{},
			Enabled: []string{"calendar", "queue"}, RefreshSeconds: 30, State: widgets.StateOK, At: now,
			Data: map[string]json.RawMessage{"calendar": json.RawMessage(calendar), "queue": json.RawMessage(queue)}})
		if err != nil {
			t.Fatal(err)
		}
	}
	connect("sonarr", "sonarr", `[{"title":"Une série","detail":"S02E06 · Sonarr","kind":"episode","at":"2030-01-09T20:00:00Z"}]`, `{"total":2,"downloading":1,"problems":0}`)
	connect("radarr", "radarr", `[{"title":"Un film","detail":"En salle · Radarr","kind":"movie","at":"2030-01-08T00:00:00Z"}]`, `{"total":0,"downloading":0,"problems":0}`)

	home, err := e.store.Home(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(home.Upcoming) != 2 || home.Upcoming[0].Title != "Un film" || home.Upcoming[1].At == "" {
		t.Errorf("upcoming: %+v", home.Upcoming)
	}
	if len(home.Downloads) != 2 {
		t.Fatalf("downloads: %+v", home.Downloads)
	}
	byName := map[string]api.DownloadCard{}
	for _, c := range home.Downloads {
		byName[c.Slug] = c
	}
	if c := byName["sonarr"]; c.Headline != "2 épisodes" || c.Note != "1 en cours · 1 en attente" || c.State != "ok" {
		t.Errorf("%+v", c)
	}
	if c := byName["radarr"]; c.Headline != "File vide" {
		t.Errorf("%+v", c)
	}

	// A refused key shows nothing on the home page; an outage keeps the last read, dated.
	list, err := e.store.DueConnections(e.ctx, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range list {
		state := widgets.StateUnreachable
		if d.Slug == "radarr" {
			state = widgets.StateAuth
		}
		if err := e.store.RecordRead(e.ctx, d, nil, state, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	home, err = e.store.Home(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(home.Downloads) != 1 || home.Downloads[0].Slug != "sonarr" || home.Downloads[0].State != "unreachable" || home.Downloads[0].DataAt == nil {
		t.Errorf("downloads: %+v", home.Downloads)
	}
	if len(home.Upcoming) != 1 || home.Upcoming[0].Title != "Une série" {
		t.Errorf("upcoming: %+v", home.Upcoming)
	}
}

func TestHomeShowsTransfersWithTheirHost(t *testing.T) {
	e := newDiscoveryEnv(t, "qnap")
	e.snapshot(t, "qnap", e.container("qbittorrent", "qbittorrent", "linuxserver/qbittorrent", "running", 8181))
	now := time.Now()
	if err := e.store.SaveServiceURL(e.ctx, "qbittorrent", "http://q", "", urlcheck.Result{Kind: urlcheck.KindOK, Status: 200}); err != nil {
		t.Fatal(err)
	}
	err := e.store.SaveWidgetConnection(e.ctx, "qbittorrent", api.NewConnection{Kind: "qbittorrent", URL: "http://q", Options: map[string]string{},
		Enabled: []string{"transfer"}, RefreshSeconds: 30, State: widgets.StateOK, At: now,
		Data: map[string]json.RawMessage{"transfer": json.RawMessage(`{"downSpeed":12400000,"upSpeed":1100000,"downloading":3,"connection":"connected"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	home, err := e.store.Home(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(home.Downloads) != 1 || home.Downloads[0].Label != "qBittorrent · qnap" || home.Downloads[0].Headline != "12,4 Mo/s" {
		t.Errorf("%+v", home.Downloads)
	}
}
