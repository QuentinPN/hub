package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"hub/server/internal/auth"
	"hub/server/internal/secrets"
	"hub/server/internal/widgets"
)

// hookPrefix is where services send their webhooks: the only route a server, not a browser,
// calls without a session.
const hookPrefix = "/api/v1/hooks/"

const (
	// maxHookBody bounds a webhook: its content is only a signal.
	maxHookBody        = 64 << 10
	webhookSecretBytes = 32
	searchMinChars     = 2
	searchMaxChars     = 100

	notConnected = "ce service n'est pas connecté"
)

// isHook is a webhook: public, authenticated by its own secret, and sent by a server, which
// sets neither Origin nor Sec-Fetch-Site.
func isHook(r *http.Request) bool {
	slug, ok := strings.CutPrefix(r.URL.Path, hookPrefix)
	return r.Method == http.MethodPost && ok && slug != "" && !strings.Contains(slug, "/")
}

func hasWebhook(kind string) bool {
	m, ok := widgets.ManifestOf(kind)
	return ok && m.Webhook
}

// WebhookSecret is shown once, when made: what to paste in the service's settings.
type WebhookSecret struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

// originOf is the dashboard's public address: HUB_ORIGIN, or else the one the request came to.
func originOf(r *http.Request, origin string) string {
	if origin != "" {
		return strings.TrimRight(origin, "/")
	}
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// actionError words why a request to the service failed. A key it refuses is not the user's
// session: never a 401, which would sign them out.
func actionError(w http.ResponseWriter, err error) {
	var refusal *widgets.Refusal
	switch {
	case errors.Is(err, widgets.ErrNoActions):
		writeError(w, http.StatusBadRequest, "ce service n'a pas d'action depuis Hub")
	case errors.As(err, &refusal) && refusal.Status == http.StatusForbidden:
		writeError(w, http.StatusForbidden, "La clé Request n'a pas le droit de faire cette action.")
	case errors.As(err, &refusal) && refusal.Status == http.StatusUnauthorized:
		writeError(w, http.StatusBadGateway, "Request refuse la clé de Hub : reconnecte le service.")
	case errors.As(err, &refusal):
		msg := refusal.Message
		if msg == "" {
			msg = "Request a refusé la demande."
		}
		writeError(w, refusal.Status, msg)
	default:
		writeError(w, http.StatusBadGateway, "Request ne répond pas.")
	}
}

// auditAction notes what was asked of a service. Not in the service's history: these are
// the service's own events, which it keeps.
func auditAction(r *http.Request, authSvc *auth.Service, kind string) {
	auditEvent(r, authSvc, kind, r.PathValue("slug"))
}

// auditEvent records an event of the signed-in user in the audit log.
func auditEvent(r *http.Request, authSvc *auth.Service, kind, detail string) {
	p, _ := current(r.Context())
	authSvc.Audit(r.Context(), auth.Event{Kind: kind, UserID: &p.user.ID, IP: clientOf(r).IP, Detail: detail})
}

// requestID is the {id} of the path: a positive integer, so the service's path is built from
// a number and nothing else.
func requestID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 || id > math.MaxInt32 {
		writeError(w, http.StatusBadRequest, "demande inconnue")
		return 0, false
	}
	return id, true
}

// registerActions serves what the user asks a connected service to do. The connection is read
// again at once after a write, so every open page sees it.
func registerActions(mux *http.ServeMux, live *Widgets, authSvc *auth.Service,
	connected func(*http.Request) (widgets.Target, secrets.Value, bool)) {
	// act runs do on the connection, then wakes it and answers status.
	act := func(w http.ResponseWriter, r *http.Request, kind string, status int, do func(context.Context, widgets.Target, secrets.Value) error) {
		where, key, ok := connected(r)
		if !ok {
			writeError(w, http.StatusBadRequest, notConnected)
			return
		}
		if err := do(r.Context(), where, key); err != nil {
			actionError(w, err)
			return
		}
		auditAction(r, authSvc, kind)
		live.Wake(r.PathValue("slug"))
		w.WriteHeader(status)
	}

	mux.HandleFunc("POST /api/v1/services/{slug}/requests/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestID(w, r)
		if !ok {
			return
		}
		act(w, r, "request_approved", http.StatusNoContent, func(ctx context.Context, t widgets.Target, key secrets.Value) error {
			return widgets.Approve(ctx, t, key, id)
		})
	})

	mux.HandleFunc("POST /api/v1/services/{slug}/requests/{id}/decline", func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestID(w, r)
		if !ok {
			return
		}
		var body struct{ Reason string }
		if !decodeJSON(w, r, &body) {
			return
		}
		reason := strings.TrimSpace(body.Reason)
		if reason == "" || utf8.RuneCountInString(reason) > widgets.DeclineReasonMax {
			writeError(w, http.StatusBadRequest, "donne un motif de 1 à "+strconv.Itoa(widgets.DeclineReasonMax)+" caractères")
			return
		}
		act(w, r, "request_declined", http.StatusNoContent, func(ctx context.Context, t widgets.Target, key secrets.Value) error {
			return widgets.Decline(ctx, t, key, id, reason)
		})
	})

	mux.HandleFunc("POST /api/v1/services/{slug}/requests", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			MediaType string `json:"mediaType"`
			TmdbID    int    `json:"tmdbId"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.MediaType != widgets.MediaMovie && body.MediaType != widgets.MediaTV || body.TmdbID <= 0 || body.TmdbID > math.MaxInt32 {
			writeError(w, http.StatusBadRequest, "titre inconnu")
			return
		}
		act(w, r, "request_created", http.StatusCreated, func(ctx context.Context, t widgets.Target, key secrets.Value) error {
			return widgets.Request(ctx, t, key, body.MediaType, body.TmdbID)
		})
	})

	mux.HandleFunc("GET /api/v1/services/{slug}/search", func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if n := utf8.RuneCountInString(q); n < searchMinChars || n > searchMaxChars {
			writeError(w, http.StatusBadRequest, "cherche avec "+strconv.Itoa(searchMinChars)+" à "+strconv.Itoa(searchMaxChars)+" caractères")
			return
		}
		where, key, ok := connected(r)
		if !ok {
			writeError(w, http.StatusBadRequest, notConnected)
			return
		}
		found, err := widgets.Search(r.Context(), where, key, q)
		if err != nil {
			actionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string][]widgets.Found{"items": found})
	})
}
