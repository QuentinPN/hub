package pgstore

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"hub/server/internal/api"
	"hub/server/internal/domain"
	"hub/server/internal/notify"
	"hub/server/internal/urlcheck"
	"hub/server/internal/widgets"
)

var _ notify.Store = (*Store)(nil)

// NotifySettings reads the settings; without a row, no channel and the defaults.
func (s *Store) NotifySettings(ctx context.Context) (notify.Settings, error) {
	out := notify.Settings{SMTPPort: 587, QuietHours: true, Alerts: map[string]bool{}}
	err := s.pool.QueryRow(ctx, `SELECT ntfy_url, ntfy_hint, smtp_host, smtp_port, smtp_username, smtp_from, smtp_to,
			alerts, quiet_hours, last_session
		FROM notify_settings`).Scan(&out.NtfyURL, &out.NtfyHint, &out.SMTPHost, &out.SMTPPort, &out.SMTPUsername,
		&out.SMTPFrom, &out.SMTPTo, &out.Alerts, &out.QuietHours, &out.LastSession)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if out.Alerts == nil {
		out.Alerts = map[string]bool{}
	}
	return out, err
}

func (s *Store) SaveNotifySettings(ctx context.Context, n notify.Settings) error {
	alerts := n.Alerts
	if alerts == nil {
		alerts = map[string]bool{}
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO notify_settings (ntfy_url, ntfy_hint, smtp_host, smtp_port, smtp_username,
			smtp_from, smtp_to, alerts, quiet_hours)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET ntfy_url = excluded.ntfy_url, ntfy_hint = excluded.ntfy_hint,
			smtp_host = excluded.smtp_host, smtp_port = excluded.smtp_port, smtp_username = excluded.smtp_username,
			smtp_from = excluded.smtp_from, smtp_to = excluded.smtp_to, alerts = excluded.alerts,
			quiet_hours = excluded.quiet_hours, updated_at = now()`,
		n.NtfyURL, n.NtfyHint, n.SMTPHost, n.SMTPPort, n.SMTPUsername, n.SMTPFrom, n.SMTPTo, alerts, n.QuietHours)
	return err
}

func (s *Store) SetNotifyLastSession(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO notify_settings (last_session) VALUES ($1)
		ON CONFLICT (id) DO UPDATE SET last_session = excluded.last_session`, id)
	return err
}

func (s *Store) ChannelStates(ctx context.Context) (map[string]notify.Send, error) {
	rows, err := s.pool.Query(ctx, "SELECT channel, at, ok, test, error FROM notify_channel_state")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]notify.Send{}
	for rows.Next() {
		var ch string
		var st notify.Send
		if err := rows.Scan(&ch, &st.At, &st.OK, &st.Test, &st.Error); err != nil {
			return nil, err
		}
		out[ch] = st
	}
	return out, rows.Err()
}

func (s *Store) SaveChannelState(ctx context.Context, channel string, st notify.Send) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO notify_channel_state (channel, at, ok, test, error) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (channel) DO UPDATE SET at = excluded.at, ok = excluded.ok, test = excluded.test, error = excluded.error`,
		channel, st.At, st.OK, st.Test, st.Error)
	return err
}

func (s *Store) DeleteChannelState(ctx context.Context, channel string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM notify_channel_state WHERE channel = $1", channel)
	return err
}

func (s *Store) Incidents(ctx context.Context) ([]notify.Incident, error) {
	rows, err := s.pool.Query(ctx, "SELECT key, alert, subject, detail, since, notified_at FROM notify_incidents ORDER BY since, key")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (notify.Incident, error) {
		var i notify.Incident
		return i, row.Scan(&i.Key, &i.Alert, &i.Subject, &i.Detail, &i.Since, &i.NotifiedAt)
	})
}

func (s *Store) PutIncident(ctx context.Context, i notify.Incident) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO notify_incidents (key, alert, subject, detail, since, notified_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (key) DO UPDATE SET subject = excluded.subject, detail = excluded.detail, notified_at = excluded.notified_at`,
		i.Key, i.Alert, i.Subject, i.Detail, i.Since, i.NotifiedAt)
	return err
}

func (s *Store) DeleteIncident(ctx context.Context, key string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM notify_incidents WHERE key = $1", key)
	return err
}

