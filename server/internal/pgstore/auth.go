package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"hub/server/internal/auth"
)

// The Store implements auth.Repo.
var _ auth.Repo = (*Store)(nil)

const uniqueViolation = "23505"

func (s *Store) HasUser(ctx context.Context) (bool, error) {
	var has bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users)").Scan(&has)
	return has, err
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string) (auth.User, error) {
	u := auth.User{Username: username}
	err := s.pool.QueryRow(ctx, "INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id",
		username, passwordHash).Scan(&u.ID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return auth.User{}, auth.ErrSetupDone
	}
	return u, err
}

func (s *Store) UserByName(ctx context.Context, username string) (auth.User, string, error) {
	u := auth.User{Username: username}
	var hash string
	err := s.pool.QueryRow(ctx, "SELECT id, password_hash FROM users WHERE username = $1", username).Scan(&u.ID, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, "", auth.ErrNotFound
	}
	return u, hash, err
}

func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, sess auth.Session) (auth.Session, error) {
	// Expired sessions are cleared here rather than by a background job.
	if _, err := s.pool.Exec(ctx, "DELETE FROM sessions WHERE expires_at <= $1", sess.CreatedAt); err != nil {
		return sess, err
	}
	err := s.pool.QueryRow(ctx, `INSERT INTO sessions (token_hash, user_id, user_agent, ip, created_at, last_seen_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		tokenHash, sess.UserID, sess.UserAgent, sess.IP, sess.CreatedAt, sess.LastSeenAt, sess.ExpiresAt).Scan(&sess.ID)
	return sess, err
}

const sessionColumns = "s.id, s.user_id, s.user_agent, s.ip, s.created_at, s.last_seen_at, s.expires_at"

func scanSession(row pgx.Row, extra ...any) (auth.Session, error) {
	var sess auth.Session
	dest := append([]any{&sess.ID, &sess.UserID, &sess.UserAgent, &sess.IP, &sess.CreatedAt, &sess.LastSeenAt, &sess.ExpiresAt}, extra...)
	return sess, row.Scan(dest...)
}

func (s *Store) SessionByToken(ctx context.Context, tokenHash []byte, now time.Time) (auth.Session, auth.User, error) {
	var u auth.User
	row := s.pool.QueryRow(ctx, `SELECT `+sessionColumns+`, u.id, u.username
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > $2`, tokenHash, now)
	sess, err := scanSession(row, &u.ID, &u.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Session{}, auth.User{}, auth.ErrNotFound
	}
	return sess, u, err
}

func (s *Store) TouchSession(ctx context.Context, id int64, lastSeen, expires time.Time) error {
	_, err := s.pool.Exec(ctx, "UPDATE sessions SET last_seen_at = $2, expires_at = $3 WHERE id = $1", id, lastSeen, expires)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, userID, id int64) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM sessions WHERE id = $1 AND user_id = $2", id, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return auth.ErrNotFound
	}
	return err
}

func (s *Store) Sessions(ctx context.Context, userID int64, now time.Time) ([]auth.Session, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+sessionColumns+` FROM sessions s
		WHERE s.user_id = $1 AND s.expires_at > $2 ORDER BY s.last_seen_at DESC`, userID, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (auth.Session, error) { return scanSession(row) })
}

func (s *Store) Audit(ctx context.Context, e auth.Event) error {
	_, err := s.pool.Exec(ctx, "INSERT INTO audit_events (kind, user_id, ip, detail) VALUES ($1, $2, $3, $4)",
		e.Kind, e.UserID, e.IP, e.Detail)
	return err
}
