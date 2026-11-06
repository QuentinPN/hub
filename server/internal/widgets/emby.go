package widgets

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"hub/server/internal/secrets"
)

// Widgets of an Emby connection.
const (
	EmbySessions = "sessions"
	EmbyLatest   = "latest"
	EmbyLibrary  = "library"

	embyOptionUser     = "userId"
	embyTicksPerMinute = 600_000_000
	embyLatestLimit    = 12
)

// The routes come from dev.emby.media/reference/RestAPI; the /emby prefix from its API key
// page's example. Only the fields kept below are read.
type emby struct{}

func (emby) Manifest() Manifest {
	return Manifest{
		Kind:    "emby",
		Name:    "Emby",
		KeyHelp: "Dans Emby : tableau de bord du serveur, Clés d’API. Crée une clé réservée à Hub : tu pourras la révoquer seule.",
		Reads:   []string{"GET /emby/Sessions", "GET /emby/Users/Query", "GET /emby/Users/…/Items/Latest", "GET /emby/Items/Counts"},
		Widgets: []Widget{
			{ID: EmbySessions, Label: "Sessions en cours", Where: "accueil et fiche"},
			{ID: EmbyLatest, Label: "Ajoutés récemment", Where: "fiche"},
			{ID: EmbyLibrary, Label: "Bibliothèque", Where: "fiche"},
		},
	}
}

func (emby) Authorize(req *http.Request, key secrets.Value) {
	req.Header.Set("X-Emby-Token", key.Reveal())
}

type embyItem struct {
	ID                string
	Name              string
	Type              string
	SeriesName        string
	ParentIndexNumber int
	IndexNumber       int
	RunTimeTicks      int64
	DateCreated       string
}

type embySession struct {
	DeviceName     string
	NowPlayingItem *embyItem
	PlayState      struct {
		PositionTicks int64
		IsPaused      bool
		PlayMethod    string
	}
}

// Session is a playback in progress. It has the same shape as the API's Session.
type Session struct {
	Title         string  `json:"title"`
	Subtitle      string  `json:"subtitle"`
	Device        string  `json:"device"`
	Progress      float64 `json:"progress"`
	DurationLabel string  `json:"durationLabel"`
	Transcoding   bool    `json:"transcoding"`
}

// MediaItem is an entry of a library.
type MediaItem struct {
	Title   string `json:"title"`
	Detail  string `json:"detail"`
	Kind    string `json:"kind"`
	AddedAt string `json:"addedAt"`
}

// Library counts what a library holds.
type Library struct {
	Movies   int `json:"movies"`
	Series   int `json:"series"`
	Episodes int `json:"episodes"`
}

func (emby) sessions(ctx context.Context, c *Conn) ([]Session, error) {
	var found []embySession
	if err := c.GetJSON(ctx, "/emby/Sessions", nil, &found); err != nil {
		return nil, err
	}
	list := []Session{}
	for _, s := range found {
		if s.NowPlayingItem != nil {
			list = append(list, s.session())
		}
	}
	return list, nil
}

func (s embySession) session() Session {
	item := s.NowPlayingItem
	out := Session{
		Title:       item.Name,
		Device:      s.DeviceName,
		Transcoding: s.PlayState.PlayMethod == "Transcode",
	}
	if item.SeriesName != "" {
		out.Title = item.SeriesName + ", " + episodeCode(*item)
		out.Subtitle = item.Name
	}
	if item.RunTimeTicks > 0 {
		out.Progress = min(1, max(0, float64(s.PlayState.PositionTicks)/float64(item.RunTimeTicks)))
		left := max(0, (item.RunTimeTicks-s.PlayState.PositionTicks+embyTicksPerMinute-1)/embyTicksPerMinute)
		out.DurationLabel = remaining(left)
	}
	if s.PlayState.IsPaused {
		if out.DurationLabel == "" {
			out.DurationLabel = "en pause"
		} else {
			out.DurationLabel = "en pause · " + out.DurationLabel
		}
	}
	return out
}

func remaining(minutes int64) string {
	if minutes <= 1 {
		return strconv.FormatInt(minutes, 10) + " min restante"
	}
	return strconv.FormatInt(minutes, 10) + " min restantes"
}

func episodeCode(i embyItem) string {
	return fmt.Sprintf("S%02dE%02d", i.ParentIndexNumber, i.IndexNumber)
}

// users lists the accounts for the "latest" widget, which Emby only answers for one account.
func (emby) users(ctx context.Context, c *Conn) ([]Choice, error) {
	var found struct {
		Items []struct{ ID, Name string }
	}
	if err := c.GetJSON(ctx, "/emby/Users/Query", nil, &found); err != nil {
		return nil, err
	}
	choices := make([]Choice, 0, len(found.Items))
	for _, u := range found.Items {
		choices = append(choices, Choice{Value: u.ID, Label: u.Name})
	}
	return choices, nil
}

func (e emby) Probe(ctx context.Context, c *Conn) (Probe, error) {
	sessions, err := e.sessions(ctx, c)
	if err != nil {
		return Probe{}, err
	}
	p := Probe{Options: []Option{}}
	switch len(sessions) {
	case 0:
		p.Note = "Aucune lecture en cours"
	case 1:
		p.Note = "1 lecture en cours"
	default:
		p.Note = strconv.Itoa(len(sessions)) + " lectures en cours"
	}
	users, err := e.users(ctx, c)
	if err != nil {
		// The key works: only the widget that needs an account stays out.
		if StateOf(err) == StateUnreachable {
			return Probe{}, err
		}
		return p, nil
	}
	p.Options = append(p.Options, Option{ID: embyOptionUser, Label: "Compte pour les nouveautés", Choices: users})
	return p, nil
}

func (e emby) Fetch(ctx context.Context, c *Conn, enabled []string) (map[string]any, error) {
	data := map[string]any{}
	if slices.Contains(enabled, EmbySessions) {
		sessions, err := e.sessions(ctx, c)
		if err != nil {
			return nil, err
		}
		data[EmbySessions] = sessions
	}
	user := c.Option(embyOptionUser)
	if slices.Contains(enabled, EmbyLatest) && user != "" {
		var found []embyItem
		query := url.Values{"Limit": {strconv.Itoa(embyLatestLimit)}, "Fields": {"DateCreated"}}
		if err := c.GetJSON(ctx, "/emby/Users/"+url.PathEscape(user)+"/Items/Latest", query, &found); err != nil {
			return nil, err
		}
		list := make([]MediaItem, 0, len(found))
		for _, i := range found {
			list = append(list, i.mediaItem())
		}
		data[EmbyLatest] = list
	}
	if slices.Contains(enabled, EmbyLibrary) {
		var counts struct{ MovieCount, SeriesCount, EpisodeCount int }
		if err := c.GetJSON(ctx, "/emby/Items/Counts", nil, &counts); err != nil {
			return nil, err
		}
		data[EmbyLibrary] = Library{Movies: counts.MovieCount, Series: counts.SeriesCount, Episodes: counts.EpisodeCount}
	}
	return data, nil
}

func (i embyItem) mediaItem() MediaItem {
	out := MediaItem{Title: i.Name, AddedAt: i.DateCreated}
	switch i.Type {
	case "Movie":
		out.Kind, out.Detail = "movie", "Film"
	case "Series":
		out.Kind, out.Detail = "episode", "Série"
	case "Episode":
		out.Kind = "episode"
		if i.SeriesName != "" {
			out.Title = i.SeriesName
		}
		out.Detail = episodeCode(i)
	default:
		out.Kind, out.Detail = "episode", i.Type
	}
	return out
}
