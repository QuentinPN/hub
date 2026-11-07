package api

import (
	"context"
	"errors"
	"net/http"

	"hub/server/internal/auth"
	"hub/server/internal/notify"
)

// Notifications runs the alerts and their settings; *notify.Service is the real one.
type Notifications interface {
	Status(ctx context.Context) (notify.Status, error)
	SaveNtfy(ctx context.Context, url, topic string) error
	SaveSMTP(ctx context.Context, in notify.SMTPInput) error
	Remove(ctx context.Context, channel string) error
	SaveAlerts(ctx context.Context, alerts map[string]bool, quietHours bool) error
	Test(ctx context.Context, channel string) error
}

// registerNotifications serves « Notifications ». The topic and the password come in, are
// sealed, and never come back out.
func registerNotifications(mux *http.ServeMux, n Notifications, authSvc *auth.Service) {
	audit := func(r *http.Request, detail string) {
		p, _ := current(r.Context())
		authSvc.Audit(r.Context(), auth.Event{Kind: "settings_updated", UserID: &p.user.ID, IP: clientOf(r).IP, Detail: detail})
	}
	// saved answers a change: 400 for what the form can fix, the new state otherwise.
	saved := func(w http.ResponseWriter, r *http.Request, err error, detail string) {
		switch {
		case errors.Is(err, notify.ErrInvalid):
			writeError(w, http.StatusBadRequest, err.Error())
			return
		case err != nil:
			internalError(w, r, err)
			return
		}
		audit(r, detail)
		respond(w, r)(n.Status(r.Context()))
	}
	channel := func(w http.ResponseWriter, r *http.Request) (string, bool) {
		ch := r.PathValue("channel")
		if ch != notify.Ntfy && ch != notify.SMTP {
			writeError(w, http.StatusNotFound, "not found")
			return "", false
		}
		return ch, true
	}

	mux.HandleFunc("GET /api/v1/notifications", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(n.Status(r.Context()))
	})
	mux.HandleFunc("PUT /api/v1/notifications/ntfy", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL   string `json:"url"`
			Topic string `json:"topic"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		saved(w, r, n.SaveNtfy(r.Context(), body.URL, body.Topic), "notify_ntfy")
	})
	mux.HandleFunc("PUT /api/v1/notifications/smtp", func(w http.ResponseWriter, r *http.Request) {
		var body notify.SMTPInput
		if !decodeJSON(w, r, &body) {
			return
		}
		saved(w, r, n.SaveSMTP(r.Context(), body), "notify_smtp")
	})
	mux.HandleFunc("DELETE /api/v1/notifications/{channel}", func(w http.ResponseWriter, r *http.Request) {
		ch, ok := channel(w, r)
		if !ok {
			return
		}
		saved(w, r, n.Remove(r.Context(), ch), "notify_remove_"+ch)
	})
	mux.HandleFunc("PUT /api/v1/notifications/alerts", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Alerts     map[string]bool `json:"alerts"`
			QuietHours bool            `json:"quietHours"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		saved(w, r, n.SaveAlerts(r.Context(), body.Alerts, body.QuietHours), "notify_alerts")
	})
	// A failed send is not an error of the request: the state says how it went.
	mux.HandleFunc("POST /api/v1/notifications/{channel}/test", func(w http.ResponseWriter, r *http.Request) {
		ch, ok := channel(w, r)
		if !ok {
			return
		}
		if err := n.Test(r.Context(), ch); errors.Is(err, notify.ErrNotConfigured) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		respond(w, r)(n.Status(r.Context()))
	})
}

// notifyCheck is the « Notifications » line of « État de Hub »: a channel whose last
// message did not leave.
func notifyCheck(ctx context.Context, n Notifications) StatusCheck {
	c := StatusCheck{ID: "notify", Name: "Notifications", Level: LevelOK, Link: "notifications"}
	st, err := n.Status(ctx)
	if err != nil {
		c.Level, c.Detail = LevelBad, "État illisible"
		return c
	}
	var names, failing []string
	for _, ch := range []struct {
		name       string
		configured bool
		last       *notify.Send
	}{{"ntfy", st.Ntfy.Configured, st.Ntfy.Last}, {"e-mail", st.SMTP.Configured, st.SMTP.Last}} {
		if !ch.configured {
			continue
		}
		names = append(names, ch.name)
		if ch.last == nil {
			continue
		}
		if !ch.last.OK {
			failing = append(failing, ch.name+" : "+ch.last.Error)
			c.Since = &ch.last.At
		} else if c.Level == LevelOK && (c.Since == nil || ch.last.At.After(*c.Since)) {
			c.Since = &ch.last.At
		}
		if !ch.last.OK {
			c.Level = LevelBad
		}
	}
	switch {
	case len(names) == 0:
		c.Detail, c.Since = "Aucun canal : Hub ne prévient personne", nil
	case len(failing) > 0:
		c.Detail = "Dernier envoi en échec · " + joinWords(failing)
	default:
		c.Detail = joinWords(names) + " · derniers envois partis"
	}
	return c
}

func joinWords(list []string) string {
	out := ""
	for i, s := range list {
		switch {
		case i == 0:
		case i == len(list)-1:
			out += " et "
		default:
			out += ", "
		}
		out += s
	}
	return out
}
