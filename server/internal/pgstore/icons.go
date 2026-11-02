package pgstore

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"hub/server/internal/icons"
)

var _ icons.Repo = (*Store)(nil)

func (s *Store) CachedIcon(ctx context.Context, ref string) (icons.CachedIcon, error) {
	c := icons.CachedIcon{Ref: ref}
	var contentType *string
	var body []byte
	err := s.pool.QueryRow(ctx, "SELECT content_type, body, fetched_at FROM icon_cache WHERE ref = $1", ref).
		Scan(&contentType, &body, &c.FetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return icons.CachedIcon{}, icons.ErrNotFound
	}
	if err == nil && contentType != nil {
		c.Icon = &icons.Icon{ContentType: *contentType, Body: body}
	}
	return c, err
}

func (s *Store) PutCachedIcon(ctx context.Context, c icons.CachedIcon) error {
	var contentType *string
	var body []byte
	if c.Icon != nil {
		contentType, body = &c.Icon.ContentType, c.Icon.Body
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO icon_cache (ref, content_type, body, fetched_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (ref) DO UPDATE SET content_type = excluded.content_type, body = excluded.body,
			fetched_at = excluded.fetched_at`,
		c.Ref, contentType, body, c.FetchedAt)
	return err
}
