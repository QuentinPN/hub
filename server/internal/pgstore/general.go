package pgstore

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"hub/server/internal/api"
)

var _ api.GeneralStore = (*Store)(nil)

// General reads the « Général » section; without a row, the defaults.
func (s *Store) General(ctx context.Context) (api.General, error) {
	g := api.DefaultGeneral()
	err := s.pool.QueryRow(ctx, "SELECT display_name, start_page, time_zone, widget_refresh, history_days FROM general_settings").
		Scan(&g.DisplayName, &g.StartPage, &g.TimeZone, &g.WidgetRefresh, &g.HistoryDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, nil
	}
	return g, err
}

// SaveGeneral writes the section; a shorter retention erases the older history at once.
func (s *Store) SaveGeneral(ctx context.Context, g api.General) error {
	if _, err := s.pool.Exec(ctx, `INSERT INTO general_settings (display_name, start_page, time_zone, widget_refresh, history_days)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET display_name = excluded.display_name, start_page = excluded.start_page,
			time_zone = excluded.time_zone, widget_refresh = excluded.widget_refresh,
			history_days = excluded.history_days, updated_at = now()`,
		g.DisplayName, g.StartPage, g.TimeZone, g.WidgetRefresh, g.HistoryDays); err != nil {
		return err
	}
	return s.PurgeHistory(ctx, time.Now())
}

// TimeZone is where schedules fall (backups, quiet hours): the zone chosen in « Général »,
// else the server's own (TZ). A name this binary no longer knows falls back to TZ too.
func (s *Store) TimeZone(ctx context.Context) (*time.Location, error) {
	var name string
	err := s.pool.QueryRow(ctx, "SELECT time_zone FROM general_settings").Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && name == "" {
		return time.Local, nil
	}
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		slog.Warn("unknown time zone, using the server's", "zone", name)
		return time.Local, nil
	}
	return loc, nil
}

// widgetEconomy says whether « Économe » is chosen.
func (s *Store) widgetEconomy(ctx context.Context) (bool, error) {
	var mode string
	err := s.pool.QueryRow(ctx, "SELECT widget_refresh FROM general_settings").Scan(&mode)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return mode == api.RefreshEconomy, err
}
