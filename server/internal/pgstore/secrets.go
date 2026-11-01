package pgstore

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"hub/server/internal/secrets"
)

var _ secrets.Repo = (*Store)(nil)

func (s *Store) SecretRow(ctx context.Context, name string) (secrets.Row, error) {
	r := secrets.Row{Name: name}
	err := s.pool.QueryRow(ctx, "SELECT key_id, ciphertext, created_at, updated_at FROM secrets WHERE name = $1", name).
		Scan(&r.KeyID, &r.Ciphertext, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return secrets.Row{}, secrets.ErrNotFound
	}
	return r, err
}

func (s *Store) PutSecretRow(ctx context.Context, r secrets.Row) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO secrets (name, key_id, ciphertext, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (name) DO UPDATE SET key_id = excluded.key_id, ciphertext = excluded.ciphertext,
			updated_at = excluded.updated_at`,
		r.Name, r.KeyID, r.Ciphertext, r.CreatedAt, r.UpdatedAt)
	return err
}

func (s *Store) DeleteSecret(ctx context.Context, name string) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM secrets WHERE name = $1", name)
	if err == nil && tag.RowsAffected() == 0 {
		return secrets.ErrNotFound
	}
	return err
}

func (s *Store) SecretInfos(ctx context.Context) ([]secrets.Info, error) {
	rows, err := s.pool.Query(ctx, "SELECT name, created_at, updated_at FROM secrets ORDER BY name")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[secrets.Info])
}

func (s *Store) SecretRowsNotSealedWith(ctx context.Context, keyID string) ([]secrets.Row, error) {
	rows, err := s.pool.Query(ctx, `SELECT name, key_id, ciphertext, created_at, updated_at
		FROM secrets WHERE key_id <> $1 ORDER BY name`, keyID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[secrets.Row])
}

// ResealSecretRow leaves updated_at alone: the value is the same, only its sealing changes.
func (s *Store) ResealSecretRow(ctx context.Context, name, oldKeyID string, r secrets.Row) (bool, error) {
	tag, err := s.pool.Exec(ctx, "UPDATE secrets SET key_id = $3, ciphertext = $4 WHERE name = $1 AND key_id = $2",
		name, oldKeyID, r.KeyID, r.Ciphertext)
	return err == nil && tag.RowsAffected() == 1, err
}
