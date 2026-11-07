package api

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
	// Time zones are checked against the binary's own list, whatever the image carries.
	_ "time/tzdata"

	"hub/server/internal/auth"
)

// Pages Hub can open on, and the two widget refresh modes.
const (
	StartHome      = "home"
	StartMedias    = "medias"
	StartDownloads = "downloads"
	StartInfra     = "infra"

	RefreshNormal  = "normal"
	RefreshEconomy = "economy"
)

const (
	DefaultDisplayName = "Hub"
	MaxDisplayNameLen  = 40
)

// General is the « Général » section, as stored.
type General struct {
	// DisplayName shows in the browser tab and on the login screen.
	DisplayName string `json:"displayName"`
	StartPage   string `json:"startPage"`
	// TimeZone is an IANA name; empty for the server's own (TZ).
	TimeZone      string `json:"timeZone"`
	WidgetRefresh string `json:"widgetRefresh"`
	// HistoryDays is how long the services' history is kept; 0 in a request keeps it as is.
	HistoryDays int `json:"historyDays"`
}

// DefaultGeneral is what a server shows before anything is saved.
func DefaultGeneral() General {
	return General{DisplayName: DefaultDisplayName, StartPage: StartHome, WidgetRefresh: RefreshNormal, HistoryDays: DefaultHistoryDays}
}

var zoneName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+-]*(/[A-Za-z0-9_+-]+)*$`)

// ValidDisplayName says what is wrong with a name, "" when it can be kept. Only printable
// characters: no control, no invisible or direction mark that would disguise the tab.
func ValidDisplayName(name string) string {
	switch n := utf8.RuneCountInString(name); {
	case !utf8.ValidString(name):
		return "nom affiché illisible"
	case strings.TrimSpace(name) == "":
		return "nom affiché vide"
	case n > MaxDisplayNameLen:
		return "nom affiché trop long (40 caractères au plus)"
	case name != strings.TrimSpace(name):
		return "nom affiché entouré d'espaces"
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return "nom affiché : caractère non autorisé"
		}
	}
	return ""
}

// ValidTimeZone says what is wrong with a zone name, "" when the server knows it. Empty
// means the server's own.
func ValidTimeZone(name string) string {
	if name == "" {
		return ""
	}
	if len(name) > 64 || !zoneName.MatchString(name) || name == "Local" {
		return "fuseau horaire inconnu"
	}
	if _, err := time.LoadLocation(name); err != nil {
		return "fuseau horaire inconnu"
	}
	return ""
}

// Validate trims the name, then says what is wrong, "" when nothing is.
func (g *General) Validate() string {
	g.DisplayName = strings.TrimSpace(g.DisplayName)
	if msg := ValidDisplayName(g.DisplayName); msg != "" {
		return msg
	}
	switch g.StartPage {
	case StartHome, StartMedias, StartDownloads, StartInfra:
	default:
		return "page d'ouverture inconnue"
	}
	if msg := ValidTimeZone(g.TimeZone); msg != "" {
		return msg
	}
	if g.WidgetRefresh != RefreshNormal && g.WidgetRefresh != RefreshEconomy {
		return "rafraîchissement inconnu"
	}
	if !slices.Contains(HistoryRetentions, g.HistoryDays) {
		return "durée de conservation inconnue"
	}
	return ""
}

// About is what the server was started with, from its .env: read only. An empty field is
// one the server does not know.
type About struct {
	Version string `json:"version"`
	// Origin is HUB_ORIGIN.
	Origin string `json:"origin"`
	// AgentURL is what the agents dial; empty while their listener is off.
	AgentURL string `json:"agentUrl"`
	// Host is HUB_HOST_NAME: from inside its container, Hub cannot tell the machine's name.
	Host string `json:"host"`
	// ServerTimeZone is TZ.
	ServerTimeZone string `json:"serverTimeZone"`
}

// GeneralSettings answers GET /api/v1/settings/general.
type GeneralSettings struct {
	General
	About About `json:"about"`
}

// GeneralStore keeps the « Général » section.
type GeneralStore interface {
	General(ctx context.Context) (General, error)
	SaveGeneral(ctx context.Context, g General) error
}

func registerGeneral(mux *http.ServeMux, gs GeneralStore, rt *Runtime, authSvc *auth.Service) {
	var about About
	if rt != nil {
		about = rt.About
		about.Version = rt.Version
	}
	read := func(ctx context.Context) (GeneralSettings, error) {
		g, err := gs.General(ctx)
		return GeneralSettings{General: g, About: about}, err
	}
	mux.HandleFunc("GET /api/v1/settings/general", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(read(r.Context()))
	})
	mux.HandleFunc("PUT /api/v1/settings/general", func(w http.ResponseWriter, r *http.Request) {
		var g General
		if !decodeJSON(w, r, &g) {
			return
		}
		if g.HistoryDays == 0 {
			cur, err := gs.General(r.Context())
			if err != nil {
				internalError(w, r, err)
				return
			}
			g.HistoryDays = cur.HistoryDays
		}
		if msg := g.Validate(); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		if err := gs.SaveGeneral(r.Context(), g); err != nil {
			internalError(w, r, err)
			return
		}
		p, _ := current(r.Context())
		authSvc.Audit(r.Context(), auth.Event{Kind: "settings_updated", UserID: &p.user.ID, IP: clientOf(r).IP, Detail: "general"})
		respond(w, r)(read(r.Context()))
	})
}
