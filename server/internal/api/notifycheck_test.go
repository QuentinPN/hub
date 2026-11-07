package api

import (
	"context"
	"testing"
	"time"

	"hub/server/internal/notify"
)

// statusOnly answers Status; the check reads nothing else.
type statusOnly struct{ st notify.Status }

func (s statusOnly) Status(context.Context) (notify.Status, error)         { return s.st, nil }
func (statusOnly) SaveNtfy(context.Context, string, string) error          { return nil }
func (statusOnly) SaveSMTP(context.Context, notify.SMTPInput) error        { return nil }
func (statusOnly) Remove(context.Context, string) error                    { return nil }
func (statusOnly) SaveAlerts(context.Context, map[string]bool, bool) error { return nil }
func (statusOnly) Test(context.Context, string) error                      { return nil }

func TestNotificationsCheck(t *testing.T) {
	at := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name  string
		st    notify.Status
		level string
	}{
		{"none", notify.Status{}, LevelOK},
		{"untested", notify.Status{Ntfy: notify.NtfyStatus{Configured: true}}, LevelOK},
		{"sent", notify.Status{Ntfy: notify.NtfyStatus{Configured: true, Last: &notify.Send{At: at, OK: true}}}, LevelOK},
		{"failed", notify.Status{Ntfy: notify.NtfyStatus{Configured: true, Last: &notify.Send{At: at, OK: true}},
			SMTP: notify.SMTPStatus{Configured: true, Last: &notify.Send{At: at, Error: "serveur injoignable"}}}, LevelBad},
	} {
		check := notifyCheck(context.Background(), statusOnly{c.st})
		if check.Level != c.level {
			t.Errorf("%s: %s (%s), want %s", c.name, check.Level, check.Detail, c.level)
		}
		if c.name == "failed" && check.Detail != "Dernier envoi en échec · e-mail : serveur injoignable" {
			t.Errorf("detail %q", check.Detail)
		}
	}
}
