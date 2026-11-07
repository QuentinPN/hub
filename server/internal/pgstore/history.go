package pgstore

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"hub/server/internal/agentproto"
	"hub/server/internal/api"
	"hub/server/internal/notify"
	"hub/server/internal/reconcile"
)

var (
	_ api.HistoryStore = (*Store)(nil)
	_ notify.AlertLog  = (*Store)(nil)
)

// maxContainerEventsPerHour keeps a container that restarts in a loop from filling the
// history: past it, the hour's other events are dropped.
const maxContainerEventsPerHour = 60

// rowQuerier is what both the pool and a transaction offer for one row.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// eventDetail is what an event keeps beyond its kind and action; the field names are those
// of api.HistoryEvent, which reads it back as is.
type eventDetail struct {
	Reason    string   `json:"reason,omitempty"`
	Minutes   *int     `json:"minutes,omitempty"`
	Container string   `json:"container,omitempty"`
	ExitCode  *int     `json:"exitCode,omitempty"`
	From      string   `json:"from,omitempty"`
	To        string   `json:"to,omitempty"`
	Image     string   `json:"image,omitempty"`
	Alert     string   `json:"alert,omitempty"`
	Title     string   `json:"title,omitempty"`
	Via       []string `json:"via,omitempty"`
}

// addEvent writes an event for a service that exists; ok is false when it does not. A user
// unknown to the database leaves it unsigned.
func addEvent(ctx context.Context, q rowQuerier, slug string, at time.Time, kind, action string, d eventDetail, userID *int64) (id int64, ok bool, err error) {
	detail, err := json.Marshal(d)
	if err != nil {
		return 0, false, err
	}
	err = q.QueryRow(ctx, `INSERT INTO service_events (service_slug, at, kind, action, detail, user_id)
		SELECT $1, $2, $3, $4, $5, (SELECT id FROM users WHERE id = $6) WHERE EXISTS (SELECT 1 FROM services WHERE slug = $1)
		RETURNING id`, slug, at, kind, action, detail, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

// imageChange is a container that replaced another of the same Compose service (or name)
// with another image or version: noted before the write, recorded once services are synced.
type imageChange struct {
	containerID     string
	from, to, image string
}

// containerHistory gathers what one agent message brings to the history.
type containerHistory struct {
	changes []imageChange
	events  []agentproto.Event
}

// note compares a container seen for the first time with the one it replaces.
func (h *containerHistory) note(ctx context.Context, tx pgx.Tx, hostID string, c agentproto.Container) error {
	project, service := c.Labels[agentproto.LabelComposeProject], c.Labels[agentproto.LabelComposeService]
	var image, version string
	err := tx.QueryRow(ctx, `SELECT image, coalesce(labels->>'org.opencontainers.image.version', '') FROM containers
		WHERE host_id = $1 AND id <> $2
			AND NOT EXISTS (SELECT 1 FROM containers WHERE host_id = $1 AND id = $2)
			AND CASE WHEN $3 <> '' THEN compose_project = $3 AND compose_service = $4
				ELSE compose_project = '' AND name = $5 END
		ORDER BY seen_at DESC LIMIT 1`, hostID, c.ID, project, service, c.Name).Scan(&image, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	from := imageVersion(reconcile.Container{Image: image, Labels: map[string]string{"org.opencontainers.image.version": version}})
	to := imageVersion(reconcile.Container{Image: c.Image, Labels: c.Labels})
	if from == to && image == c.Image {
		return nil
	}
	if from == "" && to == "" || from == to {
		from, to = image, c.Image
	}
	ref, _, _ := strings.Cut(c.Image, "@")
	h.changes = append(h.changes, imageChange{containerID: c.ID, from: from, to: to, image: ref})
	return nil
}

// write records what was noted, once the containers and services are written in tx.
func (h *containerHistory) write(ctx context.Context, tx pgx.Tx, hostID string, now time.Time) error {
	for _, c := range h.changes {
		slug, label, ok, err := serviceOfContainer(ctx, tx, hostID, c.containerID)
		if err != nil || !ok {
			return err
		}
		if _, _, err := addEvent(ctx, tx, slug, now, api.HistoryVersion, "changed",
			eventDetail{Container: label, From: c.from, To: c.to, Image: c.image}, nil); err != nil {
			return err
		}
	}
	for _, e := range h.events {
		slug, label, ok, err := serviceOfContainer(ctx, tx, hostID, e.ContainerID)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		var recent int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM service_events
			WHERE service_slug = $1 AND kind = 'container' AND at > $2`, slug, now.Add(-time.Hour)).Scan(&recent); err != nil {
			return err
		}
		if recent >= maxContainerEventsPerHour {
			continue
		}
		if _, _, err := addEvent(ctx, tx, slug, now, api.HistoryContainer, e.Action,
			eventDetail{Container: label, ExitCode: e.ExitCode}, nil); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RecordServiceEdit(ctx context.Context, slug, kind string, userID *int64, at time.Time) error {
	_, _, err := addEvent(ctx, s.pool, slug, at, api.HistoryEdit, kind, eventDetail{}, userID)
	return err
}

// LogAlert keeps an alert sent about a service; one about a service gone is dropped.
func (s *Store) LogAlert(ctx context.Context, slug string, a notify.SentAlert) error {
	action := "sent"
	if a.Recovery {
		action = "recovery"
	}
	_, _, err := addEvent(ctx, s.pool, slug, a.At, api.HistoryAlert, action, eventDetail{Alert: a.Alert, Title: a.Title, Via: a.Via}, nil)
	return err
}

// SampleHistory counts this minute for every discovered service: up, down, or unknown when
// its host does not answer, and opens or closes its outage. A second call in the same
// minute counts nothing more.
func (s *Store) SampleHistory(ctx context.Context, now time.Time) error {
	all, err := s.entries(ctx)
	if err != nil {
		return err
	}
	hosts, err := s.AgentHosts(ctx, now)
	if err != nil {
		return err
	}
	reachable := map[string]bool{}
	for _, h := range hosts {
		reachable[h.ID] = h.State == api.AgentConnected
	}
	minute, hour := now.Truncate(time.Minute), now.Truncate(time.Hour)
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, e := range all {
			if e.discoveryKey == nil {
				continue
			}
			known, down := reachable[e.HostID], e.failing()
			var up, dn, unk, latSum, latN int
			switch {
			case !known:
				unk = 1
			case down:
				dn = 1
			default:
				up = 1
				if e.LatencyMs != nil {
					latSum, latN = *e.LatencyMs, 1
				}
			}
			tag, err := tx.Exec(ctx, `INSERT INTO service_checks AS c (service_slug, hour, up_minutes, down_minutes,
					unknown_minutes, latency_sum_ms, latency_count, last_minute)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (service_slug, hour) DO UPDATE SET
					up_minutes = least(60, c.up_minutes + excluded.up_minutes),
					down_minutes = least(60, c.down_minutes + excluded.down_minutes),
					unknown_minutes = least(60, c.unknown_minutes + excluded.unknown_minutes),
					latency_sum_ms = c.latency_sum_ms + excluded.latency_sum_ms,
					latency_count = least(60, c.latency_count + excluded.latency_count),
					last_minute = excluded.last_minute
				WHERE c.last_minute < excluded.last_minute`, e.Slug, hour, up, dn, unk, latSum, latN, minute)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 || !known {
				continue
			}
			if err := trackOutage(ctx, tx, e, down, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// trackOutage writes the start of an outage, or its end with how long it lasted.
func trackOutage(ctx context.Context, tx pgx.Tx, e entry, down bool, now time.Time) error {
	var was bool
	var since time.Time
	var eventID *int64
	err := tx.QueryRow(ctx, "SELECT down, since, event_id FROM service_health WHERE service_slug = $1 FOR UPDATE", e.Slug).
		Scan(&was, &since, &eventID)
	first := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !first {
		return err
	}
	if !first && was == down {
		return nil
	}
	var id *int64
	switch {
	case down:
		n, _, err := addEvent(ctx, tx, e.Slug, now, api.HistoryOutage, api.OutageDown, eventDetail{Reason: e.Note}, nil)
		if err != nil {
			return err
		}
		id = &n
	case !first:
		minutes := int(math.Round(now.Sub(since).Minutes()))
		if eventID != nil {
			if _, err := tx.Exec(ctx, `UPDATE service_events SET detail = detail || jsonb_build_object('minutes', $2::int)
				WHERE id = $1`, *eventID, minutes); err != nil {
				return err
			}
		}
		if _, _, err := addEvent(ctx, tx, e.Slug, now, api.HistoryOutage, api.OutageUp, eventDetail{Minutes: &minutes}, nil); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO service_health (service_slug, down, since, event_id) VALUES ($1, $2, $3, $4)
		ON CONFLICT (service_slug) DO UPDATE SET down = excluded.down, since = excluded.since, event_id = excluded.event_id`,
		e.Slug, down, now, id)
	return err
}

// PurgeHistory erases what is older than the retention chosen in « Général ».
func (s *Store) PurgeHistory(ctx context.Context, now time.Time) error {
	g, err := s.General(ctx)
	if err != nil {
		return err
	}
	cut := now.AddDate(0, 0, -g.HistoryDays)
	if _, err := s.pool.Exec(ctx, "DELETE FROM service_events WHERE at < $1", cut); err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, "DELETE FROM service_checks WHERE hour < $1", cut)
	return err
}

func (s *Store) ServiceHistory(ctx context.Context, slug string, q api.HistoryQuery, now time.Time) (api.ServiceHistory, error) {
	out := api.ServiceHistory{Counts: map[string]int{}, Days: []api.HistoryDay{}, Events: []api.HistoryEvent{}, Offset: q.Offset, Limit: q.Limit}
	var created, started time.Time
	err := s.pool.QueryRow(ctx, "SELECT s.created_at, m.started_at FROM services s, history_meta m WHERE s.slug = $1", slug).
		Scan(&created, &started)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, api.ErrNotFound
	}
	if err != nil {
		return out, err
	}
	g, err := s.General(ctx)
	if err != nil {
		return out, err
	}
	loc, err := s.TimeZone(ctx)
	if err != nil {
		return out, err
	}
	out.RetentionDays = g.HistoryDays
	out.Since = latest(started, created, now.AddDate(0, 0, -g.HistoryDays))

	if err := s.historyDays(ctx, slug, loc, now, &out); err != nil {
		return out, err
	}

	rows, err := s.pool.Query(ctx, "SELECT kind, count(*) FROM service_events WHERE service_slug = $1 GROUP BY kind", slug)
	if err != nil {
		return out, err
	}
	var kind string
	var n int
	if _, err := pgx.ForEachRow(rows, []any{&kind, &n}, func() error {
		out.Counts[kind] = n
		return nil
	}); err != nil {
		return out, err
	}
	for k, n := range out.Counts {
		if len(q.Kinds) == 0 || slices.Contains(q.Kinds, k) {
			out.Total += n
		}
	}

	kinds := q.Kinds
	if kinds == nil {
		kinds = []string{}
	}
	rows, err = s.pool.Query(ctx, `SELECT e.id, e.at, e.kind, e.action, e.detail, coalesce(u.username, '')
		FROM service_events e LEFT JOIN users u ON u.id = e.user_id
		WHERE e.service_slug = $1 AND (cardinality($2::text[]) = 0 OR e.kind = ANY($2))
		ORDER BY e.at DESC, e.id DESC OFFSET $3 LIMIT $4`, slug, kinds, q.Offset, q.Limit)
	if err != nil {
		return out, err
	}
	out.Events, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (api.HistoryEvent, error) {
		var ev api.HistoryEvent
		var detail []byte
		var by string
		if err := row.Scan(&ev.ID, &ev.At, &ev.Kind, &ev.Action, &detail, &by); err != nil {
			return ev, err
		}
		if err := json.Unmarshal(detail, &ev); err != nil {
			return ev, err
		}
		ev.By = by
		return ev, nil
	})
	if out.Events == nil {
		out.Events = []api.HistoryEvent{}
	}
	return out, err
}

