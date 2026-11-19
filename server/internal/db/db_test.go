package db_test

import (
	"context"
	"testing"

	"hub/server/internal/db"
	"hub/server/internal/db/dbtest"
)

func TestMigrateIsIdempotent(t *testing.T) {
	pool := dbtest.Pool(t) // already migrated once
	ctx := context.Background()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var applied, settingsRows int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM discovery_settings").Scan(&settingsRows); err != nil {
		t.Fatal(err)
	}
	_, expected, err := db.MigrationStatus(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if applied != expected || settingsRows != 1 {
		t.Fatalf("applied = %d, discovery_settings rows = %d", applied, settingsRows)
	}
}

func TestDiscoverySettingsIsASingleRow(t *testing.T) {
	pool := dbtest.Pool(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO discovery_settings (id) VALUES (false)"); err == nil {
		t.Fatal("a second settings row was accepted")
	}
}

func TestCanActIsBackfilledFromStoredWidgetData(t *testing.T) {
	pool := dbtest.Empty(t)
	ctx := context.Background()
	if err := db.MigrateTo(ctx, pool, 15); err != nil {
		t.Fatal(err)
	}
	// The services these rows belong to are beside the point here.
	if _, err := pool.Exec(ctx, `ALTER TABLE widget_connections DROP CONSTRAINT widget_connections_service_slug_fkey`); err != nil {
		t.Fatal(err)
	}
	for _, s := range []struct{ slug, kind, data string }{
		{"ro", "request", `{"requests":{"canAct":false}}`},
		{"rw", "request", `{"requests":{"canAct":true}}`},
		{"none", "request", `{}`},
		{"other", "sonarr", `{"requests":{"canAct":false}}`},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO widget_connections (service_slug, kind, url, enabled, refresh_seconds, state, data)
			VALUES ($1, $2, 'http://x', '{}', 60, 'ok', $3::jsonb)`, s.slug, s.kind, s.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	for slug, want := range map[string]string{"ro": "false", "rw": "true", "none": "NULL", "other": "NULL"} {
		var got string
		if err := pool.QueryRow(ctx, `SELECT coalesce(can_act::text, 'NULL') FROM widget_connections WHERE service_slug = $1`, slug).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s: can_act = %s, want %s", slug, got, want)
		}
	}
}
