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
	if applied != 5 || settingsRows != 1 {
		t.Fatalf("applied = %d, discovery_settings rows = %d", applied, settingsRows)
	}
}

func TestDiscoverySettingsIsASingleRow(t *testing.T) {
	pool := dbtest.Pool(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO discovery_settings (id) VALUES (false)"); err == nil {
		t.Fatal("a second settings row was accepted")
	}
}
