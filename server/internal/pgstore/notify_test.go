package pgstore_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"hub/server/internal/auth"
	"hub/server/internal/db/dbtest"
	"hub/server/internal/fixture"
	"hub/server/internal/notify"
	"hub/server/internal/pgstore"
)

func TestNotifyStore(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Pool(t)
	store, err := pgstore.New(ctx, pool, fixture.Empty())
	if err != nil {
		t.Fatal(err)
	}

	set, err := store.NotifySettings(ctx)
	if err != nil || set.NtfyURL != "" || !set.QuietHours || set.SMTPPort != 587 || set.LastSession != nil {
		t.Fatalf("defaults %+v %v", set, err)
	}
	set.NtfyURL, set.NtfyHint, set.Alerts = "https://ntfy.sh", "hub-", map[string]bool{notify.AlertDisk: true}
	set.SMTPHost, set.SMTPPort, set.SMTPFrom, set.SMTPTo = "smtp.exemple.fr", 465, "hub@exemple.fr", "moi@exemple.fr"
	if err := store.SaveNotifySettings(ctx, set); err != nil {
		t.Fatal(err)
	}
	if err := store.SetNotifyLastSession(ctx, 42); err != nil {
		t.Fatal(err)
	}
	// Saving the settings again leaves the engine's cursor alone.
	set.QuietHours = false
	if err := store.SaveNotifySettings(ctx, set); err != nil {
		t.Fatal(err)
	}
	got, err := store.NotifySettings(ctx)
	if err != nil || got.NtfyURL != "https://ntfy.sh" || got.SMTPPort != 465 || !got.Alerts[notify.AlertDisk] || got.QuietHours ||
		got.LastSession == nil || *got.LastSession != 42 {
		t.Fatalf("read back %+v %v", got, err)
	}

	now := time.Now().Truncate(time.Microsecond)
	if err := store.SaveChannelState(ctx, notify.Ntfy, notify.Send{At: now, OK: false, Error: "serveur injoignable"}); err != nil {
		t.Fatal(err)
	}
	states, err := store.ChannelStates(ctx)
	if err != nil || states[notify.Ntfy].Error != "serveur injoignable" || !states[notify.Ntfy].At.Equal(now) {
		t.Fatalf("states %+v %v", states, err)
	}
	if err := store.DeleteChannelState(ctx, notify.Ntfy); err != nil {
		t.Fatal(err)
	}

	inc := notify.Incident{Key: "service:jellyfin", Alert: notify.AlertService, Subject: "Jellyfin", Since: now}
	if err := store.PutIncident(ctx, inc); err != nil {
		t.Fatal(err)
	}
	inc.NotifiedAt, inc.Detail, inc.Since = &now, "conteneurs arrêtés", now.Add(time.Hour)
	if err := store.PutIncident(ctx, inc); err != nil {
		t.Fatal(err)
	}
	list, err := store.Incidents(ctx)
	if err != nil || len(list) != 1 || list[0].NotifiedAt == nil || !list[0].Since.Equal(now) || list[0].Detail != "conteneurs arrêtés" {
		t.Fatalf("incidents %+v %v", list, err)
	}
	if err := store.DeleteIncident(ctx, inc.Key); err != nil {
		t.Fatal(err)
	}

	for _, title := range []string{"un", "deux", "trois"} {
		if err := store.AddDigest(ctx, notify.DigestLine{At: now, Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	lines, _ := store.Digest(ctx)
	if err := store.ClearDigest(ctx, lines[1].ID); err != nil {
		t.Fatal(err)
	}
	if lines, _ = store.Digest(ctx); len(lines) != 1 || lines[0].Title != "trois" {
		t.Fatalf("digest %+v", lines)
	}

	if isNew, err := store.LearnDevice(ctx, "Firefox sur Windows", now); err != nil || !isNew {
		t.Fatalf("first %v %v", isNew, err)
	}
	if isNew, _ := store.LearnDevice(ctx, "Firefox sur Windows", now); isNew {
		t.Fatal("learnt twice")
	}
}

func TestNotifySessionsAndConditions(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Pool(t)
	store, err := pgstore.New(ctx, pool, fixture.Empty())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := auth.New(store)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := svc.PrepareSetup(ctx)
	if _, err := svc.Setup(ctx, code, "quentin", "correct horse battery", auth.Client{IP: "192.0.2.50", UserAgent: "Firefox/140.0 (Windows)"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(ctx, "quentin", "correct horse battery", auth.Client{IP: "192.0.2.51", UserAgent: "iPhone Safari/604.1"}); err != nil {
		t.Fatal(err)
	}
	all, err := store.SessionsAfter(ctx, nil)
	if err != nil || len(all) != 2 || all[1].UserAgent != "iPhone Safari/604.1" {
		t.Fatalf("sessions %+v %v", all, err)
	}
	after, err := store.SessionsAfter(ctx, &all[0].ID)
	if err != nil || len(after) != 1 || after[0].ID != all[1].ID {
		t.Fatalf("after %+v %v", after, err)
	}

	snap, err := store.NotifyConditions(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Nothing enrolled, nothing connected: nothing fails, and the disks are unknown.
	if len(snap.Failing) != 0 || !slices.Contains(snap.Held, "disk:") {
		t.Fatalf("snapshot %+v", snap)
	}
}
