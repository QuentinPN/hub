package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"

	"hub/server/internal/auth"
	"hub/server/internal/secrets"
	"hub/server/internal/widgets"
)

// WidgetEditor is implemented by stores that keep widget connections.
type WidgetEditor interface {
	WidgetRows(ctx context.Context) ([]WidgetRow, error)
	// WidgetTarget returns ErrNotFound for an unknown service.
	WidgetTarget(ctx context.Context, slug string) (WidgetTarget, error)
	SaveWidgetConnection(ctx context.Context, slug string, c NewConnection) error
	DeleteWidgetConnection(ctx context.Context, slug string) error
}

// WidgetTarget says what can be connected on a service.
type WidgetTarget struct {
	Support widgets.Support
	// Kind is the connector's name, when Support is ready.
	Kind string
	// URL is the service's address, "" without one.
	URL    string
	Pinned string
	// ConnectedURL is the address the stored key was tested on, "" without a connection.
	ConnectedURL string
	KeyHint      string
}

// NewConnection is what a successful save writes.
type NewConnection struct {
	Kind           string
	URL            string
	KeyHint        string
	Options        map[string]string
	Enabled        []string
	RefreshSeconds int
	State          widgets.State
	Data           map[string]json.RawMessage
	At             time.Time
}

// WidgetKeys keeps the connections' keys, sealed.
type WidgetKeys interface {
	Get(ctx context.Context, name string) (secrets.Value, error)
	Put(ctx context.Context, name string, v secrets.Value) error
	Delete(ctx context.Context, name string) error
}

// A key goes in a header: nothing that could break out of one.
func validKey(key string) bool {
	return key != "" && len(key) <= 512 && strings.IndexFunc(key, unicode.IsControl) < 0
}

func (s *WidgetSave) validate(m widgets.Manifest) string {
	if !slices.Contains(widgets.RefreshChoices, s.RefreshSeconds) {
		return "fréquence de rafraîchissement inconnue"
	}
	if len(s.Widgets) == 0 {
		return "choisis au moins un widget"
	}
	for _, id := range s.Widgets {
		if !slices.ContainsFunc(m.Widgets, func(w widgets.Widget) bool { return w.ID == id }) {
			return "widget inconnu"
		}
	}
	return ""
}

// allowedOptions keeps the values the service itself listed.
func allowedOptions(asked map[string]string, offered []widgets.Option) map[string]string {
	kept := map[string]string{}
	for _, o := range offered {
		for _, c := range o.Choices {
			if asked[o.ID] == c.Value {
				kept[o.ID] = c.Value
			}
		}
	}
	return kept
}

