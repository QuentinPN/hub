package api

import (
	"context"
	"testing"
	"time"

	"hub/server/internal/backup"
)

type fakeBackups struct{ st backup.Status }

func (f fakeBackups) Status(context.Context) (backup.Status, error)          { return f.st, nil }
func (fakeBackups) SaveSchedule(context.Context, bool, string, string) error { return nil }
func (fakeBackups) SetPassphrase(context.Context, string) error              { return nil }
func (fakeBackups) Start(context.Context) error                              { return nil }

func TestBackupCheck(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	done := func(hoursAgo int, ok bool) backup.Run {
		at := now.Add(-time.Duration(hoursAgo) * time.Hour)
		end := at.Add(time.Minute)
		return backup.Run{StartedAt: at, FinishedAt: &end, OK: ok, Error: map[bool]string{false: "QNAP injoignable"}[ok]}
	}
	ready := backup.Status{Enabled: true, Frequency: backup.Daily, Configured: true, Kept: 3}
	with := func(change func(*backup.Status)) backup.Status { s := ready; change(&s); return s }
	for _, c := range []struct {
		name  string
		st    backup.Status
		level string
	}{
		{"no passphrase", backup.Status{Enabled: true}, LevelWarn},
		{"no folder", with(func(s *backup.Status) { s.DirError = "Dossier absent" }), LevelBad},
		{"never ran", ready, LevelWarn},
		{"recent", with(func(s *backup.Status) { s.Runs = []backup.Run{done(9, true)} }), LevelOK},
		{"last failed", with(func(s *backup.Status) { s.Runs = []backup.Run{done(9, false), done(33, true)} }), LevelBad},
		{"too old", with(func(s *backup.Status) { s.Runs = []backup.Run{done(60, true)} }), LevelBad},
		{"weekly, six days", with(func(s *backup.Status) { s.Frequency = backup.Weekly; s.Runs = []backup.Run{done(150, true)} }), LevelOK},
		{"turned off", with(func(s *backup.Status) { s.Enabled = false; s.Runs = []backup.Run{done(9, true)} }), LevelWarn},
		// The one under way is not a failure.
		{"running", with(func(s *backup.Status) {
			s.Running = true
			s.Runs = []backup.Run{{StartedAt: now}, done(9, true)}
		}), LevelOK},
	} {
		if got := backupCheck(context.Background(), fakeBackups{c.st}, now); got.Level != c.level {
			t.Errorf("%s: %s (%s), want %s", c.name, got.Level, got.Detail, c.level)
		}
	}
}
