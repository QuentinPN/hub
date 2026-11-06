package pgstore_test

import (
	"testing"
	"time"

	"hub/server/internal/api"
	"hub/server/internal/domain"
	"hub/server/internal/urlcheck"
)

func TestURLDecidesTheStatusOnceEverythingRuns(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	emby := e.container("emby", "emby", "linuxserver/emby", "running", 8096)
	e.snapshot(t, "mini", emby)
	cases := []struct {
		r    urlcheck.Result
		want domain.Status
		note string
	}{
		{urlcheck.Result{Kind: urlcheck.KindOK, Status: 200, LatencyMs: 42}, domain.StatusOnline, "en ligne"},
		{urlcheck.Result{Kind: urlcheck.KindAuth, Status: 401, LatencyMs: 9}, domain.StatusOnline, "connexion demandée"},
		{urlcheck.Result{Kind: urlcheck.KindCert}, domain.StatusDegraded, "certificat non reconnu"},
		{urlcheck.Result{Kind: urlcheck.KindUnreachable}, domain.StatusDegraded, "adresse injoignable"},
		{urlcheck.Result{Kind: urlcheck.KindError, Status: 502}, domain.StatusDegraded, "erreur 502"},
	}
	for _, c := range cases {
		if err := e.store.SaveServiceURL(e.ctx, "emby", "http://192.0.2.14:8096", "", c.r); err != nil {
			t.Fatal(err)
		}
		svc := e.services(t)["emby"]
		if svc.Status != c.want || svc.Note != c.note || svc.URL == nil || *svc.URL != "http://192.0.2.14:8096" {
			t.Errorf("%s: %+v", c.r.Kind, svc)
		}
	}

	// The containers come first: a stopped service is not "online" because a page answers.
	if err := e.store.SaveServiceURL(e.ctx, "emby", "http://x.fr", "", urlcheck.Result{Kind: urlcheck.KindOK}); err != nil {
		t.Fatal(err)
	}
	emby.State = "exited"
	e.snapshot(t, "mini", emby)
	if svc := e.services(t)["emby"]; svc.Status != domain.StatusDown {
		t.Fatalf("stopped with a URL: %+v", svc)
	}

	if err := e.store.ClearServiceURL(e.ctx, "emby"); err != nil {
		t.Fatal(err)
	}
	emby.State = "running"
	e.snapshot(t, "mini", emby)
	if svc := e.services(t)["emby"]; svc.Status != domain.StatusConfigRequired || svc.URL != nil {
		t.Fatalf("cleared: %+v", svc)
	}
}

