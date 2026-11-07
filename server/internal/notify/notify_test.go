package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"hub/server/internal/backup"
	"hub/server/internal/secrets"
)

// memStore keeps everything in memory, as the database would.
type memStore struct {
	mu        sync.Mutex
	set       Settings
	states    map[string]Send
	incidents map[string]Incident
	digest    []DigestLine
	nextID    int64
	devices   map[string]bool
	sessions  []SessionSeen
	snap      Snapshot
	// logged are the alerts kept in a service's history, by slug.
	logged map[string][]SentAlert
}

func (m *memStore) LogAlert(_ context.Context, slug string, a SentAlert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.logged == nil {
		m.logged = map[string][]SentAlert{}
	}
	m.logged[slug] = append(m.logged[slug], a)
	return nil
}

func newMem() *memStore {
	return &memStore{set: Settings{SMTPPort: 587, QuietHours: true, Alerts: map[string]bool{}},
		states: map[string]Send{}, incidents: map[string]Incident{}, devices: map[string]bool{}}
}

func (m *memStore) NotifySettings(context.Context) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.set
	s.Alerts = map[string]bool{}
	for k, v := range m.set.Alerts {
		s.Alerts[k] = v
	}
	return s, nil
}
func (m *memStore) SaveNotifySettings(_ context.Context, s Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.LastSession = m.set.LastSession
	m.set = s
	return nil
}
func (m *memStore) SetNotifyLastSession(_ context.Context, id int64) error {
	m.set.LastSession = &id
	return nil
}
func (m *memStore) ChannelStates(context.Context) (map[string]Send, error) {
	return m.states, nil
}
func (m *memStore) SaveChannelState(_ context.Context, ch string, s Send) error {
	m.states[ch] = s
	return nil
}
func (m *memStore) DeleteChannelState(_ context.Context, ch string) error {
	delete(m.states, ch)
	return nil
}
func (m *memStore) Incidents(context.Context) ([]Incident, error) {
	var out []Incident
	for _, i := range m.incidents {
		out = append(out, i)
	}
	slices.SortFunc(out, func(a, b Incident) int { return strings.Compare(a.Key, b.Key) })
	return out, nil
}
func (m *memStore) PutIncident(_ context.Context, i Incident) error {
	if old, ok := m.incidents[i.Key]; ok {
		i.Since = old.Since
	}
	m.incidents[i.Key] = i
	return nil
}
func (m *memStore) DeleteIncident(_ context.Context, key string) error {
	delete(m.incidents, key)
	return nil
}
func (m *memStore) AddDigest(_ context.Context, l DigestLine) error {
	m.nextID++
	l.ID = m.nextID
	m.digest = append(m.digest, l)
	return nil
}
func (m *memStore) Digest(context.Context) ([]DigestLine, error) { return slices.Clone(m.digest), nil }
func (m *memStore) ClearDigest(_ context.Context, upTo int64) error {
	m.digest = slices.DeleteFunc(m.digest, func(l DigestLine) bool { return l.ID <= upTo })
	return nil
}
func (m *memStore) LearnDevice(_ context.Context, label string, _ time.Time) (bool, error) {
	if m.devices[label] {
		return false, nil
	}
	m.devices[label] = true
	return true, nil
}
func (m *memStore) SessionsAfter(_ context.Context, id *int64) ([]SessionSeen, error) {
	var out []SessionSeen
	for _, s := range m.sessions {
		if id == nil || s.ID > *id {
			out = append(out, s)
		}
	}
	return out, nil
}
func (m *memStore) NotifyConditions(context.Context, time.Time) (Snapshot, error) { return m.snap, nil }

type memSecrets map[string]string

func (m memSecrets) Put(_ context.Context, name string, v secrets.Value) error {
	m[name] = v.Reveal()
	return nil
}
func (m memSecrets) Get(_ context.Context, name string) (secrets.Value, error) {
	v, ok := m[name]
	if !ok {
		return secrets.Value{}, secrets.ErrNotFound
	}
	return secrets.NewValue(v), nil
}
func (m memSecrets) Delete(_ context.Context, name string) error {
	if _, ok := m[name]; !ok {
		return secrets.ErrNotFound
	}
	delete(m, name)
	return nil
}

