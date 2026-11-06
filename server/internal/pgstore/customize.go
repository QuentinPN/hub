package pgstore

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"hub/server/internal/api"
	"hub/server/internal/reconcile"
	"hub/server/internal/urlcheck"
)

var (
	_ api.ServiceEditor = (*Store)(nil)
	_ urlcheck.Source   = (*Store)(nil)
)

func notFoundIfNone(affected int64, err error) error {
	if err == nil && affected == 0 {
		return api.ErrNotFound
	}
	return err
}

func (s *Store) SetFavorite(ctx context.Context, slug string, favorite bool) error {
	tag, err := s.pool.Exec(ctx, "UPDATE services SET favorite = $2, updated_at = now() WHERE slug = $1 AND archived_at IS NULL", slug, favorite)
	return notFoundIfNone(tag.RowsAffected(), err)
}

// UpdateService applies the panel's values, links included, as one change. Links must be
// projects of the service's host with no service of their own, or already its own.
func (s *Store) UpdateService(ctx context.Context, slug string, edit api.ServiceEdit) error {
	var hostID string
	err := s.pool.QueryRow(ctx, "SELECT host_id FROM services WHERE slug = $1 AND archived_at IS NULL", slug).Scan(&hostID)
	if errors.Is(err, pgx.ErrNoRows) {
		return api.ErrNotFound
	}
	if err != nil {
		return err
	}
	if len(edit.Links) > 0 {
		free, err := s.linkable(ctx, hostID, slug)
		if err != nil {
			return err
		}
		for _, key := range edit.Links {
			if !slices.ContainsFunc(free, func(p api.ProjectRef) bool { return p.Key == key }) {
				return api.ErrInvalidLink
			}
		}
	}
	var icon *string
	if edit.Icon != "" {
		icon = &edit.Icon
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE services SET name = $2, monogram = $3, category = $4, icon = $5, hidden = $6, updated_at = now()
			WHERE slug = $1`, slug, edit.Name, reconcile.Monogram(edit.Name), edit.Category, icon, edit.Hidden); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM service_links WHERE service_slug = $1", slug); err != nil {
			return err
		}
		for _, key := range edit.Links {
			if _, err := tx.Exec(ctx, "INSERT INTO service_links (service_slug, host_id, discovery_key) VALUES ($1, $2, $3)", slug, hostID, key); err != nil {
				return err
			}
		}
		return nil
	})
}

// ResetService goes back to what discovery found. The address and the favorite are not
// corrections of what it found: they stay.
func (s *Store) ResetService(ctx context.Context, slug string) error {
	var hostID string
	var key *string
	err := s.pool.QueryRow(ctx, "SELECT host_id, discovery_key FROM services WHERE slug = $1 AND archived_at IS NULL", slug).Scan(&hostID, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return api.ErrNotFound
	}
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM service_links WHERE service_slug = $1", slug); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE services SET hidden = false, updated_at = now() WHERE slug = $1", slug); err != nil {
			return err
		}
		if key == nil {
			return nil
		}
		byHost, err := liveContainers(ctx, tx, hostID)
		if err != nil {
			return err
		}
		for _, g := range reconcile.Groups(byHost[hostID]) {
			if g.Key == *key {
				_, err := tx.Exec(ctx, `UPDATE services SET name = $2, monogram = $3, icon = $4, color = $5, category = $6 WHERE slug = $1`,
					slug, g.Name, g.Monogram, g.Icon, g.Color, g.Category)
				return err
			}
		}
		return nil
	})
}

func (s *Store) SaveServiceURL(ctx context.Context, slug, url, certSHA256 string, r urlcheck.Result) error {
	tag, err := s.pool.Exec(ctx, `UPDATE services SET url = $2, url_cert_sha256 = $3, url_result = $4, url_status = $5,
			url_latency_ms = $6, url_checked_at = now(), updated_at = now()
		WHERE slug = $1 AND archived_at IS NULL`, slug, url, certSHA256, string(r.Kind), r.Status, r.LatencyMs)
	return notFoundIfNone(tag.RowsAffected(), err)
}

func (s *Store) ClearServiceURL(ctx context.Context, slug string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE services SET url = NULL, url_cert_sha256 = '', url_result = '', url_status = 0,
			url_latency_ms = 0, url_checked_at = NULL, updated_at = now()
		WHERE slug = $1 AND archived_at IS NULL`, slug)
	return notFoundIfNone(tag.RowsAffected(), err)
}

func (s *Store) URLTargets(ctx context.Context) ([]urlcheck.Target, error) {
	rows, err := s.pool.Query(ctx, "SELECT slug, url, url_cert_sha256 FROM services WHERE url IS NOT NULL AND archived_at IS NULL")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (urlcheck.Target, error) {
		var t urlcheck.Target
		return t, row.Scan(&t.Slug, &t.URL, &t.Pinned)
	})
}

