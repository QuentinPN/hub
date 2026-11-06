package pgstore

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"hub/server/internal/api"
	"hub/server/internal/domain"
	"hub/server/internal/widgets"
)

var _ api.SectionReader = (*Store)(nil)

// section is a page's view of the services: the visible ones, with their connection if any.
type section struct {
	all   []entry
	conns map[string]connection
}

func (s *Store) section(ctx context.Context) (section, error) {
	all, err := s.entries(ctx)
	if err != nil {
		return section{}, err
	}
	conns, err := s.connections(ctx)
	if err != nil {
		return section{}, err
	}
	return section{slices.DeleteFunc(all, func(e entry) bool { return e.hidden }), conns}, nil
}

// base fills in what any page says about the connections of its kinds, and calls each with
// the connection and its state when its data can be shown.
func (p section) base(kinds []string, each func(e entry, c connection, state string) error) (api.SectionBase, error) {
	out := api.SectionBase{Notices: []api.SectionNotice{}, ToConnect: []api.WidgetRow{}}
	for _, e := range p.all {
		c, ok := p.conns[e.Slug]
		if !ok {
			if slices.Contains(kinds, e.app()) && widgets.SupportOf(e.app()) == widgets.SupportReady {
				out.ToConnect = append(out.ToConnect, api.WidgetRow{Slug: e.Slug, Name: e.Name, Monogram: e.Monogram, Icon: e.Icon,
					Color: e.Color, HostID: e.HostID, HasURL: e.URL != nil, Support: widgets.SupportReady})
			}
			continue
		}
		if !slices.Contains(kinds, c.kind) {
			continue
		}
		out.Connected++
		state := c.stateFor(e)
		if state != string(widgets.StateOK) {
			out.Notices = append(out.Notices, api.SectionNotice{Slug: e.Slug, Name: e.Name, State: state, DataAt: c.dataAt})
		}
		if keepsData(state) {
			if err := each(e, c, state); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

// decode reads a stored value when the widget it belongs to is on.
func decode[T any](c connection, key, widget string) (T, bool, error) {
	var v T
	raw, ok := c.data[key]
	if !ok || !slices.Contains(c.enabled, widget) {
		return v, false, nil
	}
	return v, true, json.Unmarshal(raw, &v)
}

func (s *Store) MediaPage(ctx context.Context) (api.MediaPage, error) {
	page := api.MediaPage{Sessions: []api.Session{}, Upcoming: []api.UpcomingItem{}, Latest: []widgets.MediaItem{}, Services: []api.ServiceSummary{}}
	p, err := s.section(ctx)
	if err != nil {
		return page, err
	}
	page.SectionBase, err = p.base([]string{"emby", "seerr", "sonarr", "radarr"}, func(e entry, c connection, state string) error {
		// Stale playbacks would say something false: only a connection that answers counts.
		if sessions, ok, err := decode[[]api.Session](c, widgets.EmbySessions, widgets.EmbySessions); err != nil {
			return err
		} else if ok && state == string(widgets.StateOK) && page.SessionsSlug == "" {
			page.Sessions, page.SessionsSlug = sessions, e.Slug
		}
		if list, ok, err := decode[[]api.UpcomingItem](c, widgets.WidgetCalendar, widgets.WidgetCalendar); err != nil {
			return err
		} else if ok {
			page.Upcoming = append(page.Upcoming, list...)
		}
		if list, ok, err := decode[[]widgets.MediaItem](c, widgets.EmbyLatest, widgets.EmbyLatest); err != nil {
			return err
		} else if ok && len(page.Latest) == 0 {
			page.Latest = list
		}
		if lib, ok, err := decode[widgets.Library](c, widgets.EmbyLibrary, widgets.EmbyLibrary); err != nil {
			return err
		} else if ok && page.Library == nil {
			page.Library = &lib
		}
		if r, ok, err := decode[widgets.Requests](c, widgets.WidgetRequests, widgets.WidgetRequests); err != nil {
			return err
		} else if ok && page.Requests == nil {
			page.Requests, page.RequestsSlug, page.RequestsURL = &r, e.Slug, e.URL
		}
		return nil
	})
	if err != nil {
		return page, err
	}
	slices.SortStableFunc(page.Upcoming, func(a, b api.UpcomingItem) int { return strings.Compare(a.At, b.At) })
	page.Upcoming = page.Upcoming[:min(len(page.Upcoming), maxUpcoming)]
	for _, e := range p.all {
		if e.Category == "Médias" {
			page.Services = append(page.Services, e.ServiceSummary)
		}
	}
	return page, nil
}

func (s *Store) DownloadsPage(ctx context.Context) (api.DownloadsPage, error) {
	page := api.DownloadsPage{Torrents: []widgets.Torrent{}, Queue: []widgets.QueueItem{}, Cards: []api.DownloadCard{}}
	p, err := s.section(ctx)
	if err != nil {
		return page, err
	}
	page.SectionBase, err = p.base([]string{"qbittorrent", "sonarr", "radarr", "prowlarr"}, func(e entry, c connection, state string) error {
		if t, ok, err := decode[widgets.Transfer](c, widgets.WidgetTransfer, widgets.WidgetTransfer); err != nil {
			return err
		} else if ok && page.Transfer == nil {
			page.Transfer, page.TransferSlug, page.TransferHost = &t, e.Slug, e.HostID
		}
		if list, ok, err := decode[[]widgets.Torrent](c, widgets.DataTorrents, widgets.WidgetTransfer); err != nil {
			return err
		} else if ok {
			page.Torrents = append(page.Torrents, list...)
		}
		if list, ok, err := decode[[]widgets.QueueItem](c, widgets.DataQueueItems, widgets.WidgetQueue); err != nil {
			return err
		} else if ok {
			page.Queue = append(page.Queue, list...)
		}
		cards, err := widgets.CardsOf(c.kind, c.data, c.enabled)
		if err != nil {
			return err
		}
		for _, card := range cards {
			if card.Suffix != "" { // the transfers have the top of the page
				page.Cards = append(page.Cards, api.DownloadCard{Slug: e.Slug, Label: e.Name + " · " + card.Suffix,
					Headline: card.Headline, Note: card.Note, State: state, DataAt: c.dataAt})
			}
		}
		return nil
	})
	return page, err
}

func (s *Store) InfraPage(ctx context.Context) (api.InfraPage, error) {
	page := api.InfraPage{Hosts: []api.InfraHost{}, Watch: []api.ServiceSummary{}, Groups: []api.InfraGroup{}}
	p, err := s.section(ctx)
	if err != nil {
		return page, err
	}
	hosts, err := s.AgentHosts(ctx, time.Now())
	if err != nil {
		return page, err
	}
	page.Total = len(p.all)
	for _, e := range p.all {
		if e.Status == domain.StatusDegraded || e.Status == domain.StatusDown {
			page.Watch = append(page.Watch, e.ServiceSummary)
		}
	}
	for _, h := range hosts {
		if h.LastSeenAt == nil {
			continue
		}
		group := api.InfraGroup{HostID: h.ID, Name: h.Name, Services: []api.ServiceSummary{}}
		for _, e := range p.all {
			if e.HostID == h.ID {
				group.Services = append(group.Services, e.ServiceSummary)
			}
		}
		page.Hosts = append(page.Hosts, api.InfraHost{AgentHost: h, Services: len(group.Services)})
		if len(group.Services) > 0 {
			page.Groups = append(page.Groups, group)
		}
	}
	return page, nil
}
