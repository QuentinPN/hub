package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
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
	// CanAct is the key's write permission as the last test found it; nil when unknown.
	CanAct *bool
	Data   map[string]json.RawMessage
	At     time.Time
}

// WidgetKeys keeps the connections' keys, sealed.
type WidgetKeys interface {
	Get(ctx context.Context, name string) (secrets.Value, error)
	Put(ctx context.Context, name string, v secrets.Value) error
	Delete(ctx context.Context, name string) error
}

// Widgets is what the widget routes need besides the store.
type Widgets struct {
	Keys WidgetKeys
	Bus  *widgets.Bus
	// Wake asks for a connection's read now: after a write, or on its webhook (widgets.Wake).
	Wake func(slug string)
	// Origin is HUB_ORIGIN, the dashboard's public address; "" to take the request's.
	Origin string
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

func registerWidgets(mux *http.ServeMux, store Store, editor WidgetEditor, live *Widgets, authSvc *auth.Service) {
	keys := live.Keys
	audit := func(r *http.Request, kind, detail string) {
		auditEvent(r, authSvc, kind, detail)
		p, _ := current(r.Context())
		recordEdit(r, store, detail, kind, &p.user.ID)
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
	key := func(w http.ResponseWriter, r *http.Request, t WidgetTarget, m widgets.Manifest, typed, identity string) (secrets.Value, bool) {
		identity = strings.TrimSpace(identity)
		missing := "saisis la clé d'API"
		if m.Auth == widgets.AuthPassword {
			missing = "saisis l'identifiant et le mot de passe"
		}
		if m.Auth != widgets.AuthPassword {
			typed = strings.TrimSpace(typed)
		}
		if typed != "" {
			if !validKey(typed) || !validKey(identity) && m.Auth == widgets.AuthPassword {
				writeError(w, http.StatusBadRequest, "saisis l'identifiant et le mot de passe")
				return secrets.Value{}, false
			}
			if m.Auth == widgets.AuthPassword {
				typed = widgets.JoinCredentials(identity, typed)
			}
			return secrets.NewValue(typed), true
		}
		if t.ConnectedURL == "" || t.ConnectedURL != t.URL || identity != "" {
			writeError(w, http.StatusBadRequest, missing)
			return secrets.Value{}, false
		}
		stored, err := keys.Get(r.Context(), widgets.KeyName(r.PathValue("slug")))
		if errors.Is(err, secrets.ErrNotFound) {
			writeError(w, http.StatusBadRequest, missing)
			return secrets.Value{}, false
		}
		if err != nil {
			internalError(w, r, err)
			return secrets.Value{}, false
		}
		return stored, true
	}
	// offered lists what the panel can set: the connector's options, plus which system each
	// Hub host is for a connector that asks, the system named alike first.
	offered := func(ctx context.Context, m widgets.Manifest, probe widgets.Probe) ([]widgets.Option, error) {
		options := probe.Options
		hosts, ok := store.(HostReader)
		if !m.MapHosts || !ok {
			return options, nil
		}
		list, err := hosts.AgentHosts(ctx, time.Now())
		if err != nil {
			return nil, err
		}
		for _, h := range list {
			if h.LastSeenAt == nil {
				continue
			}
			choices := []widgets.Choice{}
			match := widgets.MatchSystem(probe.Systems, h.ID, h.Name)
			if match != "" {
				choices = append(choices, widgets.Choice{Value: match, Label: match})
			}
			choices = append(choices, widgets.Choice{Value: "", Label: "Aucun"})
			for _, name := range probe.Systems {
				if name != match {
					choices = append(choices, widgets.Choice{Value: name, Label: name})
				}
			}
			options = append(options, widgets.Option{ID: widgets.HostOptionPrefix + h.ID, Label: "Système de " + h.Name, Choices: choices})
		}
		return options, nil
	}

	mux.HandleFunc("GET /api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		streamEvents(w, r, live.Bus)
	})

	// connected is what a stored connection may use: its key, on the address it was tested on.
	connected := func(r *http.Request) (widgets.Target, secrets.Value, bool) {
		slug := r.PathValue("slug")
		t, err := editor.WidgetTarget(r.Context(), slug)
		if err != nil || t.ConnectedURL == "" || t.ConnectedURL != t.URL {
			return widgets.Target{}, secrets.Value{}, false
		}
		key, err := keys.Get(r.Context(), widgets.KeyName(slug))
		if err != nil {
			return widgets.Target{}, secrets.Value{}, false
		}
		return widgets.Target{Kind: t.Kind, URL: t.URL, Pinned: t.Pinned}, key, true
	}

	// The image goes through the server: only it holds the key. Nothing here reveals why a
	// poster is missing: the page falls back on its flat color.
	mux.HandleFunc("GET /api/v1/posters/{slug}/{ref}", func(w http.ResponseWriter, r *http.Request) {
		where, key, ok := connected(r)
		if !ok {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		poster, ok := widgets.FetchPoster(r.Context(), where, key, r.PathValue("ref"))
		if !ok {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h := w.Header()
		h.Set("Content-Type", poster.ContentType)
		h.Set("Cache-Control", "private, max-age=86400")
		_, _ = w.Write(poster.Body)
	})

	registerActions(mux, live, authSvc, connected)

	mux.HandleFunc("GET /api/v1/widgets", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(editor.WidgetRows(r.Context()))
	})

	mux.HandleFunc("POST /api/v1/services/{slug}/connection/test", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ APIKey, Identity string }
		if !decodeJSON(w, r, &body) {
			return
		}
		t, manifest, ok := target(w, r)
		if !ok {
			return
		}
		value, ok := key(w, r, t, manifest, body.APIKey, body.Identity)
		if !ok {
			return
		}
		start := time.Now()
		probe, state := widgets.TestKey(r.Context(), widgets.Target{Kind: t.Kind, URL: t.URL, Pinned: t.Pinned}, value)
		out := WidgetTest{Result: state, LatencyMs: int(time.Since(start).Milliseconds())}
		if state == widgets.StateOK {
			options, err := offered(r.Context(), manifest, probe)
			if err != nil {
				internalError(w, r, err)
				return
			}
			out.Note, out.Options = probe.Note, options
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
		value, ok := key(w, r, t, manifest, body.APIKey, body.Identity)
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
		choices, err := offered(r.Context(), manifest, probe)
		if err != nil {
			internalError(w, r, err)
			return
		}
		options := allowedOptions(body.Options, choices)
		data, state := widgets.Read(r.Context(), where, value, options, body.Widgets)
		slug := r.PathValue("slug")
		hint := t.KeyHint
		if body.APIKey != "" && strings.TrimSpace(body.APIKey+body.Identity) != "" {
			if err := keys.Put(r.Context(), widgets.KeyName(slug), value); err != nil {
				internalError(w, r, err)
				return
			}
			hint = widgets.KeyHint(value.Reveal())
			// An account is not a secret to its owner, and tells which one this is.
			if manifest.Auth == widgets.AuthPassword {
				hint = strings.TrimSpace(body.Identity)
			}
		}
		err = editor.SaveWidgetConnection(r.Context(), slug, NewConnection{
			Kind: t.Kind, URL: t.URL, KeyHint: hint, Options: options, Enabled: body.Widgets,
			RefreshSeconds: body.RefreshSeconds, State: state, CanAct: probe.CanAct, Data: data, At: time.Now(),
		})
		if err != nil {
			respond(w, r)(nil, err)
			return
		}
		audit(r, "widget_connected", slug)
		live.Bus.Publish(slug)
		svc, err := store.Service(r.Context(), slug)
		if err != nil {
			respond(w, r)(nil, err)
			return
		}
		writeJSON(w, http.StatusOK, WidgetSaved{Saved: true, Result: state, Service: &svc})
	})

	mux.HandleFunc("GET /api/v1/services/{slug}/connection", func(w http.ResponseWriter, r *http.Request) {
		svc, err := store.Service(r.Context(), r.PathValue("slug"))
		if err != nil {
			respond(w, r)(nil, err)
			return
		}
		if svc.Widget == nil || svc.Widget.Connection == nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, svc.Widget.Connection)
	})

	// A new secret replaces the last one at once: it is shown here, once, and never again.
	mux.HandleFunc("POST /api/v1/services/{slug}/connection/webhook-secret", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		t, err := editor.WidgetTarget(r.Context(), slug)
		if err != nil {
			respond(w, r)(nil, err)
			return
		}
		if !hasWebhook(t.Kind) || t.ConnectedURL == "" {
			writeError(w, http.StatusBadRequest, "ce service n'a pas de webhook à relier")
			return
		}
		raw := make([]byte, webhookSecretBytes)
		if _, err := rand.Read(raw); err != nil {
			internalError(w, r, err)
			return
		}
		secret := base64.RawURLEncoding.EncodeToString(raw)
		if err := keys.Put(r.Context(), widgets.WebhookName(slug), secrets.NewValue(secret)); err != nil {
			internalError(w, r, err)
			return
		}
		auditAction(r, authSvc, "widget_webhook_rotated")
		live.Bus.Publish(slug)
		writeJSON(w, http.StatusOK, WebhookSecret{URL: originOf(r, live.Origin) + hookPrefix + slug, Secret: secret})
	})

	// The service's signal that something changed: read it now. Public, behind its own secret,
	// and the same refusal whatever is wrong, so that it tells nothing.
	mux.HandleFunc("POST "+hookPrefix+"{slug}", func(w http.ResponseWriter, r *http.Request) {
		ip := clientOf(r).IP
		if err := authSvc.HookBlocked(ip); err != nil {
			passkeyError(w, r, err)
			return
		}
		refuse := func() {
			authSvc.HookRefused(ip)
			writeError(w, http.StatusUnauthorized, "unauthorized")
		}
		slug := r.PathValue("slug")
		secret, err := keys.Get(r.Context(), widgets.WebhookName(slug))
		if err != nil && !errors.Is(err, secrets.ErrNotFound) {
			slog.Error("webhook", "service", slug, "err", err)
		}
		given := r.Header.Get("Authorization")
		if err != nil || given == "" || subtle.ConstantTimeCompare([]byte(given), []byte(secret.Reveal())) != 1 {
			refuse()
			return
		}
		// Only now, signed: the connection must still be one that has a webhook.
		if t, err := editor.WidgetTarget(r.Context(), slug); err != nil || !hasWebhook(t.Kind) {
			refuse()
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxHookBody))
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			writeError(w, http.StatusRequestEntityTooLarge, "body too large")
			return
		case err != nil || !json.Valid(body):
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		live.Wake(slug)
		w.WriteHeader(http.StatusNoContent)
	})

	// Removing a connection erases its key, even if the row is already gone.
	mux.HandleFunc("DELETE /api/v1/services/{slug}/connection", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		if err := editor.DeleteWidgetConnection(r.Context(), slug); err != nil {
			respond(w, r)(nil, err)
			return
		}
		for _, name := range []string{widgets.KeyName(slug), widgets.WebhookName(slug)} {
			if err := keys.Delete(r.Context(), name); err != nil && !errors.Is(err, secrets.ErrNotFound) {
				internalError(w, r, err)
				return
			}
		}
		audit(r, "widget_disconnected", slug)
		live.Bus.Publish(slug)
		respond(w, r)(store.Service(r.Context(), slug))
	})
}

