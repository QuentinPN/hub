package pgstore

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"hub/server/internal/api"
	"hub/server/internal/reconcile"
)

var _ api.ServiceSplitter = (*Store)(nil)

// ownService reads a live service's host and discovery key, locked for the transaction.
func ownService(ctx context.Context, tx pgx.Tx, slug string) (hostID string, key *string, err error) {
	err = tx.QueryRow(ctx, "SELECT host_id, discovery_key FROM services WHERE slug = $1 AND archived_at IS NULL FOR UPDATE", slug).
		Scan(&hostID, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, api.ErrNotFound
	}
	return hostID, key, err
}

// groupOf finds the group of key among hostID's present containers.
func groupOf(ctx context.Context, tx pgx.Tx, hostID, key string) (reconcile.Group, bool, error) {
	byHost, err := liveContainers(ctx, tx, hostID)
	if err != nil {
		return reconcile.Group{}, false, err
	}
	for _, g := range reconcile.Groups(byHost[hostID]) {
		if g.Key == key {
			return g, true, nil
		}
	}
	return reconcile.Group{}, false, nil
}

// SplitService makes a Compose service of slug's project a service of its own, named by the
// user. The project must keep another service that is not a one-shot task. A service split
// before and joined since comes back with its address and widgets.
func (s *Store) SplitService(ctx context.Context, slug string, split api.ServiceSplit) (string, error) {
	var created string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		hostID, key, err := ownService(ctx, tx, slug)
		if err != nil {
			return err
		}
		if key == nil {
			return api.ErrInvalidSplit
		}
		if _, _, ok := reconcile.SplitOf(*key); ok {
			return api.ErrInvalidSplit
		}
		g, ok, err := groupOf(ctx, tx, hostID, *key)
		if err != nil {
			return err
		}
		if !ok {
			return api.ErrInvalidSplit
		}
		services := map[string]bool{}
		for _, m := range g.Members {
			if m.Role != reconcile.RoleOneShot && m.ComposeProject == *key && m.ComposeService != "" {
				services[m.ComposeService] = true
			}
		}
		if !services[split.Service] || len(services) < 2 {
			return api.ErrInvalidSplit
		}
		if _, err := tx.Exec(ctx, `INSERT INTO container_splits (host_id, project, service) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`, hostID, *key, split.Service); err != nil {
			return err
		}
		newKey := reconcile.SplitKey(*key, split.Service)
		g, ok, err = groupOf(ctx, tx, hostID, newKey)
		if err != nil {
			return err
		}
		if !ok {
			return api.ErrInvalidSplit
		}
		g.Slug, g.Name, g.Category = reconcile.Slugify(split.Name), split.Name, split.Category
		if err := upsertDiscovered(ctx, tx, hostID, g); err != nil {
			return err
		}
		// Shown whatever it holds: the user asked for it, a database alone included.
		if err := tx.QueryRow(ctx, `UPDATE services SET name = $3, monogram = $4, category = $5, hidden = false,
				force_visible = true, updated_at = now()
			WHERE host_id = $1 AND discovery_key = $2 RETURNING slug`,
			hostID, newKey, split.Name, reconcile.Monogram(split.Name), split.Category).Scan(&created); err != nil {
			return err
		}
		return syncServices(ctx, tx, hostID, time.Now())
	})
	return created, err
}

// JoinService gives a split service's containers back to their project. The split service
// is archived, its history kept; the project's service, if any, is returned.
func (s *Store) JoinService(ctx context.Context, slug string) (string, error) {
	var parent string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		hostID, key, err := ownService(ctx, tx, slug)
		if err != nil {
			return err
		}
		if key == nil {
			return api.ErrInvalidSplit
		}
		project, service, ok := reconcile.SplitOf(*key)
		if !ok {
			return api.ErrInvalidSplit
		}
		now := time.Now()
		if _, err := tx.Exec(ctx, "DELETE FROM container_splits WHERE host_id = $1 AND project = $2 AND service = $3",
			hostID, project, service); err != nil {
			return err
		}
		// Its links would stay taken by a service nobody sees.
		if _, err := tx.Exec(ctx, "DELETE FROM service_links WHERE service_slug = $1", slug); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE services SET archived_at = $2, absent_since = coalesce(absent_since, $2),
			updated_at = now() WHERE slug = $1`, slug, now); err != nil {
			return err
		}
		if err := syncServices(ctx, tx, hostID, now); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, "SELECT slug FROM services WHERE host_id = $1 AND discovery_key = $2 AND archived_at IS NULL",
			hostID, project).Scan(&parent)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return parent, err
}

// splitInfo describes where a split service comes from; nil for any other key.
func (s *Store) splitInfo(ctx context.Context, hostID, key string) (*api.ContainerSplit, error) {
	project, service, ok := reconcile.SplitOf(key)
	if !ok {
		return nil, nil
	}
	sp := api.ContainerSplit{Project: project, Service: service}
	err := s.pool.QueryRow(ctx, `SELECT sp.created_at, coalesce(s.slug, ''), coalesce(s.name, '')
		FROM container_splits sp
		LEFT JOIN services s ON s.host_id = sp.host_id AND s.discovery_key = sp.project AND s.archived_at IS NULL
		WHERE sp.host_id = $1 AND sp.project = $2 AND sp.service = $3`, hostID, project, service).
		Scan(&sp.Since, &sp.ParentSlug, &sp.ParentName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &sp, err
}
