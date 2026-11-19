// Package api serves the dashboard's JSON API and the embedded web UI.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"net/url"

	"hub/server/internal/agentsrv"
	"hub/server/internal/auth"
	"hub/server/internal/icons"
	"hub/server/internal/urlcheck"
)

// Store is where the API reads the inventory from.
type Store interface {
	Home(ctx context.Context) (Home, error)
	Services(ctx context.Context) ([]ServiceSummary, error)
	// Service returns ErrNotFound when the slug is unknown.
	Service(ctx context.Context, slug string) (ServiceDetail, error)
	Settings(ctx context.Context) (Settings, error)
}

// SettingsWriter is implemented by stores that persist settings. Without it, the write
// routes are not registered and the UI shows settings as read-only.
type SettingsWriter interface {
	UpdateDiscovery(ctx context.Context, d Discovery) error
}

// IconSource resolves a selfh.st icon reference; icons.ErrNotFound when there is none.
type IconSource interface {
	Get(ctx context.Context, ref string) (icons.Icon, error)
}

// ErrNotFound is returned by a Store for an unknown resource.
var ErrNotFound = errors.New("not found")

// NewHandler returns the full HTTP handler: API routes under /api/v1, all behind a session
// except the auth routes in publicAPI, a health check, and ui for everything else.
// scriptHashes are CSP sources ('sha256-…') for the UI's inline bootstrap scripts.
// agents is nil while the agent listener is off.
// live is nil when connections cannot be stored: the widget routes stay off.
// rt is what only the running process knows, for « État de Hub »; nil in most tests.
func NewHandler(store Store, authSvc *auth.Service, iconSrc IconSource, agents *Agents, live *Widgets, rt *Runtime, ui http.Handler, scriptHashes []string) http.Handler {
	mux := http.NewServeMux()
	gs, _ := store.(GeneralStore)
	registerAuth(mux, authSvc, gs)
	if hr, ok := store.(HostReader); ok {
		registerHosts(mux, hr, agents, func(r *http.Request) agentsrv.Actor {
			p, _ := current(r.Context())
			return agentsrv.Actor{UserID: &p.user.ID, IP: clientOf(r).IP}
		})
	}

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /api/v1/home", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(store.Home(r.Context()))
	})
	mux.HandleFunc("GET /api/v1/services", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(store.Services(r.Context()))
	})
	mux.HandleFunc("GET /api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(store.Settings(r.Context()))
	})
	mux.HandleFunc("GET /api/v1/services/{slug}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(store.Service(r.Context(), r.PathValue("slug")))
	})
	// Behind the session like the rest of /api/: the server never acts as an open proxy.
	mux.HandleFunc("GET /api/v1/icons/{ref}", func(w http.ResponseWriter, r *http.Request) {
		icon, err := iconSrc.Get(r.Context(), r.PathValue("ref"))
		switch {
		case errors.Is(err, icons.ErrNotFound):
			writeError(w, http.StatusNotFound, "not found")
			return
		case err != nil:
			internalError(w, r, err)
			return
		}
		h := w.Header()
		h.Set("Content-Type", icon.ContentType)
		h.Set("Cache-Control", "private, max-age=86400")
		// Opened on its own, an SVG renders as a document: no script, no request, sandboxed.
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		_, _ = w.Write(icon.Body)
	})
	if editor, ok := store.(ServiceEditor); ok {
		registerServices(mux, store, editor, authSvc, urlcheck.Check)
	}
	if editor, ok := store.(WidgetEditor); ok && live != nil {
		registerWidgets(mux, store, editor, live, authSvc)
	}
	if src, ok := store.(StatusSource); ok {
		registerStatus(mux, src, rt, live)
	}
	if rt != nil && rt.Backups != nil {
		registerBackups(mux, rt.Backups, authSvc)
	}
	if rt != nil && rt.Notifications != nil {
		registerNotifications(mux, rt.Notifications, authSvc)
	}
	if editor, ok := store.(LayoutEditor); ok {
		registerLayout(mux, editor)
	}
	if gs != nil {
		registerGeneral(mux, gs, rt, authSvc)
	}
	if sections, ok := store.(SectionReader); ok {
		registerSections(mux, sections)
	}
	if hs, ok := store.(HistoryStore); ok {
		registerHistory(mux, hs)
	}
	if sw, ok := store.(SettingsWriter); ok {
		mux.HandleFunc("PUT /api/v1/settings/discovery", func(w http.ResponseWriter, r *http.Request) {
			var d Discovery
			if !decodeJSON(w, r, &d) {
				return
			}
			if msg := d.Validate(); msg != "" {
				writeError(w, http.StatusBadRequest, msg)
				return
			}
			if err := sw.UpdateDiscovery(r.Context(), d); err != nil {
				internalError(w, r, err)
				return
			}
			p, _ := current(r.Context())
			authSvc.Audit(r.Context(), auth.Event{Kind: "settings_updated", UserID: &p.user.ID, IP: clientOf(r).IP, Detail: "discovery"})
			respond(w, r)(store.Settings(r.Context()))
		})
	}
	if cr, ok := store.(ContainerReader); ok {
		registerContainers(mux, cr)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	mux.Handle("/", ui)

	return logRequests(securityHeaders(sameOrigin(requireSession(authSvc, mux)), contentSecurityPolicy(scriptHashes)))
}

// respond writes v as JSON, or the matching error status.
func respond(w http.ResponseWriter, r *http.Request) func(any, error) {
	return func(v any, err error) {
		switch {
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "not found")
		case err != nil:
			internalError(w, r, err)
		default:
			writeJSON(w, http.StatusOK, v)
		}
	}
}

// internalError logs the cause and never sends it to the client.
func internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

const maxBody = 16 << 10

// decodeJSON accepts only a small application/json body with no unknown field. The
// content type also forces a CORS preflight, which this server never answers.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "expected application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// sameOrigin rejects state-changing requests sent by another site (CSRF). Browsers set
// Sec-Fetch-Site and Origin on such requests; a missing header is accepted for non-browser
// clients, which still need a session cookie. A webhook carries no cookie to abuse: its own
// secret authenticates it.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if isHook(r) {
			next.ServeHTTP(w, r)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
			writeError(w, http.StatusForbidden, "cross-site request refused")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host {
				writeError(w, http.StatusForbidden, "cross-origin request refused")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// contentSecurityPolicy follows SECURITY.md: same-origin only. Inline styles are allowed
// because Svelte's style: directives write them; inline scripts only by hash.
func contentSecurityPolicy(scriptHashes []string) string {
	scriptSrc := "'self'"
	for _, h := range scriptHashes {
		scriptSrc += " '" + h + "'"
	}
	return "default-src 'self'; script-src " + scriptSrc + "; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response", "err", err)
	}
}

func securityHeaders(next http.Handler, csp string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
