// Package pgstore keeps the inventory and the settings in PostgreSQL. What a service is
// (name, category, host, favorite, order) lives in the database and survives restarts;
// its live state (status, latency, widget data) still comes from the fixture until the
// agents report it.
package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"hub/server/internal/api"
	"hub/server/internal/domain"
	"hub/server/internal/fixture"
)

type Store struct {
	pool *pgxpool.Pool
	live *fixture.Store
}

// New returns a store on a migrated database, writing the example inventory if the
// database has no host yet. An existing inventory is never overwritten.
func New(ctx context.Context, pool *pgxpool.Pool, live *fixture.Store) (*Store, error) {
	s := &Store{pool: pool, live: live}
	if err := s.seed(ctx); err != nil {
		return nil, fmt.Errorf("seed inventory: %w", err)
	}
	return s, nil
}

func (s *Store) seed(ctx context.Context) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Serializes concurrent first starts; released at commit.
		if _, err := tx.Exec(ctx, "LOCK TABLE hosts IN EXCLUSIVE MODE"); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM hosts)").Scan(&exists); err != nil || exists {
			return err
		}
		hosts, services := s.live.Seed()
		for _, h := range hosts {
			if _, err := tx.Exec(ctx, "INSERT INTO hosts (id, name, os, arch) VALUES ($1, $2, $3, $4)",
				h.ID, h.Name, h.OS, h.Arch); err != nil {
				return err
			}
		}
		for i, svc := range services {
			if _, err := tx.Exec(ctx, `INSERT INTO services (slug, name, monogram, color, category, host_id, favorite, position)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				svc.Slug, svc.Name, svc.Monogram, svc.Color, svc.Category, svc.HostID, svc.Favorite, i); err != nil {
				return fmt.Errorf("service %s: %w", svc.Slug, err)
			}
		}
		return nil
	})
}

// services reads the inventory and lays the live state over it. A service the live source
// has never seen reads "unknown", never "online".
func (s *Store) services(ctx context.Context) ([]api.ServiceSummary, error) {
	live, err := s.live.Services(ctx)
	if err != nil {
		return nil, err
	}
	bySlug := make(map[string]api.ServiceSummary, len(live))
	for _, l := range live {
		bySlug[l.Slug] = l
	}

	rows, err := s.pool.Query(ctx, `SELECT slug, name, monogram, color, category, host_id, favorite
		FROM services ORDER BY position, slug`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (api.ServiceSummary, error) {
		var r api.ServiceSummary
		if err := row.Scan(&r.Slug, &r.Name, &r.Monogram, &r.Color, &r.Category, &r.HostID, &r.Favorite); err != nil {
			return r, err
		}
		l, ok := bySlug[r.Slug]
		if !ok {
			l = api.ServiceSummary{Status: domain.StatusUnknown, Note: "jamais observé"}
		}
		r.Status, r.LatencyMs, r.Note = l.Status, l.LatencyMs, l.Note
		return r, nil
	})
}

func (s *Store) Services(ctx context.Context) ([]api.ServiceSummary, error) {
	return s.services(ctx)
}

func (s *Store) Home(ctx context.Context) (api.Home, error) {
	home, err := s.live.Home(ctx)
	if err != nil {
		return home, err
	}
	all, err := s.services(ctx)
	if err != nil {
		return home, err
	}
	home.Favorites = []api.ServiceSummary{}
	home.Summary = api.Summary{Total: len(all)}
	for _, svc := range all {
		if svc.Favorite {
			home.Favorites = append(home.Favorites, svc)
		}
		if svc.Status == domain.StatusOnline {
			home.Summary.Online++
		}
	}
	return home, nil
}

func (s *Store) Service(ctx context.Context, slug string) (api.ServiceDetail, error) {
	all, err := s.services(ctx)
	if err != nil {
		return api.ServiceDetail{}, err
	}
	for _, sum := range all {
		if sum.Slug != slug {
			continue
		}
		detail, err := s.live.Service(ctx, slug)
		if errors.Is(err, api.ErrNotFound) {
			detail = api.ServiceDetail{
				Tint:           "#1f2a36",
				Stats:          []api.Stat{},
				Upcoming:       []api.UpcomingItem{},
				Identification: []api.KeyValue{{Label: "Catégorie", Value: sum.Category}},
				Access:         []api.KeyValue{{Label: "Hôte", Value: sum.HostID}},
				Connection:     []api.KeyValue{{Label: "Widget", Value: "non configuré"}},
			}
		} else if err != nil {
			return api.ServiceDetail{}, err
		}
		detail.ServiceSummary = sum
		return detail, nil
	}
	return api.ServiceDetail{}, api.ErrNotFound
}

func (s *Store) Settings(ctx context.Context) (api.Settings, error) {
	settings, err := s.live.Settings(ctx)
	if err != nil {
		return settings, err
	}
	d := &settings.Discovery
	err = s.pool.QueryRow(ctx, `SELECT follow_events, rescan_minutes, on_removal, archive_after_days, confirm_below_percent
		FROM discovery_settings`).Scan(&d.FollowEvents, &d.RescanMinutes, &d.OnRemoval, &d.ArchiveAfterDays, &d.ConfirmBelowPercent)
	d.InferURLFromPorts = false
	settings.Editable = true
	return settings, err
}

// UpdateDiscovery expects a Discovery already checked with Validate; the table
// constraints check it again.
func (s *Store) UpdateDiscovery(ctx context.Context, d api.Discovery) error {
	_, err := s.pool.Exec(ctx, `UPDATE discovery_settings SET follow_events = $1, rescan_minutes = $2, on_removal = $3,
		archive_after_days = $4, confirm_below_percent = $5, updated_at = now()`,
		d.FollowEvents, d.RescanMinutes, d.OnRemoval, d.ArchiveAfterDays, d.ConfirmBelowPercent)
	return err
}
