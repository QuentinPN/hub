package pgstore

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"hub/server/internal/api"
	"hub/server/internal/domain"
	"hub/server/internal/widgets"
)

var (
	_ api.WidgetEditor = (*Store)(nil)
	_ widgets.Source   = (*Store)(nil)
)

// maxUpcoming bounds the home page's row: beyond it, the fiches have the rest.
const maxUpcoming = 20

// connection is a row of widget_connections.
type connection struct {
	slug, kind, url, keyHint, state string
	options                         map[string]string
	enabled                         []string
	refreshSeconds, failures        int
	data                            map[string]json.RawMessage
	dataAt, failingSince            *time.Time
	canAct                          *bool
	checkedAt                       time.Time
	generation                      int64
}

func (s *Store) connections(ctx context.Context) (map[string]connection, error) {
	rows, err := s.pool.Query(ctx, `SELECT service_slug, kind, url, key_hint, state, options, enabled, refresh_seconds,
			failures, data, data_at, failing_since, checked_at, generation, can_act
		FROM widget_connections`)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (connection, error) {
		var c connection
		return c, row.Scan(&c.slug, &c.kind, &c.url, &c.keyHint, &c.state, &c.options, &c.enabled, &c.refreshSeconds,
			&c.failures, &c.data, &c.dataAt, &c.failingSince, &c.checkedAt, &c.generation, &c.canAct)
	})
	if err != nil {
		return nil, err
	}
	bySlug := make(map[string]connection, len(list))
	for _, c := range list {
		bySlug[c.slug] = c
	}
	return bySlug, nil
}

// stateFor says what the UI should word: the last read, unless the service changed since.
func (c connection) stateFor(e entry) string {
	switch {
	case !c.known():
		return api.WidgetUnsupported
	case e.URL == nil || *e.URL != c.url:
		return api.WidgetAddressChanged
	case e.Status == domain.StatusDown:
		return api.WidgetStopped
	}
	return c.state
}

// known says Hub still has the connector the connection was made with.
func (c connection) known() bool { return knownKind(c.kind) }

func knownKind(kind string) bool {
	_, ok := widgets.ManifestOf(kind)
	return ok
}

// The last read is shown, dated, through an outage; a refused key or a format the connector
// no longer knows shows nothing.
func keepsData(state string) bool {
	return state == string(widgets.StateOK) || state == string(widgets.StateUnreachable) || state == string(widgets.StateCert)
}

// app is the catalogue's name for the service, whatever the user renamed it to.
func (e entry) app() string {
	if e.group != nil {
		return e.group.Slug
	}
	return ""
}

// supportOf is the connection's own, unless Hub no longer has its connector: then what the
// catalogue offers now, and the connection stays only to be removed.
func supportOf(e entry, c *connection) (widgets.Support, string) {
	if c != nil && c.known() {
		return widgets.SupportReady, c.kind
	}
	return widgets.SupportOf(e.app()), e.app()
}

func (c connection) dto(e entry) *api.WidgetConnection {
	state := c.stateFor(e)
	checked := c.checkedAt
	out := &api.WidgetConnection{
		State: state, KeyHint: c.keyHint, RefreshSeconds: c.refreshSeconds, Enabled: c.enabled, Options: c.options,
		DataAt: c.dataAt, CheckedAt: &checked, FailingSince: c.failingSince, Data: map[string]json.RawMessage{}, CanAct: c.canAct,
	}
	if keepsData(state) {
		// Names of torrents and releases stay on the Téléchargements page.
		for id, raw := range c.data {
			if id != widgets.DataTorrents && id != widgets.DataQueueItems {
				out.Data[id] = raw
			}
		}
	}
	return out
}

