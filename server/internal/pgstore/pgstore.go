// Package pgstore keeps the inventory and the settings in PostgreSQL. What a service is
// (name, category, host, favorite, order) lives in the database and survives restarts. A
// discovered service reads its state from its containers; the example inventory still
// borrows its own from the fixture.
package pgstore

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"hub/server/internal/api"
	"hub/server/internal/domain"
	"hub/server/internal/fixture"
	"hub/server/internal/reconcile"
	"hub/server/internal/urlcheck"
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
			if _, err := tx.Exec(ctx, `INSERT INTO services (slug, name, monogram, icon, color, category, host_id, favorite, position)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				svc.Slug, svc.Name, svc.Monogram, svc.Icon, svc.Color, svc.Category, svc.HostID, svc.Favorite, i); err != nil {
				return fmt.Errorf("service %s: %w", svc.Slug, err)
			}
		}
		return nil
	})
}

// entry is a service with what the summary does not say.
type entry struct {
	api.ServiceSummary
	discoveryKey *string
	recognizedBy string
	group        *reconcile.Group
	hidden       bool
	check        *api.URLCheck
	links        []string
	// up counts it as running: "configuration required" only says no URL is known yet.
	up bool
}

// entries reads the inventory and lays the live state over it: from the containers for a
// discovered service, from the live source otherwise. A service the live source has never
// seen reads "unknown", never "online".
func (s *Store) entries(ctx context.Context) ([]entry, error) {
	live, err := s.live.Services(ctx)
	if err != nil {
		return nil, err
	}
	bySlug := make(map[string]api.ServiceSummary, len(live))
	for _, l := range live {
		bySlug[l.Slug] = l
	}
	hosts, err := s.AgentHosts(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	reachable := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		reachable[h.ID] = h.State == api.AgentConnected
	}
	containers, err := liveContainers(ctx, s.pool, "")
	if err != nil {
		return nil, err
	}
	groups := map[[2]string]reconcile.Group{}
	for host, list := range containers {
		for _, g := range reconcile.Groups(list) {
			groups[[2]string{host, g.Key}] = g
		}
	}

	links, err := s.serviceLinks(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `SELECT slug, name, monogram, icon, color, category, host_id, favorite,
			discovery_key, recognized_by, absent_since, hidden, url, url_cert_sha256, url_result, url_status,
			url_latency_ms, url_checked_at
		FROM services WHERE archived_at IS NULL ORDER BY position, slug`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (entry, error) {
		var e entry
		var absent, checkedAt *time.Time
		var pinned string
		var c api.URLCheck
		if err := row.Scan(&e.Slug, &e.Name, &e.Monogram, &e.Icon, &e.Color, &e.Category, &e.HostID, &e.Favorite,
			&e.discoveryKey, &e.recognizedBy, &absent, &e.hidden, &e.URL, &pinned, &c.Result, &c.Status,
			&c.LatencyMs, &checkedAt); err != nil {
			return e, err
		}
		if e.URL != nil {
			c.CheckedAt, c.CertPinned = checkedAt, pinned != ""
			e.check = &c
		}
		e.links = links[e.Slug]
		if e.discoveryKey == nil {
			l, ok := bySlug[e.Slug]
			if !ok {
				l = api.ServiceSummary{Status: domain.StatusUnknown, Note: "jamais observé"}
			}
			e.Status, e.LatencyMs, e.Note = l.Status, l.LatencyMs, l.Note
			e.up = e.Status == domain.StatusOnline
			return e, nil
		}
		if g, ok := groups[[2]string{e.HostID, *e.discoveryKey}]; ok && absent == nil {
			for _, key := range e.links {
				if other, ok := groups[[2]string{e.HostID, key}]; ok {
					g.Link(other)
				}
			}
			e.group = &g
			e.Status, e.Note = g.Status()
			e.applyCheck()
		} else {
			e.Status, e.Note = domain.StatusDown, "conteneurs absents"
		}
		if !reachable[e.HostID] {
			e.Status, e.Note = domain.EffectiveStatus(e.Status, false), "hôte injoignable"
		}
		e.up = e.Status == domain.StatusOnline || e.Status == domain.StatusConfigRequired
		return e, nil
	})
}

// applyCheck refines "everything runs" with what the address answers. An outage the
// containers already show stays as it is.
func (e *entry) applyCheck() {
	if e.Status != domain.StatusConfigRequired || e.check == nil {
		return
	}
	latency := e.check.LatencyMs
	switch urlcheck.Kind(e.check.Result) {
	case urlcheck.KindOK:
		e.Status, e.Note, e.LatencyMs = domain.StatusOnline, "en ligne", &latency
	case urlcheck.KindAuth:
		e.Status, e.Note, e.LatencyMs = domain.StatusOnline, "connexion demandée", &latency
	case urlcheck.KindCert:
		e.Status, e.Note = domain.StatusDegraded, "certificat non reconnu"
	case urlcheck.KindError:
		e.Status, e.Note = domain.StatusDegraded, "erreur "+strconv.Itoa(e.check.Status)
	default:
		e.Status, e.Note = domain.StatusDegraded, "adresse injoignable"
	}
}

func (s *Store) serviceLinks(ctx context.Context) (map[string][]string, error) {
	rows, err := s.pool.Query(ctx, "SELECT service_slug, discovery_key FROM service_links ORDER BY discovery_key")
	if err != nil {
		return nil, err
	}
	links := map[string][]string{}
	var slug, key string
	_, err = pgx.ForEachRow(rows, []any{&slug, &key}, func() error {
		links[slug] = append(links[slug], key)
		return nil
	})
	return links, err
}

func (s *Store) Services(ctx context.Context) ([]api.ServiceSummary, error) {
	all, err := s.entries(ctx)
	if err != nil {
		return nil, err
	}
	list := make([]api.ServiceSummary, 0, len(all))
	for _, e := range all {
		if !e.hidden {
			list = append(list, e.ServiceSummary)
		}
	}
	return list, nil
}

func (s *Store) Home(ctx context.Context) (api.Home, error) {
	home, err := s.live.Home(ctx)
	if err != nil {
		return home, err
	}
	entries, err := s.entries(ctx)
	if err != nil {
		return home, err
	}
	all := slices.DeleteFunc(entries, func(e entry) bool { return e.hidden })
	home.Favorites = []api.ServiceSummary{}
	home.Services = []api.ServiceSummary{}
	home.Summary = api.Summary{Total: len(all)}
	for _, e := range all {
		if e.Favorite {
			home.Favorites = append(home.Favorites, e.ServiceSummary)
		}
		if e.up {
			home.Summary.Online++
		}
	}
	// Until a favorite is chosen, the home page still has something to open.
	if len(home.Favorites) == 0 {
		for _, e := range all {
			home.Services = append(home.Services, e.ServiceSummary)
		}
	}
	if err := s.realSessions(ctx, &home, all); err != nil {
		return home, err
	}
	return home, s.realHosts(ctx, &home)
}

// realHosts replaces the live source's hosts as soon as an agent has reported.
func (s *Store) realHosts(ctx context.Context, home *api.Home) error {
	now := time.Now()
	hosts, err := s.AgentHosts(ctx, now)
	if err != nil {
		return err
	}
	real := []api.Host{}
	for _, h := range hosts {
		if h.LastSeenAt != nil {
			real = append(real, api.Host{ID: h.ID, Name: h.Name, OS: h.OS, Containers: h.Containers,
				Reachable: h.State == api.AgentConnected, LastSeenLabel: lastSeenLabel(h.LastSeenAt, now), Metrics: []api.HostMetric{}})
		}
	}
	if len(real) > 0 {
		home.Hosts = real
	}
	return nil
}

func (s *Store) Service(ctx context.Context, slug string) (api.ServiceDetail, error) {
	all, err := s.entries(ctx)
	if err != nil {
		return api.ServiceDetail{}, err
	}
	for _, e := range all {
		if e.Slug != slug {
			continue
		}
		if e.discoveryKey != nil {
			return s.discoveredService(ctx, e)
		}
		detail, err := s.live.Service(ctx, slug)
		if errors.Is(err, api.ErrNotFound) {
			detail = api.ServiceDetail{
				Tint:           "#1f2a36",
				Stats:          []api.Stat{},
				Upcoming:       []api.UpcomingItem{},
				Identification: []api.KeyValue{{Label: "Catégorie", Value: e.Category}},
				Access:         []api.KeyValue{{Label: "Hôte", Value: e.HostID}},
				Connection:     []api.KeyValue{{Label: "Widget", Value: "non configuré"}},
			}
		} else if err != nil {
			return api.ServiceDetail{}, err
		}
		detail.ServiceSummary = e.ServiceSummary
		return detail, nil
	}
	return api.ServiceDetail{}, api.ErrNotFound
}

// discoveredService adds what the fiche and its panel need to the containers' description.
func (s *Store) discoveredService(ctx context.Context, e entry) (api.ServiceDetail, error) {
	d := discoveredDetail(e.ServiceSummary, *e.discoveryKey, e.recognizedBy, e.group)
	d.Hidden, d.URLCheck = e.hidden, e.check
	d.Project = &api.ProjectRef{Key: *e.discoveryKey}
	if err := s.widgetInfo(ctx, e, &d); err != nil {
		return d, err
	}
	free, err := s.linkable(ctx, e.HostID, e.Slug)
	if err != nil {
		return d, err
	}
	sizes := map[string]int{}
	for _, p := range free {
		sizes[p.Key] = p.Containers
	}
	for _, key := range e.links {
		d.Links = append(d.Links, api.ProjectRef{Key: key, Containers: sizes[key]})
	}
	for _, p := range free {
		if !slices.Contains(e.links, p.Key) {
			d.Linkable = append(d.Linkable, p)
		}
	}
	if e.group != nil {
		own := 0
		for _, m := range e.group.Members {
			if m.ComposeProject == *e.discoveryKey || (m.ComposeProject == "" && m.Name == *e.discoveryKey) {
				own++
			}
		}
		d.Project.Containers = own
	}
	return d, nil
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
