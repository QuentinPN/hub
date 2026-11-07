package pgstore

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"hub/server/internal/api"
	"hub/server/internal/db"
	"hub/server/internal/widgets"
)

var _ api.StatusSource = (*Store)(nil)

func (s *Store) Ready(ctx context.Context) error {
	applied, expected, err := db.MigrationStatus(ctx, s.pool)
	if err != nil {
		return err
	}
	if applied < expected {
		return fmt.Errorf("%d migrations of %d applied", applied, expected)
	}
	return nil
}

// StoreStatus checks the database, then what the database says of the agents, the widgets
// and the addresses.
func (s *Store) StoreStatus(ctx context.Context) (api.StoreStatus, error) {
	var out api.StoreStatus
	start := time.Now()
	applied, expected, err := db.MigrationStatus(ctx, s.pool)
	if err != nil {
		return out, err
	}
	out.DBLatencyMs = int(time.Since(start).Milliseconds())
	out.MigrationsApplied, out.MigrationsExpected = applied, expected
	now := time.Now()
	dbCheck := api.StatusCheck{ID: "db", Name: "Base de données", Level: api.LevelOK, Detail: "Répond, migrations à jour", Since: &now}
	if applied < expected {
		dbCheck.Level, dbCheck.Detail = api.LevelBad, fmt.Sprintf("Migration %d sur %d", applied, expected)
	}
	out.Checks = append(out.Checks, dbCheck)

	hosts, err := s.AgentHosts(ctx, now)
	if err != nil {
		return out, err
	}
	out.Checks = append(out.Checks, agentsCheck(hosts))

	p, err := s.section(ctx)
	if err != nil {
		return out, err
	}
	out.Checks = append(out.Checks, widgetsCheck(p), urlsCheck(p))
	return out, nil
}

func agentsCheck(hosts []api.AgentHost) api.StatusCheck {
	c := api.StatusCheck{ID: "agents", Name: "Agents", Level: api.LevelOK, Link: "hosts"}
	enrolled, connected := 0, 0
	var away []string
	var since *time.Time
	for _, h := range hosts {
		switch h.State {
		case api.AgentNone, api.AgentPending:
			continue
		case api.AgentConnected:
			connected++
			if since == nil || h.LastSeenAt != nil && h.LastSeenAt.After(*since) {
				since = h.LastSeenAt
			}
		default:
			away = append(away, h.Name)
			if h.LastSeenAt != nil && (c.Since == nil || h.LastSeenAt.Before(*c.Since)) {
				c.Since = h.LastSeenAt
			}
		}
		enrolled++
	}
	if enrolled == 0 {
		c.Detail = "Aucun hôte enrôlé"
		return c
	}
	c.Detail = fmt.Sprintf("%d sur %d connectés", connected, enrolled)
	if len(away) > 0 {
		c.Level = api.LevelWarn
		c.Detail += " · " + strings.Join(away, ", ") + " injoignable"
		if len(away) > 1 {
			c.Detail += "s"
		}
		return c
	}
	c.Since = since
	return c
}

func widgetsCheck(p section) api.StatusCheck {
	c := api.StatusCheck{ID: "widgets", Name: "Widgets", Level: api.LevelOK, Link: "widgets"}
	total := 0
	var failing []string
	for _, e := range p.all {
		conn, ok := p.conns[e.Slug]
		if !ok {
			continue
		}
		total++
		state := conn.stateFor(e)
		if state == string(widgets.StateOK) {
			if c.Level == api.LevelOK && (c.Since == nil || conn.checkedAt.After(*c.Since)) {
				at := conn.checkedAt
				c.Since = &at
			}
			continue
		}
		failing = append(failing, e.Name)
		// What only the user can fix is worse than what retries alone.
		level := api.LevelWarn
		if state == string(widgets.StateAuth) || state == string(widgets.StateUnexpected) || state == api.WidgetAddressChanged {
			level = api.LevelBad
		}
		if c.Level != api.LevelBad {
			c.Level = level
		}
		if conn.failingSince != nil && (c.Level == level) {
			c.Since = conn.failingSince
		}
	}
	switch {
	case total == 0:
		c.Detail = "Aucune connexion"
	case len(failing) == 0:
		c.Detail = plural(total, "connexion à jour", "connexions, toutes à jour")
	default:
		c.Detail = fmt.Sprintf("%d sur %d en échec · %s", len(failing), total, strings.Join(failing, ", "))
	}
	return c
}

func urlsCheck(p section) api.StatusCheck {
	c := api.StatusCheck{ID: "urls", Name: "Vérification des adresses", Level: api.LevelOK, Link: "services"}
	total := 0
	var failing []string
	for _, e := range p.all {
		if e.check == nil {
			continue
		}
		total++
		if r := e.check.Result; r != "ok" && r != "auth" {
			failing = append(failing, e.Name)
		} else if e.check.CheckedAt != nil && (c.Since == nil || e.check.CheckedAt.After(*c.Since)) {
			c.Since = e.check.CheckedAt
		}
	}
	slices.Sort(failing)
	switch {
	case total == 0:
		c.Detail = "Aucune adresse enregistrée"
	case len(failing) == 0:
		c.Detail = plural(total, "adresse, elle répond", "adresses, toutes répondent")
	default:
		c.Level, c.Since = api.LevelWarn, nil
		c.Detail = fmt.Sprintf("%d sur %d ne répondent pas · %s", len(failing), total, strings.Join(failing, ", "))
	}
	return c
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
