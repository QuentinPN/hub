package api_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"hub/server/internal/api"
	"hub/server/internal/fixture"
	"hub/server/internal/notify"
)

// fakeNotifications keeps what the routes passed; the topic goes in, never back out.
type fakeNotifications struct {
	st     notify.Status
	topic  string
	smtp   notify.SMTPInput
	alerts map[string]bool
	quiet  bool
	tested []string
}

func (f *fakeNotifications) Status(context.Context) (notify.Status, error) { return f.st, nil }
func (f *fakeNotifications) SaveNtfy(_ context.Context, url, topic string) error {
	if url == "" {
		return fmt.Errorf("%w: adresse vide", notify.ErrInvalid)
	}
	f.topic = topic
	f.st.Ntfy = notify.NtfyStatus{Configured: true, URL: url, TopicHint: topic[:4]}
	return nil
}
func (f *fakeNotifications) SaveSMTP(_ context.Context, in notify.SMTPInput) error {
	f.smtp = in
	return nil
}
func (f *fakeNotifications) Remove(_ context.Context, ch string) error {
	f.st.Ntfy = notify.NtfyStatus{}
	return nil
}
func (f *fakeNotifications) SaveAlerts(_ context.Context, a map[string]bool, q bool) error {
	f.alerts, f.quiet = a, q
	return nil
}
func (f *fakeNotifications) Test(_ context.Context, ch string) error {
	if ch == notify.SMTP && !f.st.SMTP.Configured {
		return notify.ErrNotConfigured
	}
	f.tested = append(f.tested, ch)
	return errors.New("serveur injoignable")
}

func TestNotificationRoutes(t *testing.T) {
	n := &fakeNotifications{}
	e := newRuntimeEnv(t, fixture.New(), nil, nil, &api.Runtime{Notifications: n})

	status, body := e.do(t, e.user, http.MethodPut, "/api/v1/notifications/ntfy", `{"url":"https://ntfy.sh","topic":"hub-alertes-1234"}`, nil)
	if status != http.StatusOK || n.topic != "hub-alertes-1234" || strings.Contains(string(body), "alertes-1234") {
		t.Fatalf("save ntfy: %d %s", status, body)
	}
	if status, body = e.do(t, e.user, http.MethodPut, "/api/v1/notifications/ntfy", `{"url":"","topic":"x"}`, nil); status != http.StatusBadRequest {
		t.Fatalf("invalid: %d %s", status, body)
	}
	if status, _ = e.do(t, e.user, http.MethodPut, "/api/v1/notifications/smtp",
		`{"host":"smtp.exemple.fr","port":465,"username":"hub","password":"secret","from":"a@b.fr","to":"c@d.fr"}`, nil); status != http.StatusOK || n.smtp.Password != "secret" {
		t.Fatalf("save smtp: %d %+v", status, n.smtp)
	}
	if status, _ = e.do(t, e.user, http.MethodPut, "/api/v1/notifications/alerts", `{"alerts":{"disk":true},"quietHours":false}`, nil); status != http.StatusOK || !n.alerts["disk"] || n.quiet {
		t.Fatalf("alerts: %d %v %v", status, n.alerts, n.quiet)
	}
	// A failed send answers 200: the channel's state says how it went.
	if status, _ = e.do(t, e.user, http.MethodPost, "/api/v1/notifications/ntfy/test", "", nil); status != http.StatusOK || len(n.tested) != 1 {
		t.Fatalf("test: %d %v", status, n.tested)
	}
	if status, _ = e.do(t, e.user, http.MethodPost, "/api/v1/notifications/smtp/test", "", nil); status != http.StatusConflict {
		t.Fatalf("test unconfigured: %d", status)
	}
	if status, _ = e.do(t, e.user, http.MethodPost, "/api/v1/notifications/slack/test", "", nil); status != http.StatusNotFound {
		t.Fatalf("unknown channel: %d", status)
	}
	if status, _ = e.do(t, e.user, http.MethodDelete, "/api/v1/notifications/ntfy", "", nil); status != http.StatusOK || n.st.Ntfy.Configured {
		t.Fatalf("remove: %d", status)
	}
	if status, _ = e.do(t, e.anon, http.MethodGet, "/api/v1/notifications", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", status)
	}

	var kinds []string
	for _, ev := range e.repo.Events {
		if ev.Kind == "settings_updated" {
			kinds = append(kinds, ev.Detail)
		}
	}
	if strings.Join(kinds, ",") != "notify_ntfy,notify_smtp,notify_alerts,notify_remove_ntfy" {
		t.Fatalf("audit %v", kinds)
	}
}
