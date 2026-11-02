package api

import (
	"context"
	"encoding/json"
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

	"POST /api/v1/auth/passkey/login/begin":  true,
	"POST /api/v1/auth/passkey/login/finish": true,
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
	// Passkeys tells the login page whether to offer them (they need HUB_ORIGIN).
	Passkeys bool `json:"passkeys"`
}

type PasskeyInfo struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	// BackedUp is a synced passkey (password manager, iCloud…) rather than one bound to
	// a single device.
	BackedUp bool `json:"backedUp"`
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
	registerPasskeys(mux, svc)

	mux.HandleFunc("GET /api/v1/auth/state", func(w http.ResponseWriter, r *http.Request) {
		required, err := svc.SetupRequired(r.Context())
		if err != nil {
			internalError(w, r, err)
			return
		}
		state := AuthState{SetupRequired: required, Passkeys: svc.PasskeysEnabled()}
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

	// The current password is asked again; every other session ends, this one stays.
	mux.HandleFunc("PUT /api/v1/auth/password", func(w http.ResponseWriter, r *http.Request) {
		p, _ := current(r.Context())
		var body struct{ Current, New string }
		if !decodeJSON(w, r, &body) {
			return
		}
		if err := svc.ChangePassword(r.Context(), p.session, body.Current, body.New, clientOf(r)); err != nil {
			passkeyError(w, r, err)
			return
		}
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

	// scope=others keeps this session, scope=all ends it too (sign out everywhere). The
	// scope is required: a bare DELETE must not sign anyone out by accident.
	mux.HandleFunc("DELETE /api/v1/auth/sessions", func(w http.ResponseWriter, r *http.Request) {
		p, _ := current(r.Context())
		scope := r.URL.Query().Get("scope")
		if scope != "others" && scope != "all" {
			writeError(w, http.StatusBadRequest, `scope must be "others" or "all"`)
			return
		}
		ended, err := svc.RevokeAll(r.Context(), p.session, scope == "all", clientOf(r))
		if err != nil {
			internalError(w, r, err)
			return
		}
		if scope == "all" {
			clearSessionCookie(w)
		}
		writeJSON(w, http.StatusOK, struct {
			Ended int `json:"ended"`
		}{ended})
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

func registerPasskeys(mux *http.ServeMux, svc *auth.Service) {
	// Login, public: the options are sent as is to navigator.credentials.get, and the
	// browser's answer comes back unchanged.
	mux.HandleFunc("POST /api/v1/auth/passkey/login/begin", func(w http.ResponseWriter, r *http.Request) {
		assertion, err := svc.BeginPasskeyLogin(clientOf(r))
		if err != nil {
			passkeyError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, assertion)
	})

	mux.HandleFunc("POST /api/v1/auth/passkey/login/finish", func(w http.ResponseWriter, r *http.Request) {
		var credential json.RawMessage
		if !decodeJSON(w, r, &credential) {
			return
		}
		token, err := svc.FinishPasskeyLogin(r.Context(), credential, clientOf(r))
		if errors.Is(err, auth.ErrPasskeysDisabled) {
			passkeyError(w, r, err)
			return
		}
		startSession(w, r, token, err)
	})

	// Management, inside a session.
	mux.HandleFunc("GET /api/v1/auth/passkeys", func(w http.ResponseWriter, r *http.Request) {
		p, _ := current(r.Context())
		list, err := svc.Passkeys(r.Context(), p.user.ID)
		if err != nil {
			passkeyError(w, r, err)
			return
		}
		out := make([]PasskeyInfo, 0, len(list))
		for _, k := range list {
			out = append(out, PasskeyInfo{ID: k.ID, Name: k.Name, CreatedAt: k.CreatedAt, LastUsedAt: k.LastUsedAt, BackedUp: k.Credential.Flags.BackupState})
		}
		writeJSON(w, http.StatusOK, out)
	})

	mux.HandleFunc("POST /api/v1/auth/passkeys/begin", func(w http.ResponseWriter, r *http.Request) {
		p, _ := current(r.Context())
		var body struct{ Password string }
		if !decodeJSON(w, r, &body) {
			return
		}
		creation, err := svc.BeginPasskeyRegistration(r.Context(), p.user, body.Password, clientOf(r))
		if err != nil {
			passkeyError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, creation)
	})

	mux.HandleFunc("POST /api/v1/auth/passkeys/finish", func(w http.ResponseWriter, r *http.Request) {
		p, _ := current(r.Context())
		var body struct {
			Name       string
			Credential json.RawMessage
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		k, err := svc.FinishPasskeyRegistration(r.Context(), p.user, body.Name, body.Credential, clientOf(r))
		if err != nil {
			passkeyError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, PasskeyInfo{ID: k.ID, Name: k.Name, CreatedAt: k.CreatedAt, LastUsedAt: k.LastUsedAt, BackedUp: k.Credential.Flags.BackupState})
	})

	mux.HandleFunc("DELETE /api/v1/auth/passkeys/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, _ := current(r.Context())
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err := svc.DeletePasskey(r.Context(), p.user.ID, id, clientOf(r)); err != nil {
			passkeyError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// passkeyError maps passkey and password-change errors. A wrong password inside a session
// is 403, not 401:
// the UI treats 401 as an expired session and goes back to the login page.
func passkeyError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *auth.InvalidInputError
	var limited *auth.RateLimitedError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Msg)
	case errors.As(err, &limited):
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(time.Until(limited.Until).Seconds()))))
		writeError(w, http.StatusTooManyRequests, "too many attempts")
	case errors.Is(err, auth.ErrWrongPassword):
		writeError(w, http.StatusForbidden, "wrong password")
	case errors.Is(err, auth.ErrPasskeyRejected):
		writeError(w, http.StatusBadRequest, "passkey rejected")
	case errors.Is(err, auth.ErrPasskeyExists):
		writeError(w, http.StatusConflict, "passkey already registered")
	case errors.Is(err, auth.ErrPasskeysDisabled), errors.Is(err, auth.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	default:
		internalError(w, r, err)
	}
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