const (
	eventsPing = 25 * time.Second
	// eventsMaxAge ends a stream so the browser reconnects: the session is checked at each
	// request only, and a stream must not outlive a closed session for long.
	eventsMaxAge       = 5 * time.Minute
	eventsWriteTimeout = 10 * time.Second
)

// streamEvents sends the names of the services that have news, as Server-Sent Events. The
// page then reads them through the API: nothing else travels on the stream.
func streamEvents(w http.ResponseWriter, r *http.Request, bus *widgets.Bus) {
	rc := http.NewResponseController(w)
	write := func(text string) bool {
		if err := rc.SetWriteDeadline(time.Now().Add(eventsWriteTimeout)); err != nil {
			return false
		}
		if _, err := io.WriteString(w, text); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	signals, stop := bus.Subscribe()
	defer stop()
	if !write(": ok\n\n") {
		return
	}
	ping := time.NewTicker(eventsPing)
	defer ping.Stop()
	maxAge := time.NewTimer(eventsMaxAge)
	defer maxAge.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-maxAge.C:
			return
		case <-ping.C:
			if !write(": ping\n\n") {
				return
			}
		case slug, ok := <-signals:
			if !ok || !write("event: widgets\ndata: "+slug+"\n\n") {
				return
			}
		}
	}
}
