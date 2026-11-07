package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"hub/server/internal/auth"
	"hub/server/internal/urlcheck"
)

// ServiceEditor is implemented by stores that keep the user's corrections. Without it, the
// write routes are not registered.
type ServiceEditor interface {
	SetFavorite(ctx context.Context, slug string, favorite bool) error
	// UpdateService returns ErrInvalidLink when a link is not a project free on the host.
	UpdateService(ctx context.Context, slug string, edit ServiceEdit) error
	ResetService(ctx context.Context, slug string) error
	SaveServiceURL(ctx context.Context, slug, url, certSHA256 string, r urlcheck.Result) error
	ClearServiceURL(ctx context.Context, slug string) error
	HiddenGroups(ctx context.Context) ([]HiddenGroup, error)
	// ShowHidden returns the service's slug.
	ShowHidden(ctx context.Context, hostID, key string) (string, error)
}

// ErrInvalidLink refuses a link to a project that cannot be linked.
var ErrInvalidLink = errors.New("invalid link")

// ServiceSplitter is implemented by stores that can split a Compose service from its
// project. Without it, the split routes are not registered.
type ServiceSplitter interface {
	// SplitService returns the new service's slug, or ErrInvalidSplit when the project
	// would be left with no service of its own or split is not one of its Compose services.
	SplitService(ctx context.Context, slug string, split ServiceSplit) (string, error)
	// JoinService returns the project's service slug, "" if it has none; ErrInvalidSplit
	// when slug was not split.
	JoinService(ctx context.Context, slug string) (string, error)
}

// ErrInvalidSplit refuses a split, or a join, that the containers do not allow.
var ErrInvalidSplit = errors.New("invalid split")

func (s *ServiceSplit) validate() string {
	edit := ServiceEdit{Name: s.Name, Category: s.Category}
	msg := edit.validate()
	s.Name, s.Category, s.Service = edit.Name, edit.Category, strings.TrimSpace(s.Service)
	if msg == "" && s.Service == "" {
		return "service Compose manquant"
	}
	return msg
}

// URLChecker verifies an address; the tests replace it.
type URLChecker func(ctx context.Context, url, pinned string) urlcheck.Result