// widgetInfo adds to the fiche what the service offers as widgets.
func (s *Store) widgetInfo(ctx context.Context, e entry, d *api.ServiceDetail) error {
	conns, err := s.connections(ctx)
	if err != nil {
		return err
	}
	var conn *connection
	if c, ok := conns[e.Slug]; ok {
		conn = &c
	}
	support, kind := supportOf(e, conn)
	if support == widgets.SupportNone && conn == nil {
		return nil
	}
	d.Widget = &api.WidgetInfo{Support: support}
	if m, ok := widgets.ManifestOf(kind); ok {
		d.Widget.Manifest, d.Widget.RefreshChoices = &m, widgets.RefreshChoices
	}
	if conn == nil {
		return nil
	}
	d.Widget.Connection = conn.dto(e)
	if d.Widget.Manifest != nil && d.Widget.Manifest.Webhook {
		var configured bool
		err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM secrets WHERE name = $1)", widgets.WebhookName(e.Slug)).Scan(&configured)
		if err != nil {
			return err
		}
		d.Widget.Connection.Webhook = &api.WebhookState{Configured: configured}
	}
	d.Connection = []api.KeyValue{{Label: "Widget", Value: stateLabel(d.Widget.Connection.State)}}
	if conn.keyHint != "" {
		row := api.KeyValue{Label: "Clé d’API", Value: "•••• " + conn.keyHint}
		if d.Widget.Manifest != nil && d.Widget.Manifest.Auth == widgets.AuthPassword {
			row = api.KeyValue{Label: "Compte", Value: conn.keyHint}
		}
		d.Connection = append(d.Connection, row)
	}
	return nil
}

func stateLabel(state string) string {
	switch state {
	case string(widgets.StateOK):
		return "connecté"
	case string(widgets.StateAuth):
		return "clé refusée"
	case string(widgets.StateCert):
		return "certificat non reconnu"
	case string(widgets.StateUnreachable):
		return "injoignable"
	case string(widgets.StateUnexpected):
		return "réponse inattendue"
	case api.WidgetStopped:
		return "service arrêté"
	case api.WidgetAddressChanged:
		return "adresse changée"
	case api.WidgetUnsupported:
		return "plus pris en charge"
	}
	return state
}

// realWidgets lays over the live source what the connections last read.
// What the user left out of a row stays out of it.
func (s *Store) realWidgets(ctx context.Context, home *api.Home, all []entry, excluded map[string][]string) error {
	conns, err := s.connections(ctx)
	if err != nil {
		return err
	}
	var upcoming []api.UpcomingItem
	sessionsFound := false
	for _, e := range all {
		c, ok := conns[e.Slug]
		if !ok {
			continue
		}
		state := c.stateFor(e)
		// Stale playbacks would say something false: only a connection that answers counts.
		if !sessionsFound && state == string(widgets.StateOK) && slices.Contains(c.enabled, widgets.EmbySessions) &&
			!slices.Contains(excluded[api.RowPlaying], e.Slug) {
			sessions := []api.Session{}
			if raw, ok := c.data[widgets.EmbySessions]; ok {
				if err := json.Unmarshal(raw, &sessions); err != nil {
					return err
				}
			}
			home.Sessions, home.SessionsSlug, sessionsFound = sessions, e.Slug, true
			if list, ok, err := decode[[]widgets.MediaItem](c, widgets.EmbyLatest, widgets.EmbyLatest); err != nil {
				return err
			} else if ok && len(list) > 0 {
				home.LastAdded = &list[0]
			}
			if lib, ok, err := decode[widgets.Library](c, widgets.EmbyLibrary, widgets.EmbyLibrary); err != nil {
				return err
			} else if ok {
				home.Library = &lib
			}
		}
		if !keepsData(state) {
			continue
		}
		if home.LatestSlug == "" && !slices.Contains(excluded[api.RowLatest], e.Slug) {
			if list, ok, err := decode[[]widgets.MediaItem](c, widgets.EmbyLatest, widgets.EmbyLatest); err != nil {
				return err
			} else if ok && len(list) > 0 {
				home.Latest, home.LatestSlug = list, e.Slug
			}
		}
		if raw, ok := c.data[widgets.WidgetCalendar]; ok && slices.Contains(c.enabled, widgets.WidgetCalendar) &&
			!slices.Contains(excluded[api.RowUpcoming], e.Slug) {
			var list []api.UpcomingItem
			if err := json.Unmarshal(raw, &list); err != nil {
				return err
			}
			for i := range list {
				list[i].Source = e.Slug
			}
			upcoming = append(upcoming, list...)
		}
		if slices.Contains(excluded[api.RowDownloads], e.Slug) {
			continue
		}
		cards, err := widgets.CardsOf(c.kind, c.data, c.enabled)
		if err != nil {
			return err
		}
		for _, card := range cards {
			label := e.Name + " · " + e.HostID
			if card.Suffix != "" {
				label = e.Name + " · " + card.Suffix
			}
			home.Downloads = append(home.Downloads, api.DownloadCard{Slug: e.Slug, Label: label,
				Headline: card.Headline, Note: card.Note, State: state, DataAt: c.dataAt})
		}
	}
	if upcoming != nil {
		slices.SortStableFunc(upcoming, func(a, b api.UpcomingItem) int { return strings.Compare(a.At, b.At) })
		home.Upcoming = upcoming[:min(len(upcoming), maxUpcoming)]
	}
	return nil
}

