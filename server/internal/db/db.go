// Package db opens the PostgreSQL pool and applies the embedded SQL migrations.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

// migrationLock is an arbitrary key for pg_advisory_xact_lock, so two server instances
// starting at once never apply the same migration twice.
const migrationLock = 0x68756273

// Open connects and checks the database answers.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return pool, nil
}

type migration struct {
	version int
	name    string
}

func list() ([]migration, error) {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	out := make([]migration, 0, len(entries))
	for _, e := range entries {
		prefix, _, ok := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(prefix)
		if !ok || err != nil {
			return nil, fmt.Errorf("migration %q: name must start with a number and an underscore", e.Name())
		}
		out = append(out, migration{version: v, name: e.Name()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// MigrationStatus counts the migrations applied and those the binary knows. It also proves
// the database answers.
func MigrationStatus(ctx context.Context, pool *pgxpool.Pool) (applied, expected int, err error) {
	all, err := list()
	if err != nil {
		return 0, 0, err
	}
	err = pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&applied)
	return applied, len(all), err
}

// Migrate applies, in order and each in its own transaction, the migrations not yet
// recorded in schema_migrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	all, err := list()
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    integer PRIMARY KEY,
		name       text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	for _, m := range all {
		if err := apply(ctx, pool, m); err != nil {
			return err
		}
	}
	return nil
}

func apply(ctx context.Context, pool *pgxpool.Pool, m migration) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrationLock); err != nil {
			return err
		}
		var done bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", m.version).Scan(&done); err != nil {
			return err
		}
		if done {
			return nil
		}
		sql, err := migrations.ReadFile("migrations/" + m.name)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, name) VALUES ($1, $2)", m.version, m.name); err != nil {
			return err
		}
		slog.Info("migration applied", "name", m.name)
		return nil
	})
}
