// Package notify sends Hub's alerts through ntfy and e-mail. An engine watches what Hub
// already knows (agents, services, widgets, backups, Beszel, sessions) and speaks only when
// a state changes, once its delay has passed, and again when it comes back.
package notify

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"hub/server/internal/backup"
	"hub/server/internal/secrets"
)

// Channels.
const (
	Ntfy = "ntfy"
	SMTP = "smtp"
)

// Alert kinds, as stored and sent to the browser.
const (
	AlertService = "service"
	AlertHost    = "host"
	AlertWidget  = "widget"
	AlertBackup  = "backup"
	AlertDisk    = "disk"
	AlertLogin   = "login"
)

// Alert is one line of « Me prévenir quand ». Urgent ones pass through the quiet hours.
type Alert struct {
	ID      string
	Delay   time.Duration
	Default bool
	Urgent  bool
}

// Alerts are the six kinds, in the order shown.
var Alerts = []Alert{
	{ID: AlertService, Delay: 5 * time.Minute, Default: true},
	{ID: AlertHost, Delay: 3 * time.Minute, Default: true, Urgent: true},
	{ID: AlertWidget, Delay: 15 * time.Minute, Default: true},
	{ID: AlertBackup, Delay: 0, Default: true},
	{ID: AlertDisk, Delay: 30 * time.Minute, Default: false},
	{ID: AlertLogin, Delay: 0, Default: false},
}

func alertOf(id string) (Alert, bool) {
	for _, a := range Alerts {
		if a.ID == id {
			return a, true
		}
	}
	return Alert{}, false
}

// Quiet hours, in the server's time zone (TZ): alerts wait for a summary in the morning.
const (
	quietFrom = 23 * 60
	quietTo   = 8 * 60
)

func inQuietHours(t time.Time) bool {
	m := t.Hour()*60 + t.Minute()
	return m >= quietFrom || m < quietTo
}

// Secret names in the secrets store.
const (
	secretTopic    = "notify:ntfy-topic"
	secretPassword = "notify:smtp-password"
)

// Settings are what the store keeps, secrets aside.
type Settings struct {
	NtfyURL      string
	NtfyHint     string
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPFrom     string
	SMTPTo       string
	// Alerts holds the kinds turned away from their default.
	Alerts     map[string]bool
	QuietHours bool
	// LastSession is the newest session already looked at; nil until the devices are learnt.
	LastSession *int64
}

func (s Settings) enabled(id string) bool {
	if on, ok := s.Alerts[id]; ok {
		return on
	}
	a, _ := alertOf(id)
	return a.Default
}

// Send is how a channel's last message went.
type Send struct {
	At    time.Time `json:"at"`
	OK    bool      `json:"ok"`
	Test  bool      `json:"test"`
	Error string    `json:"error"`
}

// Incident is a condition under way. NotifiedAt is set once its alert has left.
type Incident struct {
	Key        string
	Alert      string
	Subject    string
	Detail     string
	Since      time.Time
	NotifiedAt *time.Time
}

// Condition is something failing now. Subject and Detail say what, without any address.
type Condition struct {
	Key     string
	Alert   string
	Subject string
	Detail  string
}

// Snapshot is what Hub sees now: what fails, and what it cannot tell. A held key, or one
// starting with a held prefix ending in ":", keeps its incident as it is.
type Snapshot struct {
	Failing []Condition
	Held    []string
}

func (s Snapshot) held(key string) bool {
	for _, h := range s.Held {
		if h == key || strings.HasSuffix(h, ":") && strings.HasPrefix(key, h) {
			return true
		}
	}
	return false
}

// SessionSeen is a session opened, for « nouvel appareil ».
type SessionSeen struct {
	ID        int64
	UserAgent string
	CreatedAt time.Time
}

// DigestLine is a message kept during the quiet hours.
type DigestLine struct {
	ID    int64
	At    time.Time
	Title string
	Body  string
}

