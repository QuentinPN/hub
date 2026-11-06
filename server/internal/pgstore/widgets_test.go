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
	if want := []string{"emby:ready", "sonarr:planned", "dockhand:link"}; !slices.Equal(got, want) {
		t.Errorf("%v, want %v", got, want)
	}
}
