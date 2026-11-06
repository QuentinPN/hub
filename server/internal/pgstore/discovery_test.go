package pgstore_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"hub/server/internal/agentproto"
	"hub/server/internal/agentsrv"
	"hub/server/internal/api"
	"hub/server/internal/db/dbtest"
	"hub/server/internal/domain"
	"hub/server/internal/fixture"
	"hub/server/internal/pgstore"
)

type discoveryEnv struct {
	store  *pgstore.Store
	pool   *pgxpool.Pool
	ctx    context.Context
	nextID int
}

func newDiscoveryEnv(t *testing.T, hosts ...string) *discoveryEnv {
	t.Helper()
	ctx := context.Background()
	pool := dbtest.Pool(t)
	store, err := pgstore.New(ctx, pool, fixture.Empty())
	if err != nil {
		t.Fatal(err)
	}
	for i, h := range hosts {
		if err := store.EnsureHost(ctx, h, h); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(h))
		cert := agentsrv.CertRecord{Serial: fmt.Sprintf("a%d", i), HostID: h, Fingerprint: sum[:],
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour)}
		if err := store.AddAgentCert(ctx, cert); err != nil {
			t.Fatal(err)
		}
	}
	return &discoveryEnv{store: store, pool: pool, ctx: ctx}
}

// container builds a Compose container with a fresh id, like a recreation does.
func (e *discoveryEnv) container(project, service, image, state string, ports ...int) agentproto.Container {
	e.nextID++
	c := agentproto.Container{
		ID:    strings.Repeat("0", 60) + fmt.Sprintf("%04x", e.nextID),
		Name:  project + "-" + service,
		Image: image,
		State: state,
		Labels: map[string]string{
			agentproto.LabelComposeProject: project,
			agentproto.LabelComposeService: service,
		},
	}
	for _, p := range ports {
		c.Ports = append(c.Ports, agentproto.Port{PublicPort: p, PrivatePort: p, Type: "tcp"})
	}
	return c
}

func (e *discoveryEnv) snapshot(t *testing.T, host string, containers ...agentproto.Container) {
	t.Helper()
	e.snapshotAt(t, host, time.Now(), containers...)
}

func (e *discoveryEnv) snapshotAt(t *testing.T, host string, at time.Time, containers ...agentproto.Container) {
	t.Helper()
	if err := e.store.ReplaceContainers(e.ctx, host, agentproto.Snapshot{Containers: containers}, at); err != nil {
		t.Fatal(err)
	}
}

func (e *discoveryEnv) services(t *testing.T) map[string]api.ServiceSummary {
	t.Helper()
	list, err := e.store.Services(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	bySlug := map[string]api.ServiceSummary{}
	for _, s := range list {
		bySlug[s.Slug] = s
	}
	return bySlug
}

func TestSnapshotCreatesServicesAndHidesTheRest(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini",
		e.container("media", "emby", "lscr.io/linuxserver/emby:latest", "running", 8096, 8096, 8920),
		e.container("media", "gluetun", "qmcgaw/gluetun:latest", "running"),
		e.container("hub-agent", "agent", "hub-agent:local", "running"),
		e.container("timescaledb", "timescaledb", "timescale/timescaledb:latest-pg17", "running"),
		e.container("hub", "hub", "hub-server:local", "running", 8443),
	)
	got := e.services(t)
	if len(got) != 1 {
		t.Fatalf("got %v, want only emby", got)
	}
	emby := got["emby"]
	if emby.Name != "Emby" || emby.Category != "Médias" || emby.HostID != "mini" || emby.Icon == nil || *emby.Icon != "emby" {
		t.Fatalf("emby: %+v", emby)
	}
	if emby.Status != domain.StatusConfigRequired || emby.Note != "ports 8096, 8920" {
		t.Fatalf("status %q, note %q", emby.Status, emby.Note)
	}

	detail, err := e.store.Service(e.ctx, "emby")
	if err != nil {
		t.Fatal(err)
	}
	access := map[string]string{}
	for _, kv := range detail.Access {
		access[kv.Label] = kv.Value
	}
	if access["media-emby"] != "en cours · principal" || access["media-gluetun"] != "en cours" || access["Ports publiés"] != "8096, 8920" || access["URL"] != "à renseigner" {
		t.Fatalf("access: %+v", detail.Access)
	}
	if detail.Stats == nil || detail.Upcoming == nil || detail.Connection == nil {
		t.Fatalf("the browser relies on lists, never null: %+v", detail)
	}
}

func TestRescanKeepsCustomizationsAndSurvivesRecreation(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshot(t, "mini", e.container("sonarr", "sonarr", "linuxserver/sonarr", "running"))
	if _, err := e.pool.Exec(e.ctx, `UPDATE services SET name = 'Mes séries', category = 'Perso', favorite = true, icon = NULL WHERE slug = 'sonarr'`); err != nil {
		t.Fatal(err)
	}
	// A recreated container has a new id: same project, same service.
	e.snapshot(t, "mini", e.container("sonarr", "sonarr", "linuxserver/sonarr:4", "running"))
	got := e.services(t)
	svc := got["sonarr"]
	if len(got) != 1 || svc.Name != "Mes séries" || svc.Category != "Perso" || !svc.Favorite || svc.Icon != nil {
		t.Fatalf("customization lost: %+v", got)
	}
}