func (s *Store) AddDigest(ctx context.Context, l notify.DigestLine) error {
	_, err := s.pool.Exec(ctx, "INSERT INTO notify_digest (at, title, body) VALUES ($1, $2, $3)", l.At, l.Title, l.Body)
	return err
}

func (s *Store) Digest(ctx context.Context) ([]notify.DigestLine, error) {
	rows, err := s.pool.Query(ctx, "SELECT id, at, title, body FROM notify_digest ORDER BY id")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (notify.DigestLine, error) {
		var l notify.DigestLine
		return l, row.Scan(&l.ID, &l.At, &l.Title, &l.Body)
	})
}

func (s *Store) ClearDigest(ctx context.Context, upTo int64) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM notify_digest WHERE id <= $1", upTo)
	return err
}

func (s *Store) LearnDevice(ctx context.Context, label string, at time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, "INSERT INTO notify_devices (label, first_seen) VALUES ($1, $2) ON CONFLICT DO NOTHING", label, at)
	return tag.RowsAffected() == 1, err
}

func (s *Store) SessionsAfter(ctx context.Context, id *int64) ([]notify.SessionSeen, error) {
	after := int64(0)
	if id != nil {
		after = *id
	}
	rows, err := s.pool.Query(ctx, "SELECT id, user_agent, created_at FROM sessions WHERE id > $1 ORDER BY id", after)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (notify.SessionSeen, error) {
		var s notify.SessionSeen
		return s, row.Scan(&s.ID, &s.UserAgent, &s.CreatedAt)
	})
}

// NotifyConditions says what fails now. What sits on an unreachable host is held: the host's
// own alert speaks for it, and its services are unknown, not back.
func (s *Store) NotifyConditions(ctx context.Context, now time.Time) (notify.Snapshot, error) {
	var out notify.Snapshot
	hosts, err := s.AgentHosts(ctx, now)
	if err != nil {
		return out, err
	}
	reachable := map[string]bool{}
	for _, h := range hosts {
		reachable[h.ID] = h.State == api.AgentConnected
		switch h.State {
		case api.AgentOffline, api.AgentDockerDown, api.AgentExpired:
			out.Failing = append(out.Failing, notify.Condition{Key: "host:" + h.ID, Alert: notify.AlertHost, Subject: h.Name, Detail: string(h.State)})
		}
	}

	p, err := s.section(ctx)
	if err != nil {
		return out, err
	}
	beszel := false
	for _, e := range p.all {
		if e.discoveryKey != nil && !reachable[e.HostID] {
			out.Held = append(out.Held, "service:"+e.Slug, "widget:"+e.Slug)
			continue
		}
		down := e.Status == domain.StatusDown
		if e.check != nil && urlcheck.Kind(e.check.Result) == urlcheck.KindUnreachable {
			down = true
		}
		if down {
			out.Failing = append(out.Failing, notify.Condition{Key: "service:" + e.Slug, Alert: notify.AlertService, Subject: e.Name, Detail: e.Note})
		}
		c, ok := p.conns[e.Slug]
		if !ok {
			continue
		}
		state := c.stateFor(e)
		switch state {
		case string(widgets.StateOK):
		case api.WidgetStopped:
			// The service's own alert says it.
			out.Held = append(out.Held, "widget:"+e.Slug)
		default:
			out.Failing = append(out.Failing, notify.Condition{Key: "widget:" + e.Slug, Alert: notify.AlertWidget, Subject: e.Name, Detail: state})
		}
		if c.kind != "beszel" || state != string(widgets.StateOK) {
			continue
		}
		systems, ok, err := decode[[]widgets.System](c, widgets.WidgetSystems, widgets.WidgetSystems)
		if err != nil || !ok {
			continue
		}
		beszel = true
		for _, sys := range systems {
			key := "disk:" + sys.Name
			switch {
			case sys.Status != "up":
				out.Held = append(out.Held, key)
			case sys.Disk >= 90:
				out.Failing = append(out.Failing, notify.Condition{Key: key, Alert: notify.AlertDisk, Subject: sys.Name, Detail: strconv.Itoa(sys.Disk)})
			}
		}
	}
	// Without a fresh read from Beszel, nothing can be said of the disks.
	if !beszel {
		out.Held = append(out.Held, "disk:")
	}
	return out, nil
}