var (
	iconRef         = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	certFingerprint = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func (e *ServiceEdit) validate() string {
	e.Name, e.Category = strings.TrimSpace(e.Name), strings.TrimSpace(e.Category)
	switch {
	case e.Name == "" || utf8.RuneCountInString(e.Name) > 80:
		return "le nom doit faire de 1 à 80 caractères"
	case e.Category == "" || utf8.RuneCountInString(e.Category) > 40:
		return "la catégorie doit faire de 1 à 40 caractères"
	case e.Icon != "" && !iconRef.MatchString(e.Icon):
		return "nom d'icône invalide"
	}
	return ""
}

func registerServices(mux *http.ServeMux, store Store, editor ServiceEditor, authSvc *auth.Service, check URLChecker) {
	audit := func(r *http.Request, kind, detail string) {
		p, _ := current(r.Context())
		authSvc.Audit(r.Context(), auth.Event{Kind: kind, UserID: &p.user.ID, IP: clientOf(r).IP, Detail: detail})
		recordEdit(r, store, detail, kind, &p.user.ID)
	}
	// Answers with the service as it now is, or the error the write gave.
	finish := func(w http.ResponseWriter, r *http.Request, slug string, err error) {
		if err != nil {
			respond(w, r)(nil, err)
			return
		}
		respond(w, r)(store.Service(r.Context(), slug))
	}

	mux.HandleFunc("PUT /api/v1/services/{slug}/favorite", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Favorite bool }
		if !decodeJSON(w, r, &body) {
			return
		}
		slug := r.PathValue("slug")
		finish(w, r, slug, editor.SetFavorite(r.Context(), slug, body.Favorite))
	})

	mux.HandleFunc("PATCH /api/v1/services/{slug}", func(w http.ResponseWriter, r *http.Request) {
		var edit ServiceEdit
		if !decodeJSON(w, r, &edit) {
			return
		}
		if msg := edit.validate(); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		slug := r.PathValue("slug")
		err := editor.UpdateService(r.Context(), slug, edit)
		if errors.Is(err, ErrInvalidLink) {
			writeError(w, http.StatusBadRequest, "ce projet ne peut pas être relié")
			return
		}
		if err == nil {
			audit(r, "service_updated", slug)
		}
		finish(w, r, slug, err)
	})

	mux.HandleFunc("POST /api/v1/services/{slug}/reset", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		err := editor.ResetService(r.Context(), slug)
		if err == nil {
			audit(r, "service_reset", slug)
		}
		finish(w, r, slug, err)
	})

	mux.HandleFunc("POST /api/v1/services/{slug}/url", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL string
			// Force saves an address that does not answer from the server.
			Force bool
			// AcceptCert is the fingerprint of the certificate the user accepts.
			AcceptCert string
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		address, err := urlcheck.Validate(body.URL)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.AcceptCert != "" && !certFingerprint.MatchString(body.AcceptCert) {
			writeError(w, http.StatusBadRequest, "empreinte de certificat invalide")
			return
		}
		slug := r.PathValue("slug")
		if _, err := store.Service(r.Context(), slug); err != nil {
			respond(w, r)(nil, err)
			return
		}
		res := check(r.Context(), address, body.AcceptCert)
		out := URLCheckResponse{Result: string(res.Kind), Status: res.Status, LatencyMs: res.LatencyMs, CertSHA256: res.CertSHA256, CertName: res.CertName}
		// A certificate is never saved by force: only accepting that one passes.
		if res.Answers() || (body.Force && res.Kind != urlcheck.KindCert) {
			pinned := ""
			if res.Answers() {
				pinned = body.AcceptCert
			}
			if err := editor.SaveServiceURL(r.Context(), slug, address, pinned, res); err != nil {
				respond(w, r)(nil, err)
				return
			}
			out.Saved = true
			audit(r, "service_url_set", slug)
		}
		writeJSON(w, http.StatusOK, out)
	})

	mux.HandleFunc("DELETE /api/v1/services/{slug}/url", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		err := editor.ClearServiceURL(r.Context(), slug)
		if err == nil {
			audit(r, "service_url_cleared", slug)
		}
		finish(w, r, slug, err)
	})

	if splitter, ok := editor.(ServiceSplitter); ok {
		mux.HandleFunc("POST /api/v1/services/{slug}/split", func(w http.ResponseWriter, r *http.Request) {
			var body ServiceSplit
			if !decodeJSON(w, r, &body) {
				return
			}
			if msg := body.validate(); msg != "" {
				writeError(w, http.StatusBadRequest, msg)
				return
			}
			slug := r.PathValue("slug")
			created, err := splitter.SplitService(r.Context(), slug, body)
			if errors.Is(err, ErrInvalidSplit) {
				writeError(w, http.StatusBadRequest, "ce conteneur ne peut pas devenir un service à part")
				return
			}
			if err == nil {
				audit(r, "service_split", slug)
				audit(r, "service_split", created)
			}
			finish(w, r, created, err)
		})

		mux.HandleFunc("POST /api/v1/services/{slug}/join", func(w http.ResponseWriter, r *http.Request) {
			slug := r.PathValue("slug")
			parent, err := splitter.JoinService(r.Context(), slug)
			if errors.Is(err, ErrInvalidSplit) {
				writeError(w, http.StatusBadRequest, "ce service n'a pas été séparé de son projet")
				return
			}
			if err != nil {
				respond(w, r)(nil, err)
				return
			}
			audit(r, "service_joined", slug)
			if parent == "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			audit(r, "service_joined", parent)
			respond(w, r)(store.Service(r.Context(), parent))
		})
	}

	mux.HandleFunc("GET /api/v1/hidden", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(editor.HiddenGroups(r.Context()))
	})

	mux.HandleFunc("POST /api/v1/hidden/show", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ HostID, Key string }
		if !decodeJSON(w, r, &body) {
			return
		}
		slug, err := editor.ShowHidden(r.Context(), body.HostID, body.Key)
		if err == nil {
			audit(r, "service_shown", slug)
		}
		finish(w, r, slug, err)
	})
}
