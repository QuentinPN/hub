package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Frequencies and retentions, as stored.
const (
	Daily  = "daily"
	Weekly = "weekly"

	Keep7d4w  = "7d4w"
	Keep30d   = "30d"
	KeepLast3 = "last3"
)

// slotHour is when scheduled backups run, in the server's time zone (TZ).
const slotHour = 3

// Settings are the backup settings as stored.
type Settings struct {
	Enabled   bool
	Frequency string
	Retention string
	Key       *Key
	KeySetAt  *time.Time
	UpdatedAt time.Time
}

// Run is one backup, done or under way.
type Run struct {
	ID         int64      `json:"id"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	Manual     bool       `json:"manual"`
	OK         bool       `json:"ok"`
	File       string     `json:"file"`
	Size       int64      `json:"size"`
	Error      string     `json:"error"`
}

// Store keeps the settings and the history.
type Store interface {
	BackupSettings(ctx context.Context) (Settings, error)
	SaveBackupSchedule(ctx context.Context, enabled bool, frequency, retention string) error
	SaveBackupKey(ctx context.Context, key Key) error
	StartBackupRun(ctx context.Context, manual bool, at time.Time) (int64, error)
	FinishBackupRun(ctx context.Context, r Run) error
	BackupRuns(ctx context.Context, limit int) ([]Run, error)
}

// Status is what the « Sauvegarde » section shows.
type Status struct {
	// Off: backups are turned off on this server (HUB_BACKUP=off); nothing else is filled.
	Off        bool       `json:"off"`
	Enabled    bool       `json:"enabled"`
	Frequency  string     `json:"frequency"`
	Retention  string     `json:"retention"`
	Dir        string     `json:"dir"`
	DirError   string     `json:"dirError"`
	Configured bool       `json:"configured"`
	KeySetAt   *time.Time `json:"keySetAt"`
	Running    bool       `json:"running"`
	Next       *time.Time `json:"next"`
	Kept       int        `json:"kept"`
	KeptBytes  int64      `json:"keptBytes"`
	Runs       []Run      `json:"runs"`
}

// Errors a manual backup can meet before it starts.
var (
	ErrRunning = errors.New("une sauvegarde est déjà en cours")
	ErrNoKey   = errors.New("choisir d'abord une phrase de passe")
	ErrOff     = errors.New("sauvegardes désactivées sur ce serveur (HUB_BACKUP=off)")
)

// Off stands in for the service when HUB_BACKUP=off: nothing runs, every change is refused.
// A restore from the command line still works.
type Off struct{}

func (Off) Status(context.Context) (Status, error)                   { return Status{Off: true}, nil }
func (Off) SaveSchedule(context.Context, bool, string, string) error { return ErrOff }
func (Off) SetPassphrase(context.Context, string) error              { return ErrOff }
func (Off) Start(context.Context) error                              { return ErrOff }

// Service runs backups, on schedule or on demand. One at a time.
type Service struct {
	store    Store
	pool     *pgxpool.Pool
	dir      string
	keyFiles map[string]string
	version  string
	now      func() time.Time

	mu      sync.Mutex
	running bool
	// base outlives the request that starts a manual backup.
	base context.Context
}

// NewService returns a service writing to dir. keyFiles maps each key role to its path on
// this server; a role without a path stays out.
func NewService(base context.Context, store Store, pool *pgxpool.Pool, dir string, keyFiles map[string]string, version string) *Service {
	return &Service{store: store, pool: pool, dir: dir, keyFiles: keyFiles, version: version, now: time.Now, base: base}
}

// filePrefix and fileExt name the files Hub writes, and the only ones it ever deletes.
const (
	filePrefix = "hub-"
	fileExt    = ".hubbackup"
	nameLayout = "20060102-150405"
)

// checkDir says why dir cannot take a backup, "" when it can. Hub never creates it: a
// folder missing in the container means nothing is mounted there, and the root file system
// is read-only anyway.
func checkDir(dir string) string {
	if dir == "" {
		return "Aucun dossier de destination (HUB_BACKUP_DIR)"
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "Dossier " + dir + " absent : à monter dans le compose"
	}
	f, err := os.CreateTemp(dir, ".hub-test-*")
	if err != nil {
		return "Écriture impossible dans " + dir
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return ""
}

type kept struct {
	name string
	at   time.Time
	size int64
}

func (s *Service) files() ([]kept, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []kept
	for _, e := range entries {
		at, ok := parseName(e.Name(), s.now().Location())
		if !ok || !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, kept{name: e.Name(), at: at, size: info.Size()})
	}
	return out, nil
}

func parseName(name string, loc *time.Location) (time.Time, bool) {
	if len(name) != len(filePrefix)+len(nameLayout)+len(fileExt) || name[:len(filePrefix)] != filePrefix || filepath.Ext(name) != fileExt {
		return time.Time{}, false
	}
	at, err := time.ParseInLocation(nameLayout, name[len(filePrefix):len(filePrefix)+len(nameLayout)], loc)
	return at, err == nil
}

// Status gathers the settings, the folder, the files kept and the last runs.
func (s *Service) Status(ctx context.Context) (Status, error) {
	set, err := s.store.BackupSettings(ctx)
	if err != nil {
		return Status{}, err
	}
	runs, err := s.store.BackupRuns(ctx, 10)
	if err != nil {
		return Status{}, err
	}
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()
	out := Status{Enabled: set.Enabled, Frequency: set.Frequency, Retention: set.Retention, Dir: s.dir,
		DirError: checkDir(s.dir), Configured: set.Key != nil, KeySetAt: set.KeySetAt, Running: running, Runs: runs}
	if out.DirError == "" {
		if files, err := s.files(); err == nil {
			out.Kept = len(files)
			for _, f := range files {
				out.KeptBytes += f.size
			}
		}
	}
	if set.Enabled && set.Key != nil {
		next := s.due(set, runs)
		if now := s.now(); next.Before(now) {
			next = now
		}
		out.Next = &next
	}
	return out, nil
}

// due is when the next scheduled backup should run: the first slot after the last scheduled
// one, or after the settings changed when none ran yet. A slot missed while Hub was off
// runs once at the next check.
func (s *Service) due(set Settings, runs []Run) time.Time {
	from := set.UpdatedAt
	if set.KeySetAt != nil && set.KeySetAt.After(from) {
		from = *set.KeySetAt
	}
	for _, r := range runs {
		if !r.Manual {
			if r.StartedAt.After(from) {
				from = r.StartedAt
			}
			break
		}
	}
	if from.IsZero() {
		from = s.now()
	}
	return NextSlot(from.In(s.now().Location()), set.Frequency)
}

// NextSlot is the first slot strictly after t: 03:00 every day, or on Sundays.
func NextSlot(t time.Time, frequency string) time.Time {
	slot := time.Date(t.Year(), t.Month(), t.Day(), slotHour, 0, 0, 0, t.Location())
	for !slot.After(t) || frequency == Weekly && slot.Weekday() != time.Sunday {
		slot = slot.AddDate(0, 0, 1)
	}
	return slot
}

// Start runs a backup in the background, for « Sauvegarder maintenant ».
func (s *Service) Start(ctx context.Context) error {
	set, err := s.store.BackupSettings(ctx)
	if err != nil {
		return err
	}
	if set.Key == nil {
		return ErrNoKey
	}
	if !s.claim() {
		return ErrRunning
	}
	go func() {
		defer s.release()
		s.once(s.base, set, true)
	}()
	return nil
}

func (s *Service) claim() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	return true
}

func (s *Service) release() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

// Watch runs the scheduled backups until ctx ends, checking every interval.
func (s *Service) Watch(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		s.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) tick(ctx context.Context) {
	set, err := s.store.BackupSettings(ctx)
	if err != nil {
		slog.Error("backup settings", "err", err)
		return
	}
	if !set.Enabled || set.Key == nil {
		return
	}
	runs, err := s.store.BackupRuns(ctx, 10)
	if err != nil {
		slog.Error("backup runs", "err", err)
		return
	}
	if s.now().Before(s.due(set, runs)) || !s.claim() {
		return
	}
	defer s.release()
	s.once(ctx, set, false)
}

// once writes one backup and records how it went.
func (s *Service) once(ctx context.Context, set Settings, manual bool) {
	start := s.now()
	id, err := s.store.StartBackupRun(ctx, manual, start)
	if err != nil {
		slog.Error("backup", "err", err)
		return
	}
	run := Run{ID: id, StartedAt: start, Manual: manual}
	run.File, run.Size, err = s.write(ctx, set, start)
	if err == nil {
		err = s.prune(set.Retention, run.File)
		if err != nil {
			// The backup itself is done: say so, and why older ones stayed.
			slog.Error("backup retention", "err", err)
			err = nil
		}
	}
	finished := s.now()
	run.FinishedAt, run.OK = &finished, err == nil
	if err != nil {
		run.Error = err.Error()
		slog.Error("backup failed", "manual", manual, "err", err)
	} else {
		slog.Info("backup written", "manual", manual, "file", run.File, "bytes", run.Size)
	}
	if err := s.store.FinishBackupRun(context.WithoutCancel(ctx), run); err != nil {
		slog.Error("backup", "err", err)
	}
}

// write streams the backup to a hidden file, reads it back, then gives it its name.
func (s *Service) write(ctx context.Context, set Settings, at time.Time) (string, int64, error) {
	if msg := checkDir(s.dir); msg != "" {
		return "", 0, errors.New(msg)
	}
	keys := map[string][]byte{}
	for role, path := range s.keyFiles {
		if path == "" {
			continue
		}
		body, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", 0, fmt.Errorf("fichier de clés illisible : %s", filepath.Base(path))
		}
		keys[role] = body
	}

	tmp, err := os.CreateTemp(s.dir, ".hub-*.partial")
	if err != nil {
		return "", 0, fmt.Errorf("écriture impossible dans %s", s.dir)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	sum := sha256.New()
	w, err := NewWriter(io.MultiWriter(tmp, sum), *set.Key, at)
	if err == nil {
		_, err = Dump(ctx, s.pool, w, Manifest{Format: FormatVersion, Version: s.version, CreatedAt: at}, keys)
		err = errors.Join(err, w.Close())
	}
	if err == nil {
		err = tmp.Sync()
	}
	err = errors.Join(err, tmp.Close())
	if err != nil {
		return "", 0, fmt.Errorf("écriture : %w", err)
	}

	// What reached the disk is what was written, and it starts like a backup.
	f, err := os.Open(tmp.Name())
	if err != nil {
		return "", 0, fmt.Errorf("relecture : %w", err)
	}
	again := sha256.New()
	size, err := io.Copy(again, f)
	_ = f.Close()
	if err != nil || !bytes.Equal(again.Sum(nil), sum.Sum(nil)) {
		return "", 0, errors.New("relecture : le fichier ne correspond pas à ce qui a été écrit")
	}
	if f, err = os.Open(tmp.Name()); err == nil {
		err = Peek(f)
		_ = f.Close()
	}
	if err != nil {
		return "", 0, fmt.Errorf("relecture : %w", err)
	}

	name := filePrefix + at.Format(nameLayout) + fileExt
	if err := os.Rename(tmp.Name(), filepath.Join(s.dir, name)); err != nil {
		return "", 0, fmt.Errorf("renommage : %w", err)
	}
	return name, size, nil
}

// prune deletes the backups the retention no longer keeps. The one just written always stays.
func (s *Service) prune(retention, latest string) error {
	files, err := s.files()
	if err != nil {
		return err
	}
	var errs []error
	for _, f := range Expired(files2times(files), retention, s.now()) {
		name := filePrefix + f.Format(nameLayout) + fileExt
		if name == latest {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, name)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func files2times(files []kept) []time.Time {
	out := make([]time.Time, len(files))
	for i, f := range files {
		out[i] = f.at
	}
	return out
}

// Expired returns the backups a retention lets go, among those made at the times given.
// The newest one is always kept.
func Expired(times []time.Time, retention string, now time.Time) []time.Time {
	sorted := slices.Clone(times)
	slices.SortFunc(sorted, func(a, b time.Time) int { return b.Compare(a) })
	keep := map[time.Time]bool{}
	if len(sorted) > 0 {
		keep[sorted[0]] = true
	}
	switch retention {
	case KeepLast3:
		for _, t := range sorted[:min(3, len(sorted))] {
			keep[t] = true
		}
	case Keep30d:
		for _, t := range sorted {
			if now.Sub(t) < 30*24*time.Hour {
				keep[t] = true
			}
		}
	default: // Keep7d4w: the last 7 days, then the newest of each of the 4 weeks before.
		weeks := map[int]bool{}
		for _, t := range sorted {
			age := now.Sub(t)
			switch {
			case age < 7*24*time.Hour:
				keep[t] = true
			case age < 35*24*time.Hour:
				week := int((age - 7*24*time.Hour) / (7 * 24 * time.Hour))
				if !weeks[week] {
					weeks[week], keep[t] = true, true
				}
			}
		}
	}
	var out []time.Time
	for _, t := range sorted {
		if !keep[t] {
			out = append(out, t)
		}
	}
	return out
}

// SaveSchedule changes when backups run and how many stay.
func (s *Service) SaveSchedule(ctx context.Context, enabled bool, frequency, retention string) error {
	if frequency != Daily && frequency != Weekly {
		return fmt.Errorf("fréquence inconnue")
	}
	if retention != Keep7d4w && retention != Keep30d && retention != KeepLast3 {
		return fmt.Errorf("conservation inconnue")
	}
	return s.store.SaveBackupSchedule(ctx, enabled, frequency, retention)
}

// SetPassphrase makes a new key pair from the passphrase. Backups made before keep theirs:
// each still opens with the passphrase it was made under.
func (s *Service) SetPassphrase(ctx context.Context, passphrase string) error {
	key, err := NewKey(passphrase)
	if err != nil {
		return err
	}
	return s.store.SaveBackupKey(ctx, key)
}