// RecordURLCheck ignores a verdict on an address changed since the check began.
func (s *Store) RecordURLCheck(ctx context.Context, t urlcheck.Target, r urlcheck.Result, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE services SET url_result = $3, url_status = $4, url_latency_ms = $5, url_checked_at = $6
		WHERE slug = $1 AND url = $2`, t.Slug, t.URL, string(r.Kind), r.Status, r.LatencyMs, at)
	return err
}

// knownProjects reads, per host, the projects that have a service or are linked to one.
func (s *Store) knownProjects(ctx context.Context) (map[[2]string]string, map[[2]string]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT host_id, discovery_key, slug FROM services
		WHERE discovery_key IS NOT NULL AND NOT hidden AND archived_at IS NULL`)
	if err != nil {
		return nil, nil, err
	}
	visible := map[[2]string]string{}
	var host, key, slug string
	if _, err := pgx.ForEachRow(rows, []any{&host, &key, &slug}, func() error {
		visible[[2]string{host, key}] = slug
		return nil
	}); err != nil {
		return nil, nil, err
	}
	rows, err = s.pool.Query(ctx, `SELECT host_id, discovery_key FROM service_links`)
	if err != nil {
		return nil, nil, err
	}
	linked := map[[2]string]bool{}
	if _, err := pgx.ForEachRow(rows, []any{&host, &key}, func() error {
		linked[[2]string{host, key}] = true
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return visible, linked, nil
}

// HiddenGroups lists what the catalogue leaves out: what discovery set aside, and what the
// user hid. A project linked to a service is part of that service.
func (s *Store) HiddenGroups(ctx context.Context) ([]api.HiddenGroup, error) {
	byHost, err := liveContainers(ctx, s.pool, "")
	if err != nil {
		return nil, err
	}
	visible, linked, err := s.knownProjects(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT host_id, discovery_key FROM services
		WHERE discovery_key IS NOT NULL AND hidden AND archived_at IS NULL`)
	if err != nil {
		return nil, err
	}
	userHidden := map[[2]string]bool{}
	var host, key string
	if _, err := pgx.ForEachRow(rows, []any{&host, &key}, func() error {
		userHidden[[2]string{host, key}] = true
		return nil
	}); err != nil {
		return nil, err
	}
	list := []api.HiddenGroup{}
	for host, containers := range byHost {
		for _, g := range reconcile.Groups(containers) {
			id := [2]string{host, g.Key}
			if _, shown := visible[id]; shown || linked[id] {
				continue
			}
			reason := string(g.Kind)
			if userHidden[id] {
				reason = "user"
			} else if g.Kind == reconcile.KindService {
				continue // no service yet: the next scan creates it
			}
			list = append(list, api.HiddenGroup{HostID: host, Key: g.Key, Reason: reason, Containers: len(g.Members)})
		}
	}
	slices.SortFunc(list, func(a, b api.HiddenGroup) int {
		if c := strings.Compare(a.HostID, b.HostID); c != 0 {
			return c
		}
		return strings.Compare(a.Key, b.Key)
	})
	return list, nil
}

// ShowHidden brings a hidden service back, or creates the service discovery set aside.
func (s *Store) ShowHidden(ctx context.Context, hostID, key string) (string, error) {
	var slug string
	err := s.pool.QueryRow(ctx, `UPDATE services SET hidden = false, updated_at = now()
		WHERE host_id = $1 AND discovery_key = $2 AND archived_at IS NULL RETURNING slug`, hostID, key).Scan(&slug)
	if err == nil {
		return slug, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	byHost, err := liveContainers(ctx, s.pool, hostID)
	if err != nil {
		return "", err
	}
	for _, g := range reconcile.Groups(byHost[hostID]) {
		if g.Key != key {
			continue
		}
		err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			if err := upsertDiscovered(ctx, tx, hostID, g); err != nil {
				return err
			}
			return tx.QueryRow(ctx, `UPDATE services SET force_visible = true
				WHERE host_id = $1 AND discovery_key = $2 RETURNING slug`, hostID, key).Scan(&slug)
		})
		return slug, err
	}
	return "", api.ErrNotFound
}

// linkable lists the projects of hostID that slug can count as its own: set aside by
// discovery, with no service, free of any other link, or already linked to slug.
func (s *Store) linkable(ctx context.Context, hostID, slug string) ([]api.ProjectRef, error) {
	byHost, err := liveContainers(ctx, s.pool, hostID)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT discovery_key FROM services WHERE host_id = $1 AND discovery_key IS NOT NULL
		UNION SELECT discovery_key FROM service_links WHERE host_id = $1 AND service_slug <> $2`, hostID, slug)
	if err != nil {
		return nil, err
	}
	taken, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	list := []api.ProjectRef{}
	for _, g := range reconcile.Groups(byHost[hostID]) {
		if g.Kind != reconcile.KindService && !slices.Contains(taken, g.Key) {
			list = append(list, api.ProjectRef{Key: g.Key, Containers: len(g.Members)})
		}
	}
	return list, nil
}