// Store keeps the settings and what the engine remembers across restarts.
type Store interface {
	NotifySettings(ctx context.Context) (Settings, error)
	// SaveNotifySettings writes everything but LastSession, which only the engine moves.
	SaveNotifySettings(ctx context.Context, s Settings) error
	SetNotifyLastSession(ctx context.Context, id int64) error
	ChannelStates(ctx context.Context) (map[string]Send, error)
	SaveChannelState(ctx context.Context, channel string, s Send) error
	DeleteChannelState(ctx context.Context, channel string) error
	Incidents(ctx context.Context) ([]Incident, error)
	PutIncident(ctx context.Context, i Incident) error
	DeleteIncident(ctx context.Context, key string) error
	AddDigest(ctx context.Context, l DigestLine) error
	Digest(ctx context.Context) ([]DigestLine, error)
	// ClearDigest removes the lines up to id, those just sent.
	ClearDigest(ctx context.Context, upTo int64) error
	// LearnDevice records a device label and says whether it was new.
	LearnDevice(ctx context.Context, label string, at time.Time) (bool, error)
	// SessionsAfter lists the sessions opened after id (all of them for nil), oldest first.
	SessionsAfter(ctx context.Context, id *int64) ([]SessionSeen, error)
	// NotifyConditions reads the agents, services, widgets and Beszel's disks.
	NotifyConditions(ctx context.Context, now time.Time) (Snapshot, error)
}

// Secrets seals the topic and the password; *secrets.Store is the real one.
type Secrets interface {
	Put(ctx context.Context, name string, v secrets.Value) error
	Get(ctx context.Context, name string) (secrets.Value, error)
	Delete(ctx context.Context, name string) error
}

// Backups reads the backup settings and history from the database: nil when backups do not
// run. Never the folder: a test file written each minute would keep the QNAP's disks awake.
type Backups interface {
	BackupSettings(ctx context.Context) (backup.Settings, error)
	BackupRuns(ctx context.Context, limit int) ([]backup.Run, error)
}

// AlertLog keeps the alerts sent about a service in its history; the store may offer it.
type AlertLog interface {
	LogAlert(ctx context.Context, slug string, a SentAlert) error
}

// SentAlert is an alert, or its recovery, that left Hub. Via names the channels that took
// it, or "digest" when it waits for the morning summary.
type SentAlert struct {
	Alert    string
	Title    string
	Recovery bool
	Via      []string
	At       time.Time
}

// Via of an alert kept for the morning summary.
const ViaDigest = "digest"

// Message is what leaves Hub: names and states only, never an address, a key or a torrent.
type Message struct {
	Title    string
	Body     string
	Recovery bool
	Test     bool
}

// Service runs the engine and the settings behind « Notifications ».
type Service struct {
	store   Store
	secrets Secrets
	backups Backups
	now     func() time.Time
	http    *http.Client
	// smtpTLS is the base TLS configuration for e-mail; tests give their own authority.
	smtpTLS *tls.Config

	// mu keeps one tick or one test at a time: a channel's state is written by one.
	mu sync.Mutex
}

// NewService returns the service; backups may be nil.
func NewService(store Store, sec Secrets, backups Backups) *Service {
	return &Service{store: store, secrets: sec, backups: backups, now: time.Now, http: newHTTPClient(), smtpTLS: &tls.Config{MinVersion: tls.VersionTLS12}}
}

// Errors the routes answer with 400.
var ErrInvalid = errors.New("invalid")

type invalidError struct{ msg string }

func (e *invalidError) Error() string { return e.msg }
func (e *invalidError) Unwrap() error { return ErrInvalid }

func invalid(format string, args ...any) error {
	return &invalidError{msg: fmt.Sprintf(format, args...)}
}

