package pgstore_test

import (
	"errors"
	"testing"
	"time"

	"hub/server/internal/api"
	"hub/server/internal/domain"
)

func TestSplitServiceSurvivesRescanAndJoinsBack(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	admin := e.container("anime", "admin", "org/anime-admin:1.2.0", "running", 5178)
	web := e.container("anime", "web", "org/anime-web:1.2.0", "running")
	e.snapshot(t, "mini", admin, web)
	if _, ok := e.services(t)["anime"]; !ok || len(e.services(t)) != 1 {
		t.Fatalf("before: %+v", e.services(t))
	}

	slug, err := e.store.SplitService(e.ctx, "anime", api.ServiceSplit{Service: "web", Name: "Anime Web", Category: "Mes applications"})
	if err != nil || slug != "anime-web" {
		t.Fatalf("split: %q, %v", slug, err)
	}
	svc := e.services(t)["anime-web"]
	if svc.Name != "Anime Web" || svc.Category != "Mes applications" || svc.Status != domain.StatusConfigRequired {
		t.Fatalf("split service: %+v", svc)
	}

	parent, err := e.store.ServiceContainers(e.ctx, "anime", time.Now())
	if err != nil || len(parent.Containers) != 1 || parent.Containers[0].Name != "anime-admin" || parent.Split != nil {
		t.Fatalf("parent containers: %+v, %v", parent, err)
	}
	own, err := e.store.ServiceContainers(e.ctx, "anime-web", time.Now())
	if err != nil || len(own.Containers) != 1 || own.Containers[0].Role != "main" || own.Containers[0].Project != "anime/web" {
		t.Fatalf("split containers: %+v, %v", own, err)
	}
	if sp := own.Split; sp == nil || sp.Project != "anime" || sp.Service != "web" || sp.ParentSlug != "anime" || sp.ParentName != "Anime" {
		t.Fatalf("split info: %+v", own.Split)
	}

	// A recreated container stays apart, and its service keeps the user's name.
	web2 := e.container("anime", "web", "org/anime-web:1.3.0", "exited")
	e.snapshot(t, "mini", admin, web2)
	list := e.services(t)
	if list["anime-web"].Name != "Anime Web" || list["anime-web"].Status != domain.StatusDown {
		t.Fatalf("after recreation: %+v", list["anime-web"])
	}
	if list["anime"].Status != domain.StatusConfigRequired {
		t.Fatalf("the project no longer counts the split container: %+v", list["anime"])
	}

	refused := []struct{ slug, service string }{
		{"anime", "admin"},   // the project would be left with nothing
		{"anime", "nope"},    // not one of its Compose services
		{"anime-web", "web"}, // already split
		{"missing", "web"},   // no such service
	}
	for _, r := range refused {
		_, err := e.store.SplitService(e.ctx, r.slug, api.ServiceSplit{Service: r.service, Name: "X", Category: "Y"})
		if !errors.Is(err, api.ErrInvalidSplit) && !errors.Is(err, api.ErrNotFound) {
			t.Errorf("split %s/%s: %v", r.slug, r.service, err)
		}
	}
	if _, err := e.store.JoinService(e.ctx, "anime"); !errors.Is(err, api.ErrInvalidSplit) {
		t.Fatalf("join a project's own service: %v", err)
	}

	back, err := e.store.JoinService(e.ctx, "anime-web")
	if err != nil || back != "anime" {
		t.Fatalf("join: %q, %v", back, err)
	}
	if _, ok := e.services(t)["anime-web"]; ok {
		t.Fatal("joined service still listed")
	}
	parent, err = e.store.ServiceContainers(e.ctx, "anime", time.Now())
	if err != nil || len(parent.Containers) != 2 {
		t.Fatalf("parent after join: %+v, %v", parent, err)
	}
	e.snapshot(t, "mini", admin, web2)
	if _, ok := e.services(t)["anime-web"]; ok {
		t.Fatal("a rescan brought the joined service back")
	}

	// Splitting again brings the same service back, renamed.
	slug, err = e.store.SplitService(e.ctx, "anime", api.ServiceSplit{Service: "web", Name: "Site", Category: "Mes applications"})
	if err != nil || slug != "anime-web" || e.services(t)["anime-web"].Name != "Site" {
		t.Fatalf("split again: %q, %v, %+v", slug, err, e.services(t)["anime-web"])
	}
}

func TestSplitIgnoresOneShotTasks(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	app := e.container("tz", "tz", "org/tz:1", "running")
	app.Labels["com.docker.compose.depends_on"] = "migrate:service_completed_successfully:false"
	migrate := e.container("tz", "migrate", "org/tz:1", "exited")
	e.snapshot(t, "mini", app, migrate)
	for _, service := range []string{"tz", "migrate"} {
		if _, err := e.store.SplitService(e.ctx, "tz", api.ServiceSplit{Service: service, Name: "X", Category: "Y"}); !errors.Is(err, api.ErrInvalidSplit) {
			t.Errorf("split %s: %v", service, err)
		}
	}
}
