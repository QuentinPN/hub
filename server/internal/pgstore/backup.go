package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"hub/server/internal/backup"
)

var _ backup.Store = (*Store)(nil)

// BackupSettings reads the settings; without a row, the defaults and no key.
func (s *Store) BackupSettings(ctx context.Context) (backup.Settings, error) {
	out := backup.Settings{Enabled: true, Frequency: backup.Daily, Retention: backup.Keep7d4w}
	var recipient *string
	var sealed []byte
	err := s.pool.QueryRow(ctx, `SELECT enabled, frequency, retention, recipient, sealed_key, key_set_at, updated_at
		FROM backup_settings`).Scan(&out.Enabled, &out.Frequency, &out.Retention, &recipient, &sealed, &out.KeySetAt, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if recipient != nil {
		out.Key = &backup.Key{Recipient: *recipient, Sealed: sealed}
	}
	return out, nil
}

func (s *Store) SaveBackupSchedule(ctx context.Context, enabled bool, frequency, retention string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO backup_settings (enabled, frequency, retention) VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET enabled = excluded.enabled, frequency = excluded.frequency,
			retention = excluded.retention, updated_at = now()`, enabled, frequency, retention)
	return err
}

func (s *Store) SaveBackupKey(ctx context.Context, key backup.Key) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO backup_settings (recipient, sealed_key, key_set_at) VALUES ($1, $2, now())
		ON CONFLICT (id) DO UPDATE SET recipient = excluded.recipient, sealed_key = excluded.sealed_key,
			key_set_at = excluded.key_set_at`, key.Recipient, key.Sealed)
	return err
}

func (s *Store) StartBackupRun(ctx context.Context, manual bool, at time.Time) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, "INSERT INTO backup_runs (manual, started_at) VALUES ($1, $2) RETURNING id", manual, at).Scan(&id)
	return id, err
}

func (s *Store) FinishBackupRun(ctx context.Context, r backup.Run) error {
	_, err := s.pool.Exec(ctx, `UPDATE backup_runs SET finished_at = $2, ok = $3, file = $4, size_bytes = $5, error = $6
		WHERE id = $1`, r.ID, r.FinishedAt, r.OK, r.File, r.Size, r.Error)
	return err
}

// BackupRuns returns the newest runs first. A run Hub stopped in the middle of stays
// unfinished: it shows as failed.
func (s *Store) BackupRuns(ctx context.Context, limit int) ([]backup.Run, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, started_at, finished_at, manual, ok, file, size_bytes, error
		FROM backup_runs ORDER BY started_at DESC, id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (backup.Run, error) {
		var r backup.Run
		err := row.Scan(&r.ID, &r.StartedAt, &r.FinishedAt, &r.Manual, &r.OK, &r.File, &r.Size, &r.Error)
		return r, err
	})
}

// Pool is the database the backups read from.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }
