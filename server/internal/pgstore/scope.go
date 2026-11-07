package pgstore

import (
	"context"

	"github.com/jackc/pgx/v5"

	"hub/server/internal/api"
)

// containerSplit says whether a container row's Compose service is split from its project.
const containerSplit = `EXISTS (SELECT 1 FROM container_splits sp WHERE sp.host_id = containers.host_id
	AND sp.project = containers.compose_project AND sp.service = containers.compose_service)`

// containerKey is a container row's group key, as reconcile.KeyOf computes it: its Compose
// project, "project/service" once split, or its name outside Compose.
const containerKey = `CASE WHEN compose_project <> '' AND compose_service <> '' AND ` + containerSplit + `
	THEN compose_project || '/' || compose_service ELSE coalesce(nullif(compose_project, ''), name) END`

// ServiceScope is what a discovered service is made of on its host: its own project first,
// then the projects linked to it.
type ServiceScope struct {
	HostID string
	Keys   []string
}

// ServiceScope returns ErrNotFound for an unknown, archived or example service: those have
// no containers.
func (s *Store) ServiceScope(ctx context.Context, slug string) (ServiceScope, error) {
	rows, err := s.pool.Query(ctx, `SELECT host_id, discovery_key FROM (
			SELECT host_id, discovery_key, 0 AS rank FROM services
				WHERE slug = $1 AND discovery_key IS NOT NULL AND archived_at IS NULL
			UNION ALL
			SELECT l.host_id, l.discovery_key, 1 FROM service_links l
				JOIN services s ON s.slug = l.service_slug
				WHERE l.service_slug = $1 AND s.archived_at IS NULL
		) k ORDER BY rank, discovery_key`, slug)
	if err != nil {
		return ServiceScope{}, err
	}
	var scope ServiceScope
	var host, key string
	if _, err := pgx.ForEachRow(rows, []any{&host, &key}, func() error {
		scope.HostID = host
		scope.Keys = append(scope.Keys, key)
		return nil
	}); err != nil {
		return ServiceScope{}, err
	}
	if scope.Keys == nil {
		return ServiceScope{}, api.ErrNotFound
	}
	return scope, nil
}

// ServiceOfContainer names the service a container belongs to, by its own project or a
// linked one, even once the container is gone. ok is false when it belongs to none.
func (s *Store) ServiceOfContainer(ctx context.Context, hostID, containerID string) (slug string, ok bool, err error) {
	slug, _, ok, err = serviceOfContainer(ctx, s.pool, hostID, containerID)
	return slug, ok, err
}

// serviceOfContainer also gives the container's label in its service: its Compose service,
// or its name outside Compose.
func serviceOfContainer(ctx context.Context, q querier, hostID, containerID string) (slug, label string, ok bool, err error) {
	rows, err := q.Query(ctx, `WITH c AS (
			SELECT `+containerKey+` AS key, coalesce(nullif(compose_service, ''), name) AS label
			FROM containers WHERE host_id = $1 AND id = $2)
		SELECT slug, c.label FROM services, c
			WHERE host_id = $1 AND discovery_key = c.key AND archived_at IS NULL
		UNION ALL
		SELECT service_slug, c.label FROM service_links, c WHERE host_id = $1 AND discovery_key = c.key
		LIMIT 1`, hostID, containerID)
	if err != nil {
		return "", "", false, err
	}
	var row struct{ Slug, Label string }
	row, err = pgx.CollectOneRow(rows, pgx.RowToStructByPos[struct{ Slug, Label string }])
	if err == pgx.ErrNoRows {
		return "", "", false, nil
	}
	return row.Slug, row.Label, err == nil, err
}