func TestWatchVerdictsFollowTheCurrentAddress(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini", e.container("emby", "emby", "linuxserver/emby", "running"))
	if err := e.store.SaveServiceURL(e.ctx, "emby", "http://new", "abc", urlcheck.Result{Kind: urlcheck.KindOK}); err != nil {
		t.Fatal(err)
	}
	targets, err := e.store.URLTargets(e.ctx)
	if err != nil || len(targets) != 1 || targets[0].Pinned != "abc" {
		t.Fatalf("targets %+v, err %v", targets, err)
	}
	// A check started before the address changed says nothing about the new one.
	if err := e.store.RecordURLCheck(e.ctx, urlcheck.Target{Slug: "emby", URL: "http://old"}, urlcheck.Result{Kind: urlcheck.KindUnreachable}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if svc := e.services(t)["emby"]; svc.Status != domain.StatusOnline {
		t.Fatalf("old verdict applied: %+v", svc)
	}
	if err := e.store.RecordURLCheck(e.ctx, targets[0], urlcheck.Result{Kind: urlcheck.KindUnreachable}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if svc := e.services(t)["emby"]; svc.Status != domain.StatusDegraded {
		t.Fatalf("new verdict ignored: %+v", svc)
	}
}

func TestEditSurvivesRescanAndResetGoesBack(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini", e.container("sonarr", "sonarr", "linuxserver/sonarr", "running"))
	edit := api.ServiceEdit{Name: "Mes séries", Category: "Perso", Icon: ""}
	if err := e.store.UpdateService(e.ctx, "sonarr", edit); err != nil {
		t.Fatal(err)
	}
	if err := e.store.SetFavorite(e.ctx, "sonarr", true); err != nil {
		t.Fatal(err)
	}
	e.snapshot(t, "mini", e.container("sonarr", "sonarr", "linuxserver/sonarr:4", "running"))
	svc := e.services(t)["sonarr"]
	if svc.Name != "Mes séries" || svc.Monogram != "M" || svc.Category != "Perso" || svc.Icon != nil || !svc.Favorite {
		t.Fatalf("edit lost: %+v", svc)
	}

	if err := e.store.ResetService(e.ctx, "sonarr"); err != nil {
		t.Fatal(err)
	}
	svc = e.services(t)["sonarr"]
	if svc.Name != "Sonarr" || svc.Category != "Téléchargements" || svc.Icon == nil || !svc.Favorite {
		t.Fatalf("reset: %+v", svc)
	}
	if err := e.store.SetFavorite(e.ctx, "nope", true); err != api.ErrNotFound {
		t.Fatalf("unknown slug: %v", err)
	}
}

func TestHideAndShow(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini",
		e.container("radarr", "radarr", "linuxserver/radarr", "running"),
		e.container("hawser", "hawser", "finsys/hawser", "running"),
		e.container("timescaledb", "timescaledb", "timescale/timescaledb", "running"),
	)
	hidden, err := e.store.HiddenGroups(e.ctx)
	if err != nil || len(hidden) != 2 || hidden[0].Key != "hawser" || hidden[0].Reason != "agent" || hidden[1].Reason != "database" {
		t.Fatalf("hidden %+v, err %v", hidden, err)
	}

	if err := e.store.UpdateService(e.ctx, "radarr", api.ServiceEdit{Name: "Radarr", Category: "X", Hidden: true}); err != nil {
		t.Fatal(err)
	}
	if _, listed := e.services(t)["radarr"]; listed {
		t.Fatal("hidden service still listed")
	}
	home, err := e.store.Home(e.ctx)
	if err != nil || home.Summary.Total != 0 {
		t.Fatalf("home %+v, err %v", home.Summary, err)
	}
	// A rescan does not bring it back, and its page still opens.
	e.snapshot(t, "mini", e.container("radarr", "radarr", "linuxserver/radarr", "running"))
	if d, err := e.store.Service(e.ctx, "radarr"); err != nil || !d.Hidden {
		t.Fatalf("detail %+v, err %v", d.Hidden, err)
	}
	if hidden, _ := e.store.HiddenGroups(e.ctx); len(hidden) != 0 {
		// The snapshot above removed the two others: only radarr, hidden by the user, is left.
		if hidden[0].Reason != "user" {
			t.Fatalf("hidden %+v", hidden)
		}
	}
	if slug, err := e.store.ShowHidden(e.ctx, "mini", "radarr"); err != nil || slug != "radarr" {
		t.Fatalf("show: %q, %v", slug, err)
	}

	// Showing what discovery set aside creates a service that stays across scans.
	hawser := e.container("hawser", "hawser", "finsys/hawser", "running")
	e.snapshot(t, "mini", hawser)
	slug, err := e.store.ShowHidden(e.ctx, "mini", "hawser")
	if err != nil {
		t.Fatal(err)
	}
	e.snapshot(t, "mini", hawser)
	if svc, listed := e.services(t)[slug]; !listed || svc.Status != domain.StatusConfigRequired {
		t.Fatalf("shown agent: %+v, listed %v", svc, listed)
	}
	if _, err := e.store.ShowHidden(e.ctx, "mini", "nope"); err != api.ErrNotFound {
		t.Fatalf("unknown project: %v", err)
	}
}

func TestLinkedProjectCountsAsSupport(t *testing.T) {
	e := newDiscoveryEnv(t, "mini", "qnap")
	app := e.container("tz", "tz", "org/tz:1", "running")
	db := e.container("tz-db", "postgres", "postgres:18", "running")
	e.snapshot(t, "mini", app, db, e.container("other", "other", "org/other:1", "running"))
	e.snapshot(t, "qnap", e.container("nas-db", "postgres", "postgres:18", "running"))

	d, err := e.store.Service(e.ctx, "tz")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Linkable) != 1 || d.Linkable[0].Key != "tz-db" || d.Project.Key != "tz" || d.Project.Containers != 1 {
		t.Fatalf("linkable %+v, project %+v", d.Linkable, d.Project)
	}
	for _, key := range []string{"other", "nas-db", "nope"} {
		if err := e.store.UpdateService(e.ctx, "tz", api.ServiceEdit{Name: "TZ", Category: "X", Links: []string{key}}); err != api.ErrInvalidLink {
			t.Errorf("link %q: %v", key, err)
		}
	}
	if err := e.store.UpdateService(e.ctx, "tz", api.ServiceEdit{Name: "TZ", Category: "X", Links: []string{"tz-db"}}); err != nil {
		t.Fatal(err)
	}
	if hidden, _ := e.store.HiddenGroups(e.ctx); len(hidden) != 1 || hidden[0].HostID != "qnap" {
		t.Fatalf("linked project still hidden: %+v", hidden)
	}

	db.State = "exited"
	e.snapshot(t, "mini", app, db, e.container("other", "other", "org/other:1", "running"))
	svc := e.services(t)["tz"]
	if svc.Status != domain.StatusDegraded {
		t.Fatalf("linked database stopped: %+v", svc)
	}
	d, err = e.store.Service(e.ctx, "tz")
	if err != nil || len(d.Links) != 1 || d.Links[0].Containers != 1 || len(d.Linkable) != 0 {
		t.Fatalf("links %+v, linkable %+v, err %v", d.Links, d.Linkable, err)
	}

	// Another service cannot take a project already linked.
	if err := e.store.UpdateService(e.ctx, "other", api.ServiceEdit{Name: "O", Category: "X", Links: []string{"tz-db"}}); err != api.ErrInvalidLink {
		t.Fatalf("second link: %v", err)
	}

	if err := e.store.ResetService(e.ctx, "tz"); err != nil {
		t.Fatal(err)
	}
	if svc := e.services(t)["tz"]; svc.Status != domain.StatusConfigRequired {
		t.Fatalf("after reset: %+v", svc)
	}
}
