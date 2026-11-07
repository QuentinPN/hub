package pgstore_test

import (
	"slices"
	"testing"

	"hub/server/internal/api"
)

func TestServiceScopeFollowsLinksAndRemovedContainers(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	app := e.container("tz", "tz", "org/tz:1", "running")
	db := e.container("tz-db", "postgres", "postgres:18", "running")
	e.snapshot(t, "mini", app, db)

	scope, err := e.store.ServiceScope(e.ctx, "tz")
	if err != nil || scope.HostID != "mini" || !slices.Equal(scope.Keys, []string{"tz"}) {
		t.Fatalf("scope %+v, err %v", scope, err)
	}
	if slug, ok, err := e.store.ServiceOfContainer(e.ctx, "mini", db.ID); err != nil || ok {
		t.Fatalf("unlinked database belongs to %q (%v, %v)", slug, ok, err)
	}

	if err := e.store.UpdateService(e.ctx, "tz", api.ServiceEdit{Name: "TZ", Category: "X", Links: []string{"tz-db"}}); err != nil {
		t.Fatal(err)
	}
	scope, err = e.store.ServiceScope(e.ctx, "tz")
	if err != nil || !slices.Equal(scope.Keys, []string{"tz", "tz-db"}) {
		t.Fatalf("scope %+v, err %v", scope, err)
	}

	// A removed container still names its service, for the history.
	e.snapshot(t, "mini", db)
	for _, id := range []string{app.ID, db.ID} {
		if slug, ok, err := e.store.ServiceOfContainer(e.ctx, "mini", id); err != nil || !ok || slug != "tz" {
			t.Errorf("container %s: %q, %v, %v", id[60:], slug, ok, err)
		}
	}

	for _, slug := range []string{"nope", "tz-db"} {
		if _, err := e.store.ServiceScope(e.ctx, slug); err != api.ErrNotFound {
			t.Errorf("%s: %v", slug, err)
		}
	}
	if _, ok, err := e.store.ServiceOfContainer(e.ctx, "mini", "f"+app.ID[1:]); err != nil || ok {
		t.Fatalf("unknown container: %v, %v", ok, err)
	}
}
