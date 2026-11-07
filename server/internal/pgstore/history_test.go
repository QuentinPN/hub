package pgstore_test

import (
	"testing"
	"time"

	"hub/server/internal/agentproto"
	"hub/server/internal/api"
	"hub/server/internal/notify"
)

func (e *discoveryEnv) history(t *testing.T, slug string, q api.HistoryQuery) api.ServiceHistory {
	t.Helper()
	if q.Limit == 0 {
		q.Limit = 100
	}
	h, err := e.store.ServiceHistory(e.ctx, slug, q, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func (e *discoveryEnv) changes(t *testing.T, host string, c agentproto.Changes) {
	t.Helper()
	if err := e.store.ApplyContainerChanges(e.ctx, host, c, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryKeepsContainerEventsVersionsEditsAndAlerts(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	app := e.container("tz", "tz", "org/tz:1.0", "running")
	db := e.container("tz", "db", "postgres:17", "running")
	other := e.container("solo", "solo", "solo:1", "running")
	e.snapshot(t, "mini", app, db, other)
	if h := e.history(t, "tz", api.HistoryQuery{}); h.Total != 0 || len(h.Events) != 0 {
		t.Fatalf("a first snapshot is no change: %+v", h.Events)
	}

	// An OOM: Docker says oom, then die with 137. An event of another service stays there.
	code := 137
	app.State = "exited"
	e.changes(t, "mini", agentproto.Changes{Upserted: []agentproto.Container{app}, Events: []agentproto.Event{
		{ContainerID: app.ID, Action: agentproto.ActionOOM},
		{ContainerID: app.ID, Action: agentproto.ActionDie, ExitCode: &code},
		{ContainerID: other.ID, Action: agentproto.ActionRestart},
		{ContainerID: "f" + app.ID[1:], Action: agentproto.ActionStart},
	}})

	// Recreated on a new image: a new container replaces the old one.
	app2 := e.container("tz", "tz", "org/tz:1.1@sha256:abc", "running")
	e.snapshot(t, "mini", app2, db, other)
	// Same image again on another container: no version change.
	app3 := e.container("tz", "tz", "org/tz:1.1@sha256:abc", "running")
	e.snapshot(t, "mini", app3, db, other)

	if err := e.store.RecordServiceEdit(e.ctx, "tz", "service_url_set", nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := e.store.RecordServiceEdit(e.ctx, "nope", "service_url_set", nil, time.Now()); err != nil {
		t.Fatalf("an edit of an unknown service is dropped, not an error: %v", err)
	}
	if err := e.store.LogAlert(e.ctx, "tz", notify.SentAlert{Alert: notify.AlertService, Title: "tz est arrêté ou injoignable",
		Via: []string{notify.Ntfy}, At: time.Now()}); err != nil {
		t.Fatal(err)
	}

	h := e.history(t, "tz", api.HistoryQuery{})
	if h.Total != 5 || h.Counts[api.HistoryContainer] != 2 || h.Counts[api.HistoryVersion] != 1 ||
		h.Counts[api.HistoryEdit] != 1 || h.Counts[api.HistoryAlert] != 1 {
		t.Fatalf("total %d, counts %v", h.Total, h.Counts)
	}
	got := map[string]api.HistoryEvent{}
	for _, ev := range h.Events {
		got[ev.Kind+"/"+ev.Action] = ev
	}
	if ev := got["container/oom"]; ev.Container != "tz" || ev.ExitCode != nil {
		t.Errorf("oom: %+v", ev)
	}
	if ev := got["container/die"]; ev.ExitCode == nil || *ev.ExitCode != 137 {
		t.Errorf("die: %+v", ev)
	}
	if ev := got["version/changed"]; ev.From != "1.0" || ev.To != "1.1" || ev.Image != "org/tz:1.1" || ev.Container != "tz" {
		t.Errorf("version: %+v", ev)
	}
	if ev := got["alert/sent"]; ev.Title != "tz est arrêté ou injoignable" || len(ev.Via) != 1 || ev.Via[0] != notify.Ntfy {
		t.Errorf("alert: %+v", ev)
	}
	if h.Events[0].Kind != api.HistoryAlert {
		t.Errorf("newest first: %+v", h.Events[0])
	}
	if s := e.history(t, "solo", api.HistoryQuery{}); s.Total != 1 || s.Events[0].Action != agentproto.ActionRestart {
		t.Errorf("other service: %+v", s.Events)
	}

	// Filters and pages.
	f := e.history(t, "tz", api.HistoryQuery{Kinds: []string{api.HistoryContainer, api.HistoryVersion}, Limit: 2, Offset: 1})
	if f.Total != 3 || len(f.Events) != 2 || f.Counts[api.HistoryAlert] != 1 {
		t.Fatalf("filtered page: total %d, %+v", f.Total, f.Events)
	}
	for _, ev := range f.Events {
		if ev.Kind == api.HistoryEdit || ev.Kind == api.HistoryAlert {
			t.Errorf("filter let %s through", ev.Kind)
		}
	}

	// A container in a restart loop fills an hour once.
	for range 5 {
		evs := make([]agentproto.Event, agentproto.MaxEvents)
		for i := range evs {
			evs[i] = agentproto.Event{ContainerID: db.ID, Action: agentproto.ActionStart}
		}
		e.changes(t, "mini", agentproto.Changes{Events: evs})
	}
	if h := e.history(t, "tz", api.HistoryQuery{Kinds: []string{api.HistoryContainer}}); h.Total != 60 {
		t.Errorf("restart loop kept %d events", h.Total)
	}

	if _, err := e.store.ServiceHistory(e.ctx, "nope", api.HistoryQuery{Limit: 10}, time.Now()); err != api.ErrNotFound {
		t.Errorf("unknown service: %v", err)
	}
}

func TestHistorySamplesOutagesAndPurges(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	app := e.container("tz", "tz", "org/tz:1", "running")
	t0 := time.Now().Truncate(time.Minute).Add(time.Minute)
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	sample := func(m int, state string) {
		t.Helper()
		app.State = state
		e.snapshotAt(t, "mini", at(m), app)
		if err := e.store.SampleHistory(e.ctx, at(m).Add(10*time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	sample(0, "running")
	sample(1, "exited")
	sample(1, "exited") // same minute: counted once
	sample(2, "exited")
	sample(21, "running")

	h := e.history(t, "tz", api.HistoryQuery{Kinds: []string{api.HistoryOutage}})
	if h.Total != 2 {
		t.Fatalf("outage events: %+v", h.Events)
	}
	up, down := h.Events[0], h.Events[1]
	if up.Action != api.OutageUp || up.Minutes == nil || *up.Minutes != 20 {
		t.Errorf("recovery: %+v", up)
	}
	if down.Action != api.OutageDown || down.Minutes == nil || *down.Minutes != 20 || down.Reason == "" {
		t.Errorf("outage: %+v", down)
	}

	var upMin, downMin, outages int
	for _, d := range e.history(t, "tz", api.HistoryQuery{}).Days {
		upMin += d.UpMinutes
		downMin += d.DownMinutes
		outages += d.Outages
	}
	if len(h.Days) != api.HistoryBarDays || upMin != 2 || downMin != 2 || outages != 1 {
		t.Errorf("%d days, up %d, down %d, outages %d", len(h.Days), upMin, downMin, outages)
	}

	// A host that does not answer: the minute is unknown, and no outage opens or closes.
	app.State = "exited"
	e.snapshotAt(t, "mini", time.Now().Add(-time.Hour), app)
	if err := e.store.SampleHistory(e.ctx, at(30)); err != nil {
		t.Fatal(err)
	}
	unknown := 0
	for _, d := range e.history(t, "tz", api.HistoryQuery{}).Days {
		unknown += d.UnknownMinutes
	}
	if unknown != 1 {
		t.Errorf("unknown minutes: %d", unknown)
	}
	if h := e.history(t, "tz", api.HistoryQuery{Kinds: []string{api.HistoryOutage}}); h.Total != 2 {
		t.Errorf("an unreachable host opened an outage: %+v", h.Events)
	}

	// A shorter retention erases the older history at once.
	if err := e.store.RecordServiceEdit(e.ctx, "tz", "service_updated", nil, time.Now().AddDate(0, 0, -40)); err != nil {
		t.Fatal(err)
	}
	g, err := e.store.General(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	g.HistoryDays = 30
	if err := e.store.SaveGeneral(e.ctx, g); err != nil {
		t.Fatal(err)
	}
	h = e.history(t, "tz", api.HistoryQuery{})
	if h.Counts[api.HistoryEdit] != 0 || h.Counts[api.HistoryOutage] != 2 || h.RetentionDays != 30 {
		t.Errorf("after purge: %v, %d days", h.Counts, h.RetentionDays)
	}
	if h.Since.Before(time.Now().AddDate(0, 0, -31)) {
		t.Errorf("history starts %v", h.Since)
	}
}
