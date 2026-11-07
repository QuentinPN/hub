package pgstore_test

import (
	"testing"
	"time"

	"hub/server/internal/api"
	"hub/server/internal/widgets"
)

func TestGeneralStartsWithTheDefaultsAndSurvivesASave(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	g, err := e.store.General(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if g != api.DefaultGeneral() {
		t.Fatalf("defaults: %+v", g)
	}
	if loc, err := e.store.TimeZone(e.ctx); err != nil || loc != time.Local {
		t.Fatalf("zone before any choice: %v, %v", loc, err)
	}

	want := api.General{DisplayName: "Hub de Quentin", StartPage: api.StartInfra, TimeZone: "America/New_York", WidgetRefresh: api.RefreshEconomy}
	if err := e.store.SaveGeneral(e.ctx, want); err != nil {
		t.Fatal(err)
	}
	if g, err = e.store.General(e.ctx); err != nil || g != want {
		t.Fatalf("%+v, %v", g, err)
	}
	// The backups read the same zone.
	set, err := e.store.BackupSettings(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if set.Location == nil || set.Location.String() != "America/New_York" {
		t.Errorf("backup zone: %v", set.Location)
	}

	want.TimeZone = ""
	if err := e.store.SaveGeneral(e.ctx, want); err != nil {
		t.Fatal(err)
	}
	if loc, err := e.store.TimeZone(e.ctx); err != nil || loc != time.Local {
		t.Errorf("back to the server's: %v, %v", loc, err)
	}
}

func TestEconomyClosesLiveConnectionsAndSpacesReads(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini", e.container("emby", "emby", "linuxserver/emby", "running", 8096))
	now := time.Now()
	e.connected(t, newConnection(now))
	due := func(at time.Time) int {
		t.Helper()
		list, err := e.store.DueConnections(e.ctx, at)
		if err != nil {
			t.Fatal(err)
		}
		return len(list)
	}
	if due(now.Add(31*time.Second)) != 1 {
		t.Fatal("normal: due after 30 s")
	}
	if live, err := e.store.LiveConnections(e.ctx); err != nil || len(live) != 1 {
		t.Fatalf("normal: %v, %v", live, err)
	}

	g := api.DefaultGeneral()
	g.WidgetRefresh = api.RefreshEconomy
	if err := e.store.SaveGeneral(e.ctx, g); err != nil {
		t.Fatal(err)
	}
	if due(now.Add(31*time.Second)) != 0 || due(now.Add(widgets.EconomyRefresh+time.Second)) != 1 {
		t.Error("economy: due after 2 min only")
	}
	if live, err := e.store.LiveConnections(e.ctx); err != nil || len(live) != 0 {
		t.Errorf("economy: no live connection, got %v, %v", live, err)
	}
}
