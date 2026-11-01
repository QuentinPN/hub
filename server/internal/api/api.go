// Package api serves the dashboard's JSON API and the embedded web UI.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Store is where the API reads the inventory from.
type Store interface {
	Home() Home
	Service(slug string) (ServiceDetail, bool)
}

// NewHandler returns the full HTTP handler: API routes under /api/v1, health check, and
// ui for everything else. scriptHashes are CSP sources ('sha256-…') for the UI's inline
// bootstrap scripts.
func NewHandler(store Store, ui http.Handler, scriptHashes []string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /api/v1/home", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, store.Home())
	})
	mux.HandleFunc("GET /api/v1/services/{slug}", func(w http.ResponseWriter, r *http.Request) {
		svc, ok := store.Service(r.PathValue("slug"))
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found"})
			return
		}
		writeJSON(w, http.StatusOK, svc)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})
	mux.Handle("/", ui)

	return securityHeaders(mux, contentSecurityPolicy(scriptHashes))
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
