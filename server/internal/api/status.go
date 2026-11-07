package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Levels of a check: a "bad" one needs a hand, a "warn" one may sort itself out.
const (
	LevelOK   = "ok"
	LevelWarn = "warn"
	LevelBad  = "bad"
)

// StatusCheck is one line of « État de Hub ». Link names the settings section or page that
// helps: hosts, widgets, services, or none.
type StatusCheck struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Level  string     `json:"level"`
	Detail string     `json:"detail"`
	Since  *time.Time `json:"since"`
	Link   string     `json:"link,omitempty"`
}

// StoreStatus is what the store knows of itself and of what it watches.
type StoreStatus struct {
	DBLatencyMs        int
	MigrationsApplied  int
	MigrationsExpected int
	Checks             []StatusCheck
}

// StatusSource is implemented by stores that can report on their own health.
type StatusSource interface {
	StoreStatus(ctx context.Context) (StoreStatus, error)
	// Ready says whether the database answers with every migration applied.
	Ready(ctx context.Context) error
}

// SecretChecker opens every stored secret, to count those the key file cannot open.
type SecretChecker interface {
	Check(ctx context.Context) (total, unreadable int, err error)
}

// Runtime is what only the running process knows.
type Runtime struct {
	Version   string
	StartedAt time.Time
	Secrets   SecretChecker
	// Streams lists the live connections open now, by service, with when they opened.
	Streams func() map[string]time.Time
	// Backups is nil when no backup service runs: the section and its check stay off.
	Backups Backups
}

// HubStatus answers GET /api/v1/status.
type HubStatus struct {
	Version            string        `json:"version"`
	StartedAt          time.Time     `json:"startedAt"`
	DBLatencyMs        int           `json:"dbLatencyMs"`
	MigrationsApplied  int           `json:"migrationsApplied"`
	MigrationsExpected int           `json:"migrationsExpected"`
	Pages              int           `json:"pages"`
	Problems           int           `json:"problems"`
	Checks             []StatusCheck `json:"checks"`
}

func registerStatus(mux *http.ServeMux, src StatusSource, rt *Runtime, live *Widgets) {
	// Public like /healthz, and as quiet: it says ready or not, nothing more.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if err := src.Ready(r.Context()); err != nil {
			slog.Warn("not ready", "err", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready\n"))
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		st, err := src.StoreStatus(r.Context())
		if err != nil {
			// The database itself may be what fails: say so rather than answer 500.
			st = StoreStatus{Checks: []StatusCheck{{ID: "db", Name: "Base de données", Level: LevelBad, Detail: "Ne répond pas"}}}
			slog.Error("status", "err", err)
		}
		out := HubStatus{DBLatencyMs: st.DBLatencyMs, MigrationsApplied: st.MigrationsApplied,
			MigrationsExpected: st.MigrationsExpected, Checks: st.Checks}
		if rt != nil {
			out.Version, out.StartedAt = rt.Version, rt.StartedAt
			if rt.Secrets != nil {
				out.Checks = append(out.Checks, secretsCheck(r.Context(), rt.Secrets, rt.StartedAt))
			}
			if rt.Streams != nil {
				out.Checks = append(out.Checks, streamsCheck(rt.Streams()))
			}
			if rt.Backups != nil {
				out.Checks = append(out.Checks, backupCheck(r.Context(), rt.Backups, time.Now()))
			}
		}
		if live != nil {
			out.Pages = live.Bus.Subscribers()
		}
		out.Checks = orderChecks(out.Checks)
		for _, c := range out.Checks {
			if c.Level != LevelOK {
				out.Problems++
			}
		}
		writeJSON(w, http.StatusOK, out)
	})
}

var checkOrder = []string{"db", "secrets", "backup", "agents", "widgets", "streams", "urls"}

func orderChecks(list []StatusCheck) []StatusCheck {
	out := make([]StatusCheck, 0, len(list))
	for _, id := range checkOrder {
		for _, c := range list {
			if c.ID == id {
				out = append(out, c)
			}
		}
	}
	return out
}

func secretsCheck(ctx context.Context, s SecretChecker, started time.Time) StatusCheck {
	c := StatusCheck{ID: "secrets", Name: "Clés de chiffrement", Level: LevelOK, Since: &started}
	total, unreadable, err := s.Check(ctx)
	switch {
	case err != nil:
		c.Level, c.Detail = LevelBad, "Lecture impossible"
	case unreadable > 0:
		c.Level, c.Detail = LevelBad, plural(unreadable, "secret illisible", "secrets illisibles")+" avec le fichier de clés"
	case total == 0:
		c.Detail = "Aucun secret enregistré"
	default:
		c.Detail = plural(total, "secret lisible", "secrets lisibles")
	}
	return c
}

func streamsCheck(open map[string]time.Time) StatusCheck {
	c := StatusCheck{ID: "streams", Name: "Écoute en direct", Level: LevelOK}
	if len(open) == 0 {
		c.Detail = "Aucune, les widgets sont relus à leur fréquence"
		return c
	}
	names := make([]string, 0, len(open))
	var oldest time.Time
	for name, at := range open {
		names = append(names, name)
		if oldest.IsZero() || at.Before(oldest) {
			oldest = at
		}
	}
	sortStrings(names)
	c.Detail = strings.Join(names, ", ") + " écoutés"
	c.Since = &oldest
	return c
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return itoa(n) + " " + many
}
