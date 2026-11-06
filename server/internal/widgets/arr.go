package widgets

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"hub/server/internal/secrets"
)

// Widgets of a Sonarr or Radarr connection.
const (
	WidgetCalendar = "calendar"
	WidgetQueue    = "queue"

	arrCalendarDays = 14
	arrQueuePage    = 100
)

// The routes and fields come from each project's openapi.json (src/*.Api.V3). Sonarr and
// Radarr share their key and queue; only their calendars differ.
type arr struct {
	kind, name, keyHelp, unit string
	calendarQuery             url.Values
	upcoming                  func(ctx context.Context, c *Conn, query url.Values, from time.Time) ([]Upcoming, error)
}

var (
	sonarr = arr{
		kind: "sonarr", name: "Sonarr", unit: "épisode",
		keyHelp:       "Dans Sonarr : Settings, General, API Key. La clé donne tous les droits sur Sonarr : Hub ne s’en sert que pour lire.",
		calendarQuery: url.Values{"includeSeries": {"true"}},
		upcoming:      sonarrUpcoming,
	}
	radarr = arr{
		kind: "radarr", name: "Radarr", unit: "film",
		keyHelp:  "Dans Radarr : Settings, General, API Key. La clé donne tous les droits sur Radarr : Hub ne s’en sert que pour lire.",
		upcoming: radarrUpcoming,
	}
)

// Upcoming is an episode or film to come. The page words its date, in the viewer's time zone.
type Upcoming struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Kind   string `json:"kind"`
	At     string `json:"at"`
}

// Queue counts what is being fetched. No title: the home page has no use for one.
type Queue struct {
	Total       int `json:"total"`
	Downloading int `json:"downloading"`
	Problems    int `json:"problems"`
}

func (a arr) Manifest() Manifest {
	return Manifest{
		Kind:    a.kind,
		Name:    a.name,
		KeyHelp: a.keyHelp,
		Reads:   []string{"GET /api/v3/queue/status", "GET /api/v3/calendar", "GET /api/v3/queue", "POST puis WebSocket /signalr/messages : signaux de changement"},
		Widgets: []Widget{
			{ID: WidgetCalendar, Label: "À venir", Where: "accueil et fiche"},
			{ID: WidgetQueue, Label: "File d’attente", Where: "accueil et fiche"},
		},
	}
}

func (arr) Authorize(req *http.Request, key secrets.Value) {
	req.Header.Set("X-Api-Key", key.Reveal())
}

func (arr) Probe(ctx context.Context, c *Conn) (Probe, error) {
	var status struct{ TotalCount int }
	if err := c.GetJSON(ctx, "/api/v3/queue/status", nil, &status); err != nil {
		return Probe{}, err
	}
	p := Probe{Options: []Option{}}
	switch status.TotalCount {
	case 0:
		p.Note = "File d’attente vide"
	case 1:
		p.Note = "1 élément en file d’attente"
	default:
		p.Note = strconv.Itoa(status.TotalCount) + " éléments en file d’attente"
	}
	return p, nil
}

func (a arr) Fetch(ctx context.Context, c *Conn, enabled []string) (map[string]any, error) {
	data := map[string]any{}
	if slices.Contains(enabled, WidgetCalendar) {
		now := time.Now().UTC()
		query := url.Values{
			"start": {now.Format(time.RFC3339)},
			"end":   {now.AddDate(0, 0, arrCalendarDays).Format(time.RFC3339)},
		}
		for k, v := range a.calendarQuery {
			query[k] = v
		}
		list, err := a.upcoming(ctx, c, query, now)
		if err != nil {
			return nil, err
		}
		slices.SortStableFunc(list, func(x, y Upcoming) int {
			if x.At < y.At {
				return -1
			}
			if x.At > y.At {
				return 1
			}
			return 0
		})
		data[WidgetCalendar] = list
	}
	if slices.Contains(enabled, WidgetQueue) {
		var page struct {
			TotalRecords int
			Records      []struct {
				Status                string
				TrackedDownloadStatus string
			}
		}
		query := url.Values{"page": {"1"}, "pageSize": {strconv.Itoa(arrQueuePage)}}
		if err := c.GetJSON(ctx, "/api/v3/queue", query, &page); err != nil {
			return nil, err
		}
		q := Queue{Total: page.TotalRecords}
		for _, r := range page.Records {
			if r.Status == "downloading" {
				q.Downloading++
			}
			if r.TrackedDownloadStatus == "warning" || r.TrackedDownloadStatus == "error" {
				q.Problems++
			}
		}
		data[WidgetQueue] = q
	}
	return data, nil
}

func sonarrUpcoming(ctx context.Context, c *Conn, query url.Values, _ time.Time) ([]Upcoming, error) {
	var found []struct {
		Title         string
		SeasonNumber  int
		EpisodeNumber int
		AirDateUtc    string
		HasFile       bool
		Series        *struct{ Title string }
	}
	if err := c.GetJSON(ctx, "/api/v3/calendar", query, &found); err != nil {
		return nil, err
	}
	list := []Upcoming{}
	for _, e := range found {
		if e.HasFile || e.AirDateUtc == "" {
			continue // already there, or no date to show
		}
		title := e.Title
		if e.Series != nil && e.Series.Title != "" {
			title = e.Series.Title
		}
		list = append(list, Upcoming{
			Title:  title,
			Detail: fmt.Sprintf("S%02dE%02d · Sonarr", e.SeasonNumber, e.EpisodeNumber),
			Kind:   "episode",
			At:     e.AirDateUtc,
		})
	}
	return list, nil
}

func radarrUpcoming(ctx context.Context, c *Conn, query url.Values, from time.Time) ([]Upcoming, error) {
	var found []struct {
		Title           string
		InCinemas       string
		DigitalRelease  string
		PhysicalRelease string
		HasFile         *bool
	}
	if err := c.GetJSON(ctx, "/api/v3/calendar", query, &found); err != nil {
		return nil, err
	}
	list := []Upcoming{}
	for _, m := range found {
		if m.HasFile != nil && *m.HasFile {
			continue
		}
		// A film has up to three dates: the next one is what the calendar answered for.
		var next time.Time
		var label string
		for _, d := range []struct{ at, label string }{
			{m.InCinemas, "En salle"},
			{m.DigitalRelease, "En numérique"},
			{m.PhysicalRelease, "En physique"},
		} {
			at, err := time.Parse(time.RFC3339, d.at)
			if err != nil || at.Before(from) {
				continue
			}
			if next.IsZero() || at.Before(next) {
				next, label = at, d.label
			}
		}
		if next.IsZero() {
			continue
		}
		list = append(list, Upcoming{Title: m.Title, Detail: label + " · Radarr", Kind: "movie", At: next.UTC().Format(time.RFC3339)})
	}
	return list, nil
}

// QueueText words a queue for the home page. Sonarr counts episodes, Radarr films.
func QueueText(kind string, q Queue) (headline, note string) {
	if q.Total == 0 {
		return "File vide", "Rien à récupérer"
	}
	unit := "élément"
	if a, ok := connectors[kind].(arr); ok {
		unit = a.unit
	}
	if q.Total > 1 {
		unit += "s"
	}
	headline = strconv.Itoa(q.Total) + " " + unit
	note = strconv.Itoa(q.Downloading) + " en cours · " + strconv.Itoa(q.Total-q.Downloading) + " en attente"
	if q.Problems > 0 {
		note += " · " + strconv.Itoa(q.Problems) + " à vérifier"
	}
	return headline, note
}