// historyDays fills the availability bar: one day per local date, oldest first.
func (s *Store) historyDays(ctx context.Context, slug string, loc *time.Location, now time.Time, out *api.ServiceHistory) error {
	local := now.In(loc)
	first := time.Date(local.Year(), local.Month(), local.Day()-(api.HistoryBarDays-1), 0, 0, 0, 0, loc)
	index := map[string]int{}
	latSum, latN := make([]int64, api.HistoryBarDays), make([]int, api.HistoryBarDays)
	for i := range api.HistoryBarDays {
		d := time.Date(first.Year(), first.Month(), first.Day()+i, 0, 0, 0, 0, loc).Format(time.DateOnly)
		index[d] = i
		out.Days = append(out.Days, api.HistoryDay{Date: d})
	}
	rows, err := s.pool.Query(ctx, `SELECT hour, up_minutes, down_minutes, unknown_minutes, latency_sum_ms, latency_count
		FROM service_checks WHERE service_slug = $1 AND hour >= $2`, slug, first)
	if err != nil {
		return err
	}
	var hour time.Time
	var up, down, unknown, n int
	var sum int64
	if _, err := pgx.ForEachRow(rows, []any{&hour, &up, &down, &unknown, &sum, &n}, func() error {
		i, ok := index[hour.In(loc).Format(time.DateOnly)]
		if !ok {
			return nil
		}
		d := &out.Days[i]
		d.UpMinutes += up
		d.DownMinutes += down
		d.UnknownMinutes += unknown
		latSum[i] += sum
		latN[i] += n
		return nil
	}); err != nil {
		return err
	}
	for i := range out.Days {
		if latN[i] > 0 {
			ms := int(math.Round(float64(latSum[i]) / float64(latN[i])))
			out.Days[i].LatencyMs = &ms
		}
	}
	rows, err = s.pool.Query(ctx, `SELECT at FROM service_events
		WHERE service_slug = $1 AND kind = 'outage' AND action = 'down' AND at >= $2`, slug, first)
	if err != nil {
		return err
	}
	var at time.Time
	_, err = pgx.ForEachRow(rows, []any{&at}, func() error {
		if i, ok := index[at.In(loc).Format(time.DateOnly)]; ok {
			out.Days[i].Outages++
		}
		return nil
	})
	return err
}

func latest(ts ...time.Time) time.Time {
	out := ts[0]
	for _, t := range ts[1:] {
		if t.After(out) {
			out = t
		}
	}
	return out
}
