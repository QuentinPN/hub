package api

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Kinds of history events, as stored and as the tab filters them.
const (
	HistoryOutage    = "outage"
	HistoryContainer = "container"
	HistoryVersion   = "version"
	HistoryEdit      = "edit"
	HistoryAlert     = "alert"
)

// HistoryKinds are the kinds in the order the tab shows its filters.
var HistoryKinds = []string{HistoryOutage, HistoryContainer, HistoryVersion, HistoryEdit, HistoryAlert}

// Outage actions: the service went down, or came back.
const (
	OutageDown = "down"
	OutageUp   = "up"
)

// Retention choices of « Général », in days.
var HistoryRetentions = []int{30, 90, 180, 365}

const (
	DefaultHistoryDays = 90
	// HistoryBarDays is the length of the availability bar, whatever the retention.
	HistoryBarDays   = 90
	historyPageSize  = 25
	historyPageLimit = 100
)

// HistoryQuery is one page of the events: Kinds empty for all of them.
type HistoryQuery struct {
	Kinds  []string
	Offset int
	Limit  int
}

// ServiceHistory answers GET /api/v1/services/{slug}/history.
type ServiceHistory struct {
	// Since is when Hub started keeping this service's history: nothing before it is known.
	Since         time.Time `json:"since"`
	RetentionDays int       `json:"retentionDays"`
	// Days are the last HistoryBarDays days in the server's time zone, oldest first.
	Days []HistoryDay `json:"days"`
	// Counts are the events kept, by kind, whatever the filter.
	Counts map[string]int `json:"counts"`
	// Total is the number of events the filter keeps; Events is the page asked for.
	Total  int            `json:"total"`
	Offset int            `json:"offset"`
	Limit  int            `json:"limit"`
	Events []HistoryEvent `json:"events"`
}

// HistoryDay is one day of the availability bar, in minutes measured.
type HistoryDay struct {
	Date           string `json:"date"`
	UpMinutes      int    `json:"upMinutes"`
	DownMinutes    int    `json:"downMinutes"`
	UnknownMinutes int    `json:"unknownMinutes"`
	// LatencyMs is the mean of the address's answers that day; nil without one.
	LatencyMs *int `json:"latencyMs"`
	// Outages is the number that started that day.
	Outages int `json:"outages"`
}

// HistoryEvent is one line of the timeline; the fields beyond Action depend on Kind.
type HistoryEvent struct {
	ID     int64     `json:"id"`
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Action string    `json:"action"`
	// Reason is Hub's own words for an outage (« adresse injoignable »).
	Reason string `json:"reason,omitempty"`
	// Minutes is how long an outage lasted, on both its ends; nil while it goes on.
	Minutes   *int   `json:"minutes,omitempty"`
	Container string `json:"container,omitempty"`
	ExitCode  *int   `json:"exitCode,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Image     string `json:"image,omitempty"`
	// By is who made an edit in Hub; empty once the account is gone.
	By string `json:"by,omitempty"`
	// Alert, Title and Via describe a message sent: its kind, its title, the channels it
	// went through (« digest » when kept for the morning).
	Alert string   `json:"alert,omitempty"`
	Title string   `json:"title,omitempty"`
	Via   []string `json:"via,omitempty"`
}

// HistoryStore keeps the services' history. Without it, the route is not registered.
type HistoryStore interface {
	// ServiceHistory returns ErrNotFound for an unknown service.
	ServiceHistory(ctx context.Context, slug string, q HistoryQuery, now time.Time) (ServiceHistory, error)
	// RecordServiceEdit notes an edit made in Hub, by the audit kind that names it.
	RecordServiceEdit(ctx context.Context, slug, kind string, userID *int64, at time.Time) error
}

// recordEdit adds an audited edit to the service's history, when the store keeps one. A
// failure is logged: the edit itself is done.
func recordEdit(r *http.Request, store any, slug, kind string, userID *int64) {
	hs, ok := store.(HistoryStore)
	if !ok {
		return
	}
	if err := hs.RecordServiceEdit(r.Context(), slug, kind, userID, time.Now()); err != nil {
		slog.Error("history", "service", slug, "err", err)
	}
}

func registerHistory(mux *http.ServeMux, hs HistoryStore) {
	mux.HandleFunc("GET /api/v1/services/{slug}/history", func(w http.ResponseWriter, r *http.Request) {
		q, msg := parseHistoryQuery(r)
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		respond(w, r)(hs.ServiceHistory(r.Context(), r.PathValue("slug"), q, time.Now()))
	})
}

func parseHistoryQuery(r *http.Request) (HistoryQuery, string) {
	v := r.URL.Query()
	q := HistoryQuery{Limit: historyPageSize}
	if s := v.Get("kinds"); s != "" {
		for _, k := range strings.Split(s, ",") {
			if !slices.Contains(HistoryKinds, k) {
				return q, "type d'événement inconnu"
			}
			if !slices.Contains(q.Kinds, k) {
				q.Kinds = append(q.Kinds, k)
			}
		}
	}
	if s := v.Get("offset"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > 1_000_000 {
			return q, "offset invalide"
		}
		q.Offset = n
	}
	if s := v.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > historyPageLimit {
			return q, "limit invalide (1 à 100)"
		}
		q.Limit = n
	}
	return q, ""
}
