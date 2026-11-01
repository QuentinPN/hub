package api

import (
	"context"
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hub/server/internal/auth"
)

// SessionCookie uses the __Host- prefix: the browser only accepts it with Secure, Path=/
// and no Domain, so no other host or path can plant or read it.
const SessionCookie = "__Host-hub_session"

// publicAPI are the only /api/ routes reachable without a session.
var publicAPI = map[string]bool{
	"GET /api/v1/auth/state":  true,
	"POST /api/v1/auth/setup": true,
	"POST /api/v1/auth/login": true,
}

type ctxKey struct{}

type principal struct {
	session auth.Session
	user    auth.User
}

func current(ctx context.Context) (principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(principal)
	return p, ok
}

// clientOf identifies the caller. RemoteAddr is used as is: behind Caddy every client
// shares the proxy's address, which turns the per-client login limit into a global one.
// For a single account on a LAN that is acceptable (DECISIONS.md).
func clientOf(r *http.Request) auth.Client {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return auth.Client{IP: ip, UserAgent: r.UserAgent()}
}

// requireSession lets public routes and the UI through, and every other /api/ request
// only with a valid session. Deny by default: a new route is protected without opt-in.
func requireSession(svc *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(SessionCookie)
		if err == nil {
			sess, user, err := svc.Authenticate(r.Context(), cookie.Value)
			switch {
			case err == nil:
				setSessionCookie(w, cookie.Value, sess.ExpiresAt)
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, principal{sess, user})))
				return
			case !errors.Is(err, auth.ErrUnauthenticated):
				internalError(w, r, err)
				return
			}
			clearSessionCookie(w)
		}
		if publicAPI[r.Method+" "+r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusUnauthorized, "authentication required")
	})
}

func setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(math.Max(1, time.Until(expires).Seconds())),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
}

type AuthState struct {
	SetupRequired bool   `json:"setupRequired"`
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username,omitempty"`
}

type SessionInfo struct {
	ID         int64     `json:"id"`
	Current    bool      `json:"current"`
	UserAgent  string    `json:"userAgent"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

func registerAuth(mux *http.ServeMux, svc *auth.Service) {
	mux.HandleFunc("GET /api/v1/auth/state", func(w http.ResponseWriter, r *http.Request) {
		required, err := svc.SetupRequired(r.Context())
		if err != nil {
			internalError(w, r, err)
			return
		}
		state := AuthState{SetupRequired: required}
		if p, ok := current(r.Context()); ok {
			state.Authenticated, state.Username = true, p.user.Username
		}
		writeJSON(w, http.StatusOK, state)
	})

	mux.HandleFunc("POST /api/v1/auth/setup", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Code, Username, Password string }
		if !decodeJSON(w, r, &body) {
			return
		}
		token, err := svc.Setup(r.Context(), body.Code, body.Username, body.Password, clientOf(r))
		startSession(w, r, token, err)
	})

	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Username, Password string }
		if !decodeJSON(w, r, &body) {
			return
		}
		token, err := svc.Login(r.Context(), body.Username, body.Password, clientOf(r))
		startSession(w, r, token, err)
	})

	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		p, _ := current(r.Context())
		if err := svc.Logout(r.Context(), p.session, clientOf(r)); err != nil {
			internalError(w, r, err)
			return
		}
		clearSessionCookie(w)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/v1/auth/sessions", func(w http.ResponseWriter, r *http.Request) {
		p, _ := current(r.Context())
		list, err := svc.Sessions(r.Context(), p.user.ID)
		if err != nil {
			internalError(w, r, err)
			return
		}
		out := make([]SessionInfo, 0, len(list))
		for _, s := range list {
			out = append(out, SessionInfo{ID: s.ID, Current: s.ID == p.session.ID, UserAgent: s.UserAgent, IP: s.IP, CreatedAt: s.CreatedAt, LastSeenAt: s.LastSeenAt})
		}
		writeJSON(w, http.StatusOK, out)
	})

	mux.HandleFunc("DELETE /api/v1/auth/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, _ := current(r.Context())
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		switch err := svc.Revoke(r.Context(), p.user.ID, id, clientOf(r)); {
		case errors.Is(err, auth.ErrNotFound):
			writeError(w, http.StatusNotFound, "not found")
		case err != nil:
			internalError(w, r, err)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
}

// startSession sets the cookie after a successful setup or login, or maps the error.
func startSession(w http.ResponseWriter, r *http.Request, token string, err error) {
	var invalid *auth.InvalidInputError
	var limited *auth.RateLimitedError
	switch {
	case err == nil:
		setSessionCookie(w, token, time.Now().Add(auth.SessionTTL))
		w.WriteHeader(http.StatusNoContent)
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Msg)
	case errors.As(err, &limited):
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(time.Until(limited.Until).Seconds()))))
		writeError(w, http.StatusTooManyRequests, "too many attempts")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid credentials")
	case errors.Is(err, auth.ErrInvalidSetupCode):
		writeError(w, http.StatusForbidden, "invalid setup code")
	case errors.Is(err, auth.ErrSetupDone):
		writeError(w, http.StatusConflict, "account already exists")
	default:
		internalError(w, r, err)
	}
}