// WidgetRows lists what the settings show: the services Hub knows something about, with or
// without a connection.
func (s *Store) WidgetRows(ctx context.Context) ([]api.WidgetRow, error) {
	all, err := s.entries(ctx)
	if err != nil {
		return nil, err
	}
	conns, err := s.connections(ctx)
	if err != nil {
		return nil, err
	}
	rows := []api.WidgetRow{}
	for _, e := range all {
		var conn *connection
		if c, ok := conns[e.Slug]; ok {
			conn = &c
		}
		support, kind := supportOf(e, conn)
		if e.hidden || support == widgets.SupportNone && conn == nil {
			continue
		}
		row := api.WidgetRow{Slug: e.Slug, Name: e.Name, Monogram: e.Monogram, Icon: e.Icon, Color: e.Color, HostID: e.HostID,
			HasURL: e.URL != nil, Support: support}
		if conn != nil {
			row.Kind, row.State, row.DataAt, row.FailingSince = kind, conn.stateFor(e), conn.dataAt, conn.failingSince
			row.CanAct = conn.canAct
		}
		rows = append(rows, row)
	}
	// What can be acted on first, then what is only announced.
	rank := func(r api.WidgetRow) int {
		switch {
		case r.State != "":
			return 0
		case r.Support == widgets.SupportReady:
			return 1
		case r.Support == widgets.SupportPlanned:
			return 2
		}
		return 3
	}
	slices.SortStableFunc(rows, func(a, b api.WidgetRow) int { return rank(a) - rank(b) })
	return rows, nil
}

func (s *Store) WidgetTarget(ctx context.Context, slug string) (api.WidgetTarget, error) {
	all, err := s.entries(ctx)
	if err != nil {
		return api.WidgetTarget{}, err
	}
	i := slices.IndexFunc(all, func(e entry) bool { return e.Slug == slug })
	if i < 0 {
		return api.WidgetTarget{}, api.ErrNotFound
	}
	e := all[i]
	var t api.WidgetTarget
	var conn *connection
	err = s.pool.QueryRow(ctx, "SELECT url_cert_sha256 FROM services WHERE slug = $1", slug).Scan(&t.Pinned)
	if err != nil {
		return t, err
	}
	conns, err := s.connections(ctx)
	if err != nil {
		return t, err
	}
	if c, ok := conns[slug]; ok {
		conn = &c
		t.ConnectedURL, t.KeyHint = c.url, c.keyHint
	}
	t.Support, t.Kind = supportOf(e, conn)
	if e.URL != nil {
		t.URL = *e.URL
	}
	return t, nil
}