// fakeNtfy records what ntfy would have received. Nothing leaves the machine.
type fakeNtfy struct {
	*httptest.Server
	mu     sync.Mutex
	got    []map[string]any
	paths  []string
	status int
}

func newFakeNtfy(t *testing.T) *fakeNtfy {
	f := &fakeNtfy{status: http.StatusOK}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var body map[string]any
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.paths = append(f.paths, r.URL.Path)
		if f.status == http.StatusOK {
			f.got = append(f.got, body)
		}
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"id":"x","event":"message"}`))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeNtfy) titles() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, b := range f.got {
		out = append(out, b["title"].(string))
	}
	return out
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time                { return c.t }
func (c *clock) add(d time.Duration) time.Time { c.t = c.t.Add(d); return c.t }

// setup gives a service with ntfy on the fake server, at 14:00 on a weekday.
func setup(t *testing.T) (*Service, *memStore, *fakeNtfy, *clock) {
	t.Helper()
	store, sec, ntfy := newMem(), memSecrets{}, newFakeNtfy(t)
	svc := NewService(store, sec, nil)
	c := &clock{t: time.Date(2026, 11, 4, 14, 0, 0, 0, time.Local)}
	svc.now = c.now
	ctx := context.Background()
	if err := svc.SaveNtfy(ctx, ntfy.URL, "hub-alertes-1234"); err != nil {
		t.Fatal(err)
	}
	return svc, store, ntfy, c
}

func tick(t *testing.T, svc *Service) {
	t.Helper()
	if err := svc.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAlertAfterDelayThenRecovery(t *testing.T) {
	svc, store, ntfy, c := setup(t)
	store.snap = Snapshot{Failing: []Condition{{Key: "service:jellyfin", Alert: AlertService, Subject: "Jellyfin", Detail: "conteneurs arrêtés"}}}
	tick(t, svc)
	c.add(4 * time.Minute)
	tick(t, svc)
	if got := ntfy.titles(); len(got) != 0 {
		t.Fatalf("sent before the delay: %v", got)
	}
	c.add(time.Minute)
	tick(t, svc)
	c.add(time.Minute)
	tick(t, svc)
	if got := ntfy.titles(); !slices.Equal(got, []string{"Jellyfin est arrêté ou injoignable"}) {
		t.Fatalf("alert: %v", got)
	}
	if body := ntfy.got[0]; body["topic"] != "hub-alertes-1234" || body["priority"] != float64(4) || ntfy.paths[0] != "/" {
		t.Fatalf("publish: %v on %v", body, ntfy.paths)
	}

	store.snap = Snapshot{}
	c.add(7 * time.Minute)
	tick(t, svc)
	tick(t, svc)
	if got := ntfy.titles(); !slices.Equal(got, []string{"Jellyfin est arrêté ou injoignable", "Jellyfin est revenu"}) {
		t.Fatalf("recovery: %v", got)
	}
	if ntfy.got[1]["message"] != "Après 13 min." || len(store.incidents) != 0 {
		t.Fatalf("recovery body %q, incidents %v", ntfy.got[1]["message"], store.incidents)
	}
	if st := store.states[Ntfy]; !st.OK || st.Test {
		t.Fatalf("channel state: %+v", st)
	}
	// Both ends went to the service's history, with the channel that took them.
	got := store.logged["jellyfin"]
	if len(got) != 2 || got[0].Recovery || !got[1].Recovery || got[1].Title != "Jellyfin est revenu" ||
		!slices.Equal(got[0].Via, []string{Ntfy}) || got[0].Alert != AlertService {
		t.Fatalf("history: %+v", got)
	}
}

func TestHistoryKeepsServiceAlertsOnly(t *testing.T) {
	svc, store, _, c := setup(t)
	c.t = time.Date(2026, 11, 4, 23, 30, 0, 0, time.Local)
	store.snap = Snapshot{Failing: []Condition{
		{Key: "service:emby", Alert: AlertService, Subject: "Emby", Detail: "conteneurs arrêtés"},
		{Key: "host:pi", Alert: AlertHost, Subject: "Raspberry Pi", Detail: "offline"},
	}}
	tick(t, svc)
	c.add(5 * time.Minute)
	tick(t, svc)
	if len(store.logged) != 1 || len(store.logged["emby"]) != 1 || !slices.Equal(store.logged["emby"][0].Via, []string{ViaDigest}) {
		t.Fatalf("history: %+v", store.logged)
	}
}

func TestShortOutageSaysNothing(t *testing.T) {
	svc, store, ntfy, c := setup(t)
	store.snap = Snapshot{Failing: []Condition{{Key: "host:qnap", Alert: AlertHost, Subject: "QNAP", Detail: "offline"}}}
	tick(t, svc)
	c.add(2 * time.Minute)
	store.snap = Snapshot{}
	tick(t, svc)
	c.add(10 * time.Minute)
	tick(t, svc)
	if got := ntfy.titles(); len(got) != 0 || len(store.incidents) != 0 {
		t.Fatalf("sent %v, incidents %v", got, store.incidents)
	}
}

func TestHeldKeepsTheIncident(t *testing.T) {
	svc, store, ntfy, c := setup(t)
	store.snap = Snapshot{Failing: []Condition{{Key: "disk:mini-pc", Alert: AlertDisk, Subject: "mini-pc", Detail: "93"}}}
	if err := svc.SaveAlerts(context.Background(), map[string]bool{AlertDisk: true}, true); err != nil {
		t.Fatal(err)
	}
	tick(t, svc)
	c.add(31 * time.Minute)
	tick(t, svc)
	// Beszel stops answering: the disk is not back, only unknown.
	store.snap = Snapshot{Held: []string{"disk:"}}
	c.add(time.Hour)
	tick(t, svc)
	if got := ntfy.titles(); !slices.Equal(got, []string{"Disque à 93 % sur mini-pc"}) {
		t.Fatalf("sent %v", got)
	}
	if _, ok := store.incidents["disk:mini-pc"]; !ok {
		t.Fatal("held incident dropped")
	}
}

func TestDisabledAlertDropsItsIncidents(t *testing.T) {
	svc, store, ntfy, c := setup(t)
	store.snap = Snapshot{Failing: []Condition{{Key: "disk:nas", Alert: AlertDisk, Subject: "nas", Detail: "95"}}}
	tick(t, svc)
	c.add(time.Hour)
	tick(t, svc)
	if len(ntfy.titles()) != 0 || len(store.incidents) != 0 {
		t.Fatalf("disk is off by default: %v %v", ntfy.titles(), store.incidents)
	}
}

func TestRestartDoesNotResend(t *testing.T) {
	svc, store, ntfy, c := setup(t)
	store.snap = Snapshot{Failing: []Condition{{Key: "backup", Alert: AlertBackup, Subject: "Sauvegarde", Detail: "failed"}}}
	tick(t, svc)
	// A new process over the same store.
	again := NewService(store, svc.secrets, nil)
	again.now = c.now
	c.add(time.Minute)
	tick(t, again)
	if got := ntfy.titles(); !slices.Equal(got, []string{"Sauvegarde en échec"}) {
		t.Fatalf("sent %v", got)
	}
}

func TestQuietHoursKeepForTheMorning(t *testing.T) {
	svc, store, ntfy, c := setup(t)
	c.t = time.Date(2026, 11, 4, 23, 30, 0, 0, time.Local)
	store.snap = Snapshot{Failing: []Condition{
		{Key: "backup", Alert: AlertBackup, Subject: "Sauvegarde", Detail: "failed"},
		{Key: "host:pi", Alert: AlertHost, Subject: "Raspberry Pi", Detail: "offline"},
	}}
	tick(t, svc)
	c.add(3 * time.Minute)
	tick(t, svc)
	if got := ntfy.titles(); !slices.Equal(got, []string{"Raspberry Pi ne répond plus"}) {
		t.Fatalf("at night, only the host: %v", got)
	}
	if len(store.digest) != 1 {
		t.Fatalf("digest: %v", store.digest)
	}
	st, err := svc.Status(context.Background())
	if err != nil || st.Pending != 1 {
		t.Fatalf("pending: %+v %v", st, err)
	}
	c.t = time.Date(2026, 11, 5, 7, 59, 0, 0, time.Local)
	tick(t, svc)
	if len(ntfy.titles()) != 1 {
		t.Fatal("digest sent before 08:00")
	}
	c.add(time.Minute)
	tick(t, svc)
	got := ntfy.titles()
	if len(got) != 2 || got[1] != "Résumé de la nuit : 1 message" || !strings.Contains(ntfy.got[1]["message"].(string), "23:30 · Sauvegarde en échec") {
		t.Fatalf("digest: %v %v", got, ntfy.got)
	}
	if len(store.digest) != 0 {
		t.Fatal("digest not cleared")
	}
}

func TestFailedSendIsRetried(t *testing.T) {
	svc, store, ntfy, c := setup(t)
	ntfy.status = http.StatusTooManyRequests
	store.snap = Snapshot{Failing: []Condition{{Key: "backup", Alert: AlertBackup, Subject: "Sauvegarde", Detail: "late"}}}
	tick(t, svc)
	if st := store.states[Ntfy]; st.OK || st.Error != "trop d'envois, le serveur limite (429)" {
		t.Fatalf("state: %+v", st)
	}
	if store.incidents["backup"].NotifiedAt != nil {
		t.Fatal("marked sent")
	}
	ntfy.status = http.StatusOK
	c.add(time.Minute)
	tick(t, svc)
	if got := ntfy.titles(); !slices.Equal(got, []string{"Sauvegarde en échec"}) || ntfy.got[0]["message"] != "Aucune réussie depuis 2 jours. Détail dans Réglages, Sauvegarde." {
		t.Fatalf("sent %v %v", got, ntfy.got)
	}
}

func TestNewDevice(t *testing.T) {
	svc, store, ntfy, c := setup(t)
	firefox := "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0"
	iphone := "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"
	store.sessions = []SessionSeen{{ID: 3, UserAgent: firefox, CreatedAt: c.t}}
	if err := svc.SaveAlerts(context.Background(), map[string]bool{AlertLogin: true}, false); err != nil {
		t.Fatal(err)
	}
	tick(t, svc) // learns Firefox in silence
	store.sessions = append(store.sessions,
		SessionSeen{ID: 4, UserAgent: strings.Replace(firefox, "140", "141", 2), CreatedAt: c.t},
		SessionSeen{ID: 5, UserAgent: iphone, CreatedAt: c.t})
	c.add(time.Minute)
	tick(t, svc)
	tick(t, svc)
	if got := ntfy.titles(); !slices.Equal(got, []string{"Nouvelle connexion à Hub"}) || !strings.Contains(ntfy.got[0]["message"].(string), "Safari sur iPhone") {
		t.Fatalf("sent %v %v", got, ntfy.got)
	}
	if *store.set.LastSession != 5 {
		t.Fatalf("cursor %d", *store.set.LastSession)
	}
}

func TestBackupConditions(t *testing.T) {
	now := time.Date(2026, 11, 4, 14, 0, 0, 0, time.Local)
	at := func(h time.Duration) *time.Time { t := now.Add(-h * time.Hour); return &t }
	key := &backup.Key{Recipient: "age1x"}
	on := backup.Settings{Enabled: true, Key: key, KeySetAt: at(100)}
	done := func(h time.Duration, ok bool) backup.Run {
		return backup.Run{StartedAt: *at(h), FinishedAt: at(h), OK: ok}
	}
	for _, c := range []struct {
		name string
		set  backup.Settings
		runs []backup.Run
		want string
	}{
		{"no key", backup.Settings{Enabled: true}, nil, ""},
		{"ok", on, []backup.Run{done(10, true)}, ""},
		{"failed", on, []backup.Run{done(1, false), done(20, true)}, "failed"},
		{"late", on, []backup.Run{done(50, true)}, "late"},
		{"never", on, nil, "late"},
		{"automatic off", backup.Settings{Key: key, KeySetAt: at(100)}, nil, ""},
		{"running", on, []backup.Run{{StartedAt: now.Add(-time.Minute)}, done(5, true)}, ""},
		{"interrupted", on, []backup.Run{{StartedAt: *at(5)}, done(20, true)}, "failed"},
	} {
		svc := NewService(newMem(), memSecrets{}, fixedBackups{c.set, c.runs})
		var snap Snapshot
		svc.addBackup(context.Background(), now, &snap)
		got := ""
		if len(snap.Failing) == 1 {
			got = snap.Failing[0].Detail
		}
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

type fixedBackups struct {
	set  backup.Settings
	runs []backup.Run
}

func (f fixedBackups) BackupSettings(context.Context) (backup.Settings, error) { return f.set, nil }
func (f fixedBackups) BackupRuns(context.Context, int) ([]backup.Run, error)   { return f.runs, nil }

func TestSaveNtfyKeepsTheTopicOnItsServer(t *testing.T) {
	svc, store, ntfy, _ := setup(t)
	ctx := context.Background()
	st, err := svc.Status(ctx)
	if err != nil || st.Ntfy.URL != ntfy.URL || st.Ntfy.TopicHint != "hub-" {
		t.Fatalf("status %+v %v", st, err)
	}
	out, _ := json.Marshal(st)
	if strings.Contains(string(out), "alertes-1234") {
		t.Fatalf("topic in status: %s", out)
	}
	if err := svc.SaveNtfy(ctx, ntfy.URL+"/", ""); err != nil {
		t.Fatalf("same server, topic kept: %v", err)
	}
	if err := svc.SaveNtfy(ctx, "https://ntfy.example.org", ""); err == nil {
		t.Fatal("topic sent to another server")
	}
	for _, bad := range []string{"ftp://x", "https://u:p@ntfy.sh", "https://ntfy.sh/sujet?x=1", ""} {
		if err := svc.SaveNtfy(ctx, bad, "abc"); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if err := svc.SaveNtfy(ctx, ntfy.URL, "pas un sujet !"); err == nil {
		t.Error("accepted a bad topic")
	}
	if err := svc.Remove(ctx, Ntfy); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.secrets.(memSecrets)[secretTopic]; ok || store.set.NtfyURL != "" {
		t.Fatal("topic kept after removal")
	}
}

func TestTestRecordsTheResult(t *testing.T) {
	svc, store, ntfy, _ := setup(t)
	if err := svc.Test(context.Background(), Ntfy); err != nil {
		t.Fatal(err)
	}
	if st := store.states[Ntfy]; !st.OK || !st.Test || ntfy.got[0]["title"] != "Test de Hub" {
		t.Fatalf("state %+v, sent %v", st, ntfy.got)
	}
	ntfy.status = http.StatusForbidden
	if err := svc.Test(context.Background(), Ntfy); err == nil || strings.Contains(err.Error(), ntfy.URL) {
		t.Fatalf("error %v", err)
	}
	if err := svc.Test(context.Background(), SMTP); err != ErrNotConfigured {
		t.Fatalf("smtp: %v", err)
	}
}

func TestDeviceLabel(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36 Edg/142.0.0.0": "Edge sur Windows",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15":         "Safari sur Mac",
		"Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Mobile Safari/537.36":         "Chrome sur Android",
		"curl/8.0": "Navigateur inconnu sur système inconnu",
		"":         "un appareil sans nom",
	} {
		if got := DeviceLabel(ua); got != want {
			t.Errorf("%q: %q, want %q", ua, got, want)
		}
	}
}
