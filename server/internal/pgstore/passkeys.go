package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"hub/server/internal/auth"
)

var _ auth.PasskeyRepo = (*Store)(nil)

func (s *Store) PasswordHash(ctx context.Context, userID int64) (string, error) {
	var hash string
	err := s.pool.QueryRow(ctx, "SELECT password_hash FROM users WHERE id = $1", userID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrNotFound
	}
	return hash, err
}

func (s *Store) UserHandle(ctx context.Context, userID int64) ([]byte, error) {
	var handle []byte
	err := s.pool.QueryRow(ctx, "SELECT webauthn_handle FROM users WHERE id = $1", userID).Scan(&handle)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, auth.ErrNotFound
	}
	return handle, err
}

func (s *Store) UserByHandle(ctx context.Context, handle []byte) (auth.User, error) {
	var u auth.User
	err := s.pool.QueryRow(ctx, "SELECT id, username FROM users WHERE webauthn_handle = $1", handle).Scan(&u.ID, &u.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, auth.ErrNotFound
	}
	return u, err
}

func (s *Store) Passkeys(ctx context.Context, userID int64) ([]auth.Passkey, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, user_id, name, created_at, last_used_at, credential
		FROM passkeys WHERE user_id = $1 ORDER BY created_at, id`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (auth.Passkey, error) {
		var p auth.Passkey
		err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt, &p.LastUsedAt, &p.Credential)
		return p, err
	})
}

func (s *Store) CreatePasskey(ctx context.Context, p auth.Passkey) (auth.Passkey, error) {
	err := s.pool.QueryRow(ctx, `INSERT INTO passkeys (user_id, credential_id, name, credential, created_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		p.UserID, p.Credential.ID, p.Name, p.Credential, p.CreatedAt).Scan(&p.ID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return auth.Passkey{}, auth.ErrPasskeyExists
	}
	return p, err
}

func (s *Store) UsePasskey(ctx context.Context, id int64, cred webauthn.Credential, usedAt time.Time) error {
	_, err := s.pool.Exec(ctx, "UPDATE passkeys SET credential = $2, last_used_at = $3 WHERE id = $1", id, cred, usedAt)
	return err
}

func (s *Store) DeletePasskey(ctx context.Context, userID, id int64) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM passkeys WHERE id = $1 AND user_id = $2", id, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return auth.ErrNotFound
	}
	return err
}
