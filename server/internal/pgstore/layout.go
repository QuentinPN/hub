package pgstore

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"hub/server/internal/api"
	"hub/server/internal/widgets"
)

var _ api.LayoutEditor = (*Store)(nil)

// layoutRows reads the arrangement, always in the shape the pages expect.
func (s *Store) layoutRows(ctx context.Context) ([]api.LayoutRow, error) {
	var rows []api.LayoutRow
	err := s.pool.QueryRow(ctx, "SELECT layout FROM home_layout").Scan(&rows)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return api.NormalizeLayout(rows), nil
}

func (s *Store) SaveHomeLayout(ctx context.Context, rows []api.LayoutRow) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO home_layout (layout) VALUES ($1)
		ON CONFLICT (id) DO UPDATE SET layout = excluded.layout, updated_at = now()`, rows)
	return err
}

func (s *Store) ResetHomeLayout(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM home_layout")
	return err
}

// HomeLayout adds to the arrangement what each row could show today. A source left out but
// gone for now stays excluded, in case it comes back.
func (s *Store) HomeLayout(ctx context.Context) (api.HomeLayout, error) {
	rows, err := s.layoutRows(ctx)
	if err != nil {
		return api.HomeLayout{}, err
	}
	p, err := s.section(ctx)
	if err != nil {
		return api.HomeLayout{}, err
	}
	choices := map[string][]api.LayoutChoice{
		api.RowPlaying: {}, api.RowFavorites: {}, api.RowUpcoming: {}, api.RowDownloads: {}, api.RowHosts: {},
	}
	for _, e := range p.all {
		c, ok := p.conns[e.Slug]
		if !ok {
			continue
		}
		choice := api.LayoutChoice{ID: e.Slug, Label: e.Name}
		if slices.Contains(c.enabled, widgets.EmbySessions) {
			choices[api.RowPlaying] = append(choices[api.RowPlaying], choice)
		}
		if slices.Contains(c.enabled, widgets.WidgetCalendar) {
			choices[api.RowUpcoming] = append(choices[api.RowUpcoming], choice)
		}
		if cards, err := widgets.CardsOf(c.kind, c.data, c.enabled); err == nil && len(cards) > 0 {
			choices[api.RowDownloads] = append(choices[api.RowDownloads], choice)
		}
	}
	hosts, err := s.AgentHosts(ctx, time.Now())
	if err != nil {
		return api.HomeLayout{}, err
	}
	for _, h := range hosts {
		if h.LastSeenAt != nil {
			choices[api.RowHosts] = append(choices[api.RowHosts], api.LayoutChoice{ID: h.ID, Label: h.Name})
		}
	}
	return api.HomeLayout{Rows: rows, Choices: choices}, nil
}

// excludedBy looks up what each row leaves out.
func excludedBy(rows []api.LayoutRow) map[string][]string {
	out := make(map[string][]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.Excluded
	}
	return out
}