func TestSameApplicationOnTwoHosts(t *testing.T) {
	e := newDiscoveryEnv(t, "mini", "qnap")
	e.snapshot(t, "mini", e.container("radarr", "radarr", "linuxserver/radarr", "running"))
	e.snapshot(t, "qnap", e.container("radarr", "radarr", "linuxserver/radarr", "exited"))
	got := e.services(t)
	if got["radarr"].HostID != "mini" || got["radarr-qnap"].HostID != "qnap" {
		t.Fatalf("got %+v", got)
	}
	if got["radarr"].Status != domain.StatusConfigRequired || got["radarr-qnap"].Status != domain.StatusDown {
		t.Fatalf("statuses mixed up: %+v", got)
	}
}

func TestEventsUpdateStatusAndServices(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	app := e.container("app", "app", "org/app:1", "running")
	worker := e.container("app", "worker", "org/app:1", "running")
	e.snapshot(t, "mini", app, worker)

	worker.State = "exited"
	if err := e.store.ApplyContainerChanges(e.ctx, "mini", agentproto.Changes{Upserted: []agentproto.Container{worker}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if svc := e.services(t)["app"]; svc.Status != domain.StatusDegraded {
		t.Fatalf("worker stopped: %+v", svc)
	}

	other := e.container("other", "other", "org/other:1", "running")
	if err := e.store.ApplyContainerChanges(e.ctx, "mini", agentproto.Changes{Upserted: []agentproto.Container{other}, Removed: []string{app.ID, worker.ID}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	got := e.services(t)
	if got["other"].Name != "Other" || got["other"].Category != "Autres" {
		t.Fatalf("new project: %+v", got["other"])
	}
	if got["app"].Status != domain.StatusDown || got["app"].Note != "conteneurs absents" {
		t.Fatalf("removed project: %+v", got["app"])
	}
}

func TestRemovalFollowsTheSettings(t *testing.T) {
	cases := []struct {
		onRemoval   string
		afterADay   bool
		afterAMonth bool
	}{
		{"absent_then_archive", true, false},
		{"absent_only", true, true},
		{"archive_now", false, false},
	}
	for _, c := range cases {
		t.Run(c.onRemoval, func(t *testing.T) {
			e := newDiscoveryEnv(t, "mini")
			if _, err := e.pool.Exec(e.ctx, "UPDATE discovery_settings SET on_removal = $1, archive_after_days = 7", c.onRemoval); err != nil {
				t.Fatal(err)
			}
			start := time.Now().Add(-30 * 24 * time.Hour)
			e.snapshotAt(t, "mini", start, e.container("radarr", "radarr", "linuxserver/radarr", "running"))
			if _, err := e.pool.Exec(e.ctx, "UPDATE services SET favorite = true"); err != nil {
				t.Fatal(err)
			}

			e.snapshotAt(t, "mini", start.Add(24*time.Hour))
			if _, ok := e.services(t)["radarr"]; ok != c.afterADay {
				t.Fatalf("after a day: listed = %v", ok)
			}
			e.snapshotAt(t, "mini", start.Add(29*24*time.Hour))
			if _, ok := e.services(t)["radarr"]; ok != c.afterAMonth {
				t.Fatalf("after a month: listed = %v", ok)
			}

			// Back again: the same service, with its favorite.
			e.snapshot(t, "mini", e.container("radarr", "radarr", "linuxserver/radarr", "running"))
			svc, ok := e.services(t)["radarr"]
			if !ok || !svc.Favorite || svc.Status != domain.StatusConfigRequired {
				t.Fatalf("back: %+v", svc)
			}
		})
	}
}

func TestSilentHostReadsUnknown(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	e.snapshotAt(t, "mini", time.Now().Add(-time.Hour), e.container("radarr", "radarr", "linuxserver/radarr", "exited"))
	svc := e.services(t)["radarr"]
	if svc.Status != domain.StatusUnknown || svc.Note != "hôte injoignable" {
		t.Fatalf("got %+v", svc)
	}
}

func TestHomeWithoutFavoritesListsServices(t *testing.T) {
	e := newDiscoveryEnv(t, "mini", "pi")
	e.snapshot(t, "mini",
		e.container("radarr", "radarr", "linuxserver/radarr", "running"),
		e.container("sonarr", "sonarr", "linuxserver/sonarr", "exited"),
	)
	e.snapshotAt(t, "pi", time.Now().Add(-10*time.Minute), e.container("hawser", "hawser", "finsys/hawser", "running"))

	home, err := e.store.Home(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(home.Favorites) != 0 || len(home.Services) != 2 {
		t.Fatalf("favorites %d, services %d", len(home.Favorites), len(home.Services))
	}
	// Running without a URL yet still counts as running.
	if home.Summary.Total != 2 || home.Summary.Online != 1 {
		t.Fatalf("summary %+v", home.Summary)
	}
	if len(home.Hosts) != 2 || home.Hosts[0].ID != "mini" || !home.Hosts[0].Reachable || home.Hosts[0].Containers != 2 {
		t.Fatalf("hosts %+v", home.Hosts)
	}
	if pi := home.Hosts[1]; pi.Reachable || pi.LastSeenLabel != "il y a 10 min" || pi.Metrics == nil {
		t.Fatalf("pi %+v", pi)
	}

	if _, err := e.pool.Exec(e.ctx, "UPDATE services SET favorite = true WHERE slug = 'radarr'"); err != nil {
		t.Fatal(err)
	}
	home, err = e.store.Home(e.ctx)
	if err != nil || len(home.Favorites) != 1 || len(home.Services) != 0 {
		t.Fatalf("with a favorite: %+v, err %v", home, err)
	}
}