func registerWidgets(mux *http.ServeMux, store Store, editor WidgetEditor, keys WidgetKeys, authSvc *auth.Service) {
	audit := func(r *http.Request, kind, detail string) {
		p, _ := current(r.Context())
		authSvc.Audit(r.Context(), auth.Event{Kind: kind, UserID: &p.user.ID, IP: clientOf(r).IP, Detail: detail})
	}
	// target checks the service can be connected, and writes the refusal otherwise.
	target := func(w http.ResponseWriter, r *http.Request) (WidgetTarget, widgets.Manifest, bool) {
		t, err := editor.WidgetTarget(r.Context(), r.PathValue("slug"))
		if err != nil {
			respond(w, r)(nil, err)
			return t, widgets.Manifest{}, false
		}
		m, ok := widgets.ManifestOf(t.Kind)
		switch {
		case !ok:
			writeError(w, http.StatusBadRequest, "aucun widget disponible pour ce service")
		case t.URL == "":
			writeError(w, http.StatusBadRequest, "donne d'abord l'adresse du service")
		default:
			return t, m, true
		}
		return t, m, false
	}
	// key picks the key to use. The stored one only serves on the address it was tested on.
	key := func(w http.ResponseWriter, r *http.Request, t WidgetTarget, typed string) (secrets.Value, bool) {
		typed = strings.TrimSpace(typed)
		if typed != "" {
			if !validKey(typed) {
				writeError(w, http.StatusBadRequest, "clé illisible")
				return secrets.Value{}, false
			}
			return secrets.NewValue(typed), true
		}
		if t.ConnectedURL == "" || t.ConnectedURL != t.URL {
			writeError(w, http.StatusBadRequest, "saisis la clé d'API")
			return secrets.Value{}, false
		}
		stored, err := keys.Get(r.Context(), widgets.KeyName(r.PathValue("slug")))
		if errors.Is(err, secrets.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "saisis la clé d'API")
			return secrets.Value{}, false
		}
		if err != nil {
			internalError(w, r, err)
			return secrets.Value{}, false
		}
		return stored, true
	}

	mux.HandleFunc("GET /api/v1/widgets", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(editor.WidgetRows(r.Context()))
	})

	mux.HandleFunc("POST /api/v1/services/{slug}/connection/test", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ APIKey string }
		if !decodeJSON(w, r, &body) {
			return
		}
		t, _, ok := target(w, r)
		if !ok {
			return
		}
		value, ok := key(w, r, t, body.APIKey)
		if !ok {
			return
		}
		start := time.Now()
		probe, state := widgets.TestKey(r.Context(), widgets.Target{Kind: t.Kind, URL: t.URL, Pinned: t.Pinned}, value)
		out := WidgetTest{Result: state, LatencyMs: int(time.Since(start).Milliseconds())}
		if state == widgets.StateOK {
			out.Note, out.Options = probe.Note, probe.Options
		}
		writeJSON(w, http.StatusOK, out)
	})

	mux.HandleFunc("PUT /api/v1/services/{slug}/connection", func(w http.ResponseWriter, r *http.Request) {
		var body WidgetSave
		if !decodeJSON(w, r, &body) {
			return
		}
		t, manifest, ok := target(w, r)
		if !ok {
			return
		}
		if msg := body.validate(manifest); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		value, ok := key(w, r, t, body.APIKey)
		if !ok {
			return
		}
		// A new key is only kept once it has passed a test, here on the server.
		where := widgets.Target{Kind: t.Kind, URL: t.URL, Pinned: t.Pinned}
		probe, state := widgets.TestKey(r.Context(), where, value)
		if state != widgets.StateOK {
			writeJSON(w, http.StatusOK, WidgetSaved{Result: state})
			return
		}
		options := allowedOptions(body.Options, probe.Options)
		data, state := widgets.Read(r.Context(), where, value, options, body.Widgets)
		slug := r.PathValue("slug")
		hint := t.KeyHint
		if strings.TrimSpace(body.APIKey) != "" {
			if err := keys.Put(r.Context(), widgets.KeyName(slug), value); err != nil {
				internalError(w, r, err)
				return
			}
			hint = widgets.KeyHint(value.Reveal())
		}
		err := editor.SaveWidgetConnection(r.Context(), slug, NewConnection{
			Kind: t.Kind, URL: t.URL, KeyHint: hint, Options: options, Enabled: body.Widgets,
			RefreshSeconds: body.RefreshSeconds, State: state, Data: data, At: time.Now(),
		})
		if err != nil {
			respond(w, r)(nil, err)
			return
		}
		audit(r, "widget_connected", slug)
		svc, err := store.Service(r.Context(), slug)
		if err != nil {
			respond(w, r)(nil, err)
			return
		}
		writeJSON(w, http.StatusOK, WidgetSaved{Saved: true, Result: state, Service: &svc})
	})

	// Removing a connection erases its key, even if the row is already gone.
	mux.HandleFunc("DELETE /api/v1/services/{slug}/connection", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		if err := editor.DeleteWidgetConnection(r.Context(), slug); err != nil {
			respond(w, r)(nil, err)
			return
		}
		if err := keys.Delete(r.Context(), widgets.KeyName(slug)); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			internalError(w, r, err)
			return
		}
		audit(r, "widget_disconnected", slug)
		respond(w, r)(store.Service(r.Context(), slug))
	})
}
