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

// Widgets of an Emby connection.
const (
	EmbySessions = "sessions"
	EmbyLatest   = "latest"
	EmbyLibrary  = "library"

	embyOptionUser     = "userId"
	embyTicksPerMinute = 600_000_000
	embyLatestLimit    = 12
	// embyLatestScan is how many episodes are read to fill embyLatestLimit series: a whole
	// season added at once takes one entry.
	embyLatestScan = 100
)

// The routes come from dev.emby.media/reference/RestAPI; the /emby prefix from its API key
// page's example. Only the fields kept below are read.
type emby struct{}

func (emby) Manifest() Manifest {
	return Manifest{
		Kind:    "emby",
		Name:    "Emby",
		KeyHelp: "Dans Emby : tableau de bord du serveur, Clés d’API. Crée une clé réservée à Hub : tu pourras la révoquer seule.",
		Reads:   []string{"GET /emby/Sessions", "GET /emby/Users/Query", "GET /emby/Users/…/Items/Latest", "GET /emby/Items/Counts", "GET /emby/Items/…/Images/Primary et Backdrop : affiches et fonds", "WebSocket : signaux de changement"},
		Widgets: []Widget{
			{ID: EmbySessions, Label: "Sessions en cours", Where: "accueil et fiche"},
			{ID: EmbyLatest, Label: "Ajoutés récemment", Where: "accueil et fiche"},
			{ID: EmbyLibrary, Label: "Bibliothèque", Where: "accueil et fiche"},
		},
	}
}

func (emby) Authorize(req *http.Request, key secrets.Value) {
	req.Header.Set("X-Emby-Token", key.Reveal())
}

type embyItem struct {
	ID                string
	SeriesID          string `json:"SeriesId"`
	Name              string
	Type              string
	SeriesName        string
	ParentIndexNumber int
	IndexNumber       int
	RunTimeTicks      int64
	DateCreated       string
	// Images: an empty tag list means the item has none of that kind.
	ImageTags               map[string]string
	BackdropImageTags       []string
	SeriesPrimaryImageTag   string
	ParentBackdropItemID    string `json:"ParentBackdropItemId"`
	ParentBackdropImageTags []string
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
	// Poster and Backdrop name the images to ask for: see PosterRequest.
	Poster   string `json:"poster,omitempty"`
	Backdrop string `json:"backdrop,omitempty"`
}

// MediaItem is an entry of a library.
type MediaItem struct {
	Title   string `json:"title"`
	Detail  string `json:"detail"`
	Kind    string `json:"kind"`
	AddedAt string `json:"addedAt"`
	// Poster names the image to ask for: see PosterRequest.
	Poster string `json:"poster,omitempty"`
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
	out.Poster, out.Backdrop = item.images()
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

// images names what plays. An episode shows its series: its own image is a still of the
// episode, and it seldom has a backdrop of its own.
func (i embyItem) images() (poster, backdrop string) {
	if i.SeriesID != "" {
		if i.SeriesPrimaryImageTag != "" {
			poster = i.SeriesID
		}
	} else if i.ImageTags["Primary"] != "" {
		poster = i.ID
	}
	switch {
	case i.SeriesID != "" && i.ParentBackdropItemID != "" && len(i.ParentBackdropImageTags) > 0:
		backdrop = i.ParentBackdropItemID
	case len(i.BackdropImageTags) > 0:
		backdrop = i.ID
	case i.ParentBackdropItemID != "" && len(i.ParentBackdropImageTags) > 0:
		backdrop = i.ParentBackdropItemID
	default:
		return poster, ""
	}
	return poster, backdrop + embyBackdropSuffix
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
		// Grouped, a series comes back as the series itself, dated when its first episode was
		// added: episodes come back one by one and the newest of each series is kept.
		query := url.Values{"Limit": {strconv.Itoa(embyLatestScan)}, "Fields": {"DateCreated"}, "GroupItems": {"false"}}
		if err := c.GetJSON(ctx, "/emby/Users/"+url.PathEscape(user)+"/Items/Latest", query, &found); err != nil {
			return nil, err
		}
		data[EmbyLatest] = latest(found)
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

// latest keeps the newest episode of each series, newest first, embyLatestLimit at most.
func latest(found []embyItem) []MediaItem {
	slices.SortStableFunc(found, func(a, b embyItem) int { return addedAt(b).Compare(addedAt(a)) })
	list := make([]MediaItem, 0, min(len(found), embyLatestLimit))
	seen := map[string]bool{}
	for _, i := range found {
		key := i.ID
		if i.SeriesID != "" {
			key = "series:" + i.SeriesID
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		list = append(list, i.mediaItem())
		if len(list) == embyLatestLimit {
			break
		}
	}
	return list
}

// addedAt is zero for a date it cannot read, which sorts it last.
func addedAt(i embyItem) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, i.DateCreated)
	return t
}

func (i embyItem) mediaItem() MediaItem {
	out := MediaItem{Title: i.Name, AddedAt: i.DateCreated, Poster: i.ID}
	// An episode shows its series' poster: its own image is a landscape still.
	if i.SeriesID != "" {
		out.Poster = i.SeriesID
	}
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
