package pgstore_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"hub/server/internal/api"
	"hub/server/internal/secrets"
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
	if want := []string{"beszel:ready", "emby:ready", "prowlarr:ready", "sonarr:ready", "dockhand:link"}; !slices.Equal(got, want) {
		t.Errorf("%v, want %v", got, want)
	}
}

// The key's permission is kept with the connection: it shows in the row and in the fiche
// whatever widgets are on, and a refresh leaves it be.
func TestConnectionKeepsWhetherTheKeyCanAct(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini", e.container("request", "request", "ghcr.io/quentinpn/request", "running"))
	if err := e.store.SaveServiceURL(e.ctx, "request", "http://request", "", urlcheck.Result{Kind: urlcheck.KindOK, Status: 200}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		canAct *bool
		want   *bool
	}{
		{new(false), new(false)},
		{new(true), new(true)},
		// Saved before the column existed, or by a connector that does not say: unknown, never "cannot act".
		{nil, nil},
	} {
		// The requests widget is off: only the stored permission can tell.
		err := e.store.SaveWidgetConnection(e.ctx, "request", api.NewConnection{Kind: "request", URL: "http://request", KeyHint: "abcd",
			Options: map[string]string{}, Enabled: []string{"pending"}, RefreshSeconds: 60, State: widgets.StateOK, CanAct: c.canAct, At: time.Now(),
			Data: map[string]json.RawMessage{"pending": json.RawMessage(`{"total":0,"items":[]}`)}})
		if err != nil {
			t.Fatal(err)
		}
		rows, err := e.store.WidgetRows(e.ctx)
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(rows, func(r api.WidgetRow) bool { return r.Slug == "request" })
		if i < 0 || !sameFlag(rows[i].CanAct, c.want) {
			t.Errorf("row: want %v, got %+v", c.want, rows)
		}
		d, err := e.store.Service(e.ctx, "request")
		if err != nil {
			t.Fatal(err)
		}
		if !sameFlag(d.Widget.Connection.CanAct, c.want) {
			t.Errorf("fiche: want %v, got %v", c.want, d.Widget.Connection.CanAct)
		}
	}
}

func sameFlag(a, b *bool) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

func TestARefreshLeavesWhetherTheKeyCanActAlone(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini", e.container("request", "request", "ghcr.io/quentinpn/request", "running"))
	if err := e.store.SaveServiceURL(e.ctx, "request", "http://request", "", urlcheck.Result{Kind: urlcheck.KindOK, Status: 200}); err != nil {
		t.Fatal(err)
	}
	err := e.store.SaveWidgetConnection(e.ctx, "request", api.NewConnection{Kind: "request", URL: "http://request", KeyHint: "abcd",
		Options: map[string]string{}, Enabled: []string{"pending"}, RefreshSeconds: 60, State: widgets.StateOK, CanAct: new(false), At: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	due := widgets.Due{Slug: "request", Generation: 1}
	for _, state := range []widgets.State{widgets.StateOK, widgets.StateUnreachable} {
		if err := e.store.RecordRead(e.ctx, due, map[string]json.RawMessage{}, state, time.Now()); err != nil {
			t.Fatal(err)
		}
		d, err := e.store.Service(e.ctx, "request")
		if err != nil {
			t.Fatal(err)
		}
		if c := d.Widget.Connection.CanAct; c == nil || *c {
			t.Errorf("after a %s read: %v", state, c)
		}
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

// A connection made with a connector Hub no longer has (Seerr) stays until removed: shown as
// such, never read, its data and cards gone, and removable.
func TestConnectionOfAConnectorNoLongerInHub(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini", e.container("seerr", "seerr", "seerr-team/seerr", "running", 5055))
	e.connect(t, "seerr", "seerr", []string{"requests"}, map[string]string{"requests": `{"total":9,"pending":7}`})

	d, err := e.store.Service(e.ctx, "seerr")
	if err != nil {
		t.Fatal(err)
	}
	if d.Widget == nil || d.Widget.Support != widgets.SupportNone || d.Widget.Manifest != nil || d.Widget.Connection == nil ||
		d.Widget.Connection.State != api.WidgetUnsupported || len(d.Widget.Connection.Data) != 0 || d.Widget.Connection.Webhook != nil {
		t.Fatalf("%+v", d.Widget)
	}
	rows, err := e.store.WidgetRows(e.ctx)
	if err != nil || len(rows) != 1 || rows[0].State != api.WidgetUnsupported {
		t.Errorf("rows: %+v %v", rows, err)
	}
	due, err := e.store.DueConnections(e.ctx, time.Now().Add(time.Hour))
	if err != nil || len(due) != 0 {
		t.Errorf("due: %+v %v", due, err)
	}
	if live, err := e.store.LiveConnections(e.ctx); err != nil || len(live) != 0 {
		t.Errorf("live: %+v %v", live, err)
	}
	home, err := e.store.Home(e.ctx)
	if err != nil || len(home.Downloads) != 0 {
		t.Errorf("home: %+v %v", home.Downloads, err)
	}
	target, err := e.store.WidgetTarget(e.ctx, "seerr")
	if err != nil || target.Support != widgets.SupportNone {
		t.Errorf("target: %+v %v", target, err)
	}
	if err := e.store.DeleteWidgetConnection(e.ctx, "seerr"); err != nil {
		t.Fatal(err)
	}
	if d, err := e.store.Service(e.ctx, "seerr"); err != nil || d.Widget != nil {
		t.Errorf("after removal: %+v %v", d.Widget, err)
	}
}

func TestWebhookStateNeverCarriesTheSecret(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini", e.container("request", "request", "ghcr.io/quentinpn/request", "running"))
	e.connect(t, "request", "request", []string{"requests"}, map[string]string{"requests": `{"pendingApprovals":1,"mineActive":0,"openIssues":0}`})
	webhook := func() *api.WebhookState {
		d, err := e.store.Service(e.ctx, "request")
		if err != nil {
			t.Fatal(err)
		}
		return d.Widget.Connection.Webhook
	}
	if w := webhook(); w == nil || w.Configured {
		t.Fatalf("%+v", w)
	}
	now := time.Now()
	if err := e.store.PutSecretRow(e.ctx, secrets.Row{Name: widgets.WebhookName("request"), KeyID: "k", Ciphertext: []byte(strings.Repeat("x", 29)), CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if w := webhook(); w == nil || !w.Configured {
		t.Errorf("%+v", w)
	}
}