// Watch runs the engine until ctx ends.
func (s *Service) Watch(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			slog.Error("notifications", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick looks once at everything and sends what changed.
func (s *Service) Tick(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	set, err := s.store.NotifySettings(ctx)
	if err != nil {
		return err
	}
	snap, err := s.store.NotifyConditions(ctx, now)
	if err != nil {
		return err
	}
	s.addBackup(ctx, now, &snap)

	failing := map[string]Condition{}
	for _, c := range snap.Failing {
		if set.enabled(c.Alert) {
			failing[c.Key] = c
		}
	}
	incidents, err := s.store.Incidents(ctx)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, inc := range incidents {
		known[inc.Key] = true
		if !set.enabled(inc.Alert) {
			if err := s.store.DeleteIncident(ctx, inc.Key); err != nil {
				return err
			}
			continue
		}
		if snap.held(inc.Key) {
			continue
		}
		c, still := failing[inc.Key]
		if !still {
			if inc.NotifiedAt != nil {
				m := recoveryMessage(inc, now)
				via, ok := s.send(ctx, set, inc.Alert, m, now)
				if !ok {
					continue // retried at the next tick
				}
				s.logAlert(ctx, inc, m, via, now)
			}
			if err := s.store.DeleteIncident(ctx, inc.Key); err != nil {
				return err
			}
			continue
		}
		inc.Subject, inc.Detail = c.Subject, c.Detail
		if err := s.maybeAlert(ctx, set, inc, now); err != nil {
			return err
		}
	}
	for key, c := range failing {
		if known[key] {
			continue
		}
		inc := Incident{Key: key, Alert: c.Alert, Subject: c.Subject, Detail: c.Detail, Since: now}
		if err := s.store.PutIncident(ctx, inc); err != nil {
			return err
		}
		if err := s.maybeAlert(ctx, set, inc, now); err != nil {
			return err
		}
	}

	if err := s.watchSessions(ctx, set, now); err != nil {
		return err
	}
	return s.sendDigest(ctx, set, now)
}

func (s *Service) maybeAlert(ctx context.Context, set Settings, inc Incident, now time.Time) error {
	a, _ := alertOf(inc.Alert)
	if inc.NotifiedAt != nil || now.Sub(inc.Since) < a.Delay {
		return nil
	}
	m := alertMessage(inc)
	via, ok := s.send(ctx, set, inc.Alert, m, now)
	if !ok {
		return nil
	}
	s.logAlert(ctx, inc, m, via, now)
	inc.NotifiedAt = &now
	return s.store.PutIncident(ctx, inc)
}

// addBackup adds the backup condition: the last one failed, or none succeeded for two days
// while the automatic ones are on. A run unfinished for less than two hours is under way;
// older, Hub stopped in its middle, and it counts as failed.
func (s *Service) addBackup(ctx context.Context, now time.Time, snap *Snapshot) {
	if s.backups == nil {
		return
	}
	set, err := s.backups.BackupSettings(ctx)
	if err != nil {
		snap.Held = append(snap.Held, "backup")
		return
	}
	if set.Key == nil {
		return
	}
	runs, err := s.backups.BackupRuns(ctx, 10)
	if err != nil {
		snap.Held = append(snap.Held, "backup")
		return
	}
	var last, lastOK *backup.Run
	for i := range runs {
		r := &runs[i]
		if r.FinishedAt == nil && i == 0 && now.Sub(r.StartedAt) < 2*time.Hour {
			continue
		}
		if last == nil {
			last = r
		}
		if r.OK {
			lastOK = r
			break
		}
	}
	c := Condition{Key: "backup", Alert: AlertBackup, Subject: "Sauvegarde"}
	switch {
	case last != nil && !last.OK:
		c.Detail = "failed"
	case !set.Enabled:
		return
	case lastOK != nil && now.Sub(lastOK.StartedAt) > 48*time.Hour,
		lastOK == nil && set.KeySetAt != nil && now.Sub(*set.KeySetAt) > 48*time.Hour:
		c.Detail = "late"
	default:
		return
	}
	snap.Failing = append(snap.Failing, c)
}

// watchSessions tells of a session opened from a device never seen. The first time, every
// device already there is learnt in silence.
func (s *Service) watchSessions(ctx context.Context, set Settings, now time.Time) error {
	list, err := s.store.SessionsAfter(ctx, set.LastSession)
	if err != nil {
		return err
	}
	first := set.LastSession == nil
	last := int64(0)
	if set.LastSession != nil {
		last = *set.LastSession
	}
	for _, sess := range list {
		label := DeviceLabel(sess.UserAgent)
		isNew, err := s.store.LearnDevice(ctx, label, sess.CreatedAt)
		if err != nil {
			return err
		}
		if isNew && !first && set.enabled(AlertLogin) {
			// Once only: a lost one is not sent again, the session list keeps it.
			_, _ = s.send(ctx, set, AlertLogin, Message{Title: "Nouvelle connexion à Hub",
				Body: "Depuis " + label + ", un appareil jamais vu. Si ce n'est pas toi, ferme la session dans Réglages, Sécurité et accès."}, now)
		}
		last = max(last, sess.ID)
	}
	if first || len(list) > 0 {
		return s.store.SetNotifyLastSession(ctx, last)
	}
	return nil
}

// send delivers now, or keeps the message for the morning during the quiet hours. It says
// where the message went, and whether it is taken care of: false when every channel failed.
func (s *Service) send(ctx context.Context, set Settings, alert string, m Message, now time.Time) ([]string, bool) {
	a, _ := alertOf(alert)
	if set.QuietHours && inQuietHours(now) && !a.Urgent {
		if err := s.store.AddDigest(ctx, DigestLine{At: now, Title: m.Title, Body: m.Body}); err != nil {
			slog.Error("notifications digest", "err", err)
			return nil, false
		}
		return []string{ViaDigest}, true
	}
	return s.deliver(ctx, set, m, now)
}

// deliver sends to every configured channel and names those that took it. With none, there
// is nobody to tell: done.
func (s *Service) deliver(ctx context.Context, set Settings, m Message, now time.Time) ([]string, bool) {
	tried := 0
	var via []string
	for _, ch := range []string{Ntfy, SMTP} {
		if !configured(set, ch) {
			continue
		}
		tried++
		err := s.sendTo(ctx, set, ch, m)
		if err == nil {
			via = append(via, ch)
		} else {
			slog.Warn("notification not sent", "channel", ch, "err", err)
		}
		s.record(ctx, ch, now, m.Test, err)
	}
	return via, tried == 0 || len(via) > 0
}

// logAlert adds an alert about a service or its widget to that service's history, when it
// went somewhere and the store keeps one.
func (s *Service) logAlert(ctx context.Context, inc Incident, m Message, via []string, now time.Time) {
	h, ok := s.store.(AlertLog)
	if !ok || len(via) == 0 {
		return
	}
	_, slug, _ := strings.Cut(inc.Key, ":")
	if inc.Alert != AlertService && inc.Alert != AlertWidget {
		return
	}
	a := SentAlert{Alert: inc.Alert, Title: m.Title, Recovery: m.Recovery, Via: via, At: now}
	if err := h.LogAlert(context.WithoutCancel(ctx), slug, a); err != nil {
		slog.Error("notifications history", "err", err)
	}
}

func (s *Service) record(ctx context.Context, ch string, now time.Time, test bool, err error) {
	st := Send{At: now, OK: err == nil, Test: test}
	if err != nil {
		st.Error = err.Error()
	}
	if err := s.store.SaveChannelState(context.WithoutCancel(ctx), ch, st); err != nil {
		slog.Error("notifications", "err", err)
	}
}

func configured(set Settings, ch string) bool {
	if ch == Ntfy {
		return set.NtfyURL != ""
	}
	return set.SMTPHost != ""
}

func (s *Service) sendTo(ctx context.Context, set Settings, ch string, m Message) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if ch == Ntfy {
		topic, err := s.secrets.Get(ctx, secretTopic)
		if err != nil {
			return errors.New("sujet illisible : à saisir de nouveau")
		}
		return sendNtfy(ctx, s.http, set.NtfyURL, topic, m)
	}
	var password secrets.Value
	if set.SMTPUsername != "" {
		p, err := s.secrets.Get(ctx, secretPassword)
		if err != nil {
			return errors.New("mot de passe illisible : à saisir de nouveau")
		}
		password = p
	}
	return sendSMTP(ctx, s.smtpTLS, set, password, m, s.now())
}

// sendDigest sends the night's messages as one, once the quiet hours are over.
func (s *Service) sendDigest(ctx context.Context, set Settings, now time.Time) error {
	if set.QuietHours && inQuietHours(now) {
		return nil
	}
	lines, err := s.store.Digest(ctx)
	if err != nil || len(lines) == 0 {
		return err
	}
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "%s · %s", l.At.In(now.Location()).Format("15:04"), l.Title)
		if l.Body != "" {
			b.WriteString(" — " + l.Body)
		}
		b.WriteString("\n")
	}
	title := "Résumé de la nuit : 1 message"
	if len(lines) > 1 {
		title = fmt.Sprintf("Résumé de la nuit : %d messages", len(lines))
	}
	if _, ok := s.deliver(ctx, set, Message{Title: title, Body: strings.TrimSpace(b.String())}, now); !ok {
		return nil
	}
	return s.store.ClearDigest(ctx, lines[len(lines)-1].ID)
}