// SaveWidgetConnection replaces the connection and moves its generation on, so a read begun
// before the save is ignored.
func (s *Store) SaveWidgetConnection(ctx context.Context, slug string, c api.NewConnection) error {
	data := c.Data
	if data == nil {
		data = map[string]json.RawMessage{}
	}
	var dataAt, failingSince *time.Time
	failures := 0
	if c.State == widgets.StateOK {
		dataAt = &c.At
	} else {
		failingSince, failures = &c.At, 1
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO widget_connections (service_slug, kind, url, key_hint, options, enabled,
			refresh_seconds, state, failures, data, data_at, checked_at, failing_since, can_act)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (service_slug) DO UPDATE SET kind = excluded.kind, url = excluded.url, key_hint = excluded.key_hint,
			options = excluded.options, enabled = excluded.enabled, refresh_seconds = excluded.refresh_seconds,
			state = excluded.state, failures = excluded.failures, data = excluded.data, data_at = excluded.data_at,
			checked_at = excluded.checked_at, failing_since = excluded.failing_since, can_act = excluded.can_act,
			generation = widget_connections.generation + 1, updated_at = now()`,
		slug, c.Kind, c.URL, c.KeyHint, c.Options, c.Enabled, c.RefreshSeconds, string(c.State), failures, data, dataAt, c.At, failingSince, c.CanAct)
	return err
}

func (s *Store) DeleteWidgetConnection(ctx context.Context, slug string) error {
	var exists bool
	if err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM services WHERE slug = $1)", slug).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return api.ErrNotFound
	}
	_, err := s.pool.Exec(ctx, "DELETE FROM widget_connections WHERE service_slug = $1", slug)
	return err
}

// liveRow is a connection worth talking to, with when to read it.
type liveRow struct {
	widgets.Due
	refresh, failures int
	checked           time.Time
}

// liveRows leaves out a refused key, an address changed since the key was tested, a stopped
// service, whose API would only time out, and a connector Hub no longer has.
func (s *Store) liveRows(ctx context.Context) ([]liveRow, error) {
	rows, err := s.pool.Query(ctx, `SELECT c.service_slug, c.kind, c.url, s.url_cert_sha256, c.options, c.enabled,
			c.refresh_seconds, c.failures, c.checked_at, c.generation
		FROM widget_connections c JOIN services s ON s.slug = c.service_slug
		WHERE s.archived_at IS NULL AND s.url = c.url AND c.state <> 'auth'`)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (liveRow, error) {
		var r liveRow
		return r, row.Scan(&r.Slug, &r.Kind, &r.URL, &r.Pinned, &r.Options, &r.Enabled, &r.refresh, &r.failures, &r.checked, &r.Generation)
	})
	if err != nil || len(list) == 0 {
		return nil, err
	}
	all, err := s.entries(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list, func(r liveRow) bool {
		return !knownKind(r.Kind) || slices.ContainsFunc(all, func(e entry) bool { return e.Slug == r.Slug && e.Status == domain.StatusDown })
	}), nil
}

// LiveConnections is empty in « Économe »: no connection stays open, reads keep to the clock.
func (s *Store) LiveConnections(ctx context.Context) ([]widgets.Due, error) {
	if economy, err := s.widgetEconomy(ctx); err != nil || economy {
		return nil, err
	}
	rows, err := s.liveRows(ctx)
	if err != nil {
		return nil, err
	}
	list := make([]widgets.Due, 0, len(rows))
	for _, r := range rows {
		list = append(list, r.Due)
	}
	return list, nil
}

// DueConnections reads each connection at its own rate, never more often than
// widgets.EconomyRefresh in « Économe ».
func (s *Store) DueConnections(ctx context.Context, now time.Time) ([]widgets.Due, error) {
	economy, err := s.widgetEconomy(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.liveRows(ctx)
	if err != nil {
		return nil, err
	}
	due := []widgets.Due{}
	for _, r := range rows {
		refresh := time.Duration(r.refresh) * time.Second
		if economy {
			refresh = max(refresh, widgets.EconomyRefresh)
		}
		if !r.checked.Add(widgets.NextDelay(refresh, r.failures)).After(now) {
			due = append(due, r.Due)
		}
	}
	return due, nil
}

// RecordRead keeps the last good data through a failure.
func (s *Store) RecordRead(ctx context.Context, d widgets.Due, data map[string]json.RawMessage, state widgets.State, at time.Time) error {
	if state == widgets.StateOK {
		_, err := s.pool.Exec(ctx, `UPDATE widget_connections SET state = 'ok', failures = 0, failing_since = NULL, data = $3,
				data_at = $4, checked_at = $4 WHERE service_slug = $1 AND generation = $2`, d.Slug, d.Generation, data, at)
		return err
	}
	_, err := s.pool.Exec(ctx, `UPDATE widget_connections SET state = $3, failures = failures + 1,
				failing_since = COALESCE(failing_since, $4), checked_at = $4 WHERE service_slug = $1 AND generation = $2`,
		d.Slug, d.Generation, string(state), at)
	return err
}
