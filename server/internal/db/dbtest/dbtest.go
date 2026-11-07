// Package dbtest gives tests a migrated database in a schema of their own, dropped at the
// end. Tests are skipped when HUB_TEST_DATABASE_URL is not set.
package dbtest

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"hub/server/internal/db"
)

func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := Empty(t)
	if err := db.Migrate(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

// Empty is Pool without the migrations: a database as a restore finds it.
func Empty(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("HUB_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("HUB_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	// The clock alone repeats itself between test packages running side by side.
	schema := fmt.Sprintf("test_%d_%d", time.Now().UnixNano(), rand.Uint64())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(ctx)
	})
	return pool
}
