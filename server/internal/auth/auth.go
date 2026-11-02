// Package auth implements the single local account: first-start setup with a one-time
// code, password login (Argon2id), passkeys (WebAuthn), server-side sessions, rate limiting
// and audit events.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/webauthn"
)

const (
	// SessionTTL is renewed on use: a session expires after 30 days without a visit.
	SessionTTL = 30 * 24 * time.Hour
	// touchEvery limits session writes to one per hour per session.
	touchEvery = time.Hour

	MinPasswordLen = 12
	// maxPasswordLen bounds the hashing work an attacker can ask for.
	maxPasswordLen = 256
	maxUsernameLen = 64

	maxFailures   = 5
	failureWindow = 10 * time.Minute
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidSetupCode   = errors.New("invalid setup code")
	ErrSetupDone          = errors.New("account already exists")
	ErrUnauthenticated    = errors.New("unauthenticated")
	ErrNotFound           = errors.New("not found")
)

// RateLimitedError says until when a client is locked out.
type RateLimitedError struct{ Until time.Time }

func (e *RateLimitedError) Error() string { return "too many attempts" }

// InvalidInputError carries a message meant for the user.
type InvalidInputError struct{ Msg string }

func (e *InvalidInputError) Error() string { return e.Msg }

type User struct {
	ID       int64
	Username string
}

type Session struct {
	ID         int64
	UserID     int64
	UserAgent  string
	IP         string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
}

// Client describes who sends a request, for sessions, rate limiting and the audit log.
type Client struct {
	IP        string
	UserAgent string
}

type Event struct {
	Kind   string
	UserID *int64
	IP     string
	Detail string
}

// Repo is the storage the service needs; pgstore implements it.
type Repo interface {
	HasUser(ctx context.Context) (bool, error)
	// CreateUser returns ErrSetupDone if an account already exists.
	CreateUser(ctx context.Context, username, passwordHash string) (User, error)
	// UserByName returns ErrNotFound for an unknown username.
	UserByName(ctx context.Context, username string) (User, string, error)
	// SoleUser returns the account, or ErrNotFound before setup.
	SoleUser(ctx context.Context) (User, error)
	PasswordHash(ctx context.Context, userID int64) (string, error)
	// SetPassword stores a new hash and, in the same transaction, ends every session of the
	// user except keep (0 ends them all). It returns how many sessions ended.
	SetPassword(ctx context.Context, userID int64, hash string, keep int64) (int, error)
	CreateSession(ctx context.Context, tokenHash []byte, s Session) (Session, error)
	// SessionByToken returns ErrNotFound for an unknown or expired session.
	SessionByToken(ctx context.Context, tokenHash []byte, now time.Time) (Session, User, error)
	TouchSession(ctx context.Context, id int64, lastSeen, expires time.Time) error
	DeleteSession(ctx context.Context, userID, id int64) error
	Sessions(ctx context.Context, userID int64, now time.Time) ([]Session, error)
	Audit(ctx context.Context, e Event) error
}

type Service struct {
	repo    Repo
	limiter *limiter
	now     func() time.Time
	// hashing bounds concurrent Argon2id runs (64 MiB each).
	hashing chan struct{}
	// dummyHash keeps a login for an unknown user as slow as for the real one.
	dummyHash string

	mu        sync.Mutex
	setupCode string

	// Set by EnablePasskeys; nil keeps passkeys off.
	webauthn   *webauthn.WebAuthn
	ceremonies *ceremonies
}

func New(repo Repo) (*Service, error) {
	dummy, err := HashPassword("not-a-real-password")
	if err != nil {
		return nil, err
	}
	return &Service{
		repo:      repo,
		limiter:   newLimiter(maxFailures, failureWindow),
		now:       time.Now,
		hashing:   make(chan struct{}, 2),
		dummyHash: dummy,
	}, nil
}

// SetClock replaces the clock, for tests.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// PrepareSetup returns a new one-time setup code when no account exists, or "" otherwise.
// The caller shows it in the server logs only: whoever can read them owns the server.
func (s *Service) PrepareSetup(ctx context.Context) (string, error) {
	has, err := s.repo.HasUser(ctx)
	if err != nil || has {
		return "", err
	}
	raw := make([]byte, 10)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	code := base32.StdEncoding.EncodeToString(raw) // 16 characters, 80 bits
	code = code[:4] + "-" + code[4:8] + "-" + code[8:12] + "-" + code[12:]
	s.mu.Lock()
	s.setupCode = code
	s.mu.Unlock()
	return code, nil
}

func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	has, err := s.repo.HasUser(ctx)
	return !has, err
}

// Setup creates the account with the setup code and opens a first session.
func (s *Service) Setup(ctx context.Context, code, username, password string, c Client) (string, error) {
	key := "setup:" + c.IP
	if err := s.checkLimit(key); err != nil {
		return "", err
	}
	s.mu.Lock()
	want := s.setupCode
	s.mu.Unlock()
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if want == "" || subtle.ConstantTimeCompare([]byte(normalized), []byte(want)) != 1 {
		s.limiter.fail(key, s.now())
		s.audit(ctx, Event{Kind: "setup_failed", IP: c.IP})
		return "", ErrInvalidSetupCode
	}
	username = strings.TrimSpace(username)
	if err := validateCredentials(username, password); err != nil {
		return "", err
	}
	hash, err := s.hash(password)
	if err != nil {
		return "", err
	}
	user, err := s.repo.CreateUser(ctx, username, hash)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.setupCode = ""
	s.mu.Unlock()
	s.limiter.reset(key)
	s.audit(ctx, Event{Kind: "account_created", UserID: &user.ID, IP: c.IP})
	return s.openSession(ctx, user, c)
}

func (s *Service) Login(ctx context.Context, username, password string, c Client) (string, error) {
	key := "login:" + c.IP
	if err := s.checkLimit(key); err != nil {
		return "", err
	}
	username = strings.TrimSpace(username)
	if len(password) > maxPasswordLen {
		password = "" // never matches (shorter than MinPasswordLen), still hashed for timing
	}
	user, hash, err := s.repo.UserByName(ctx, username)
	if errors.Is(err, ErrNotFound) {
		hash = s.dummyHash
	} else if err != nil {
		return "", err
	}
	ok, err := s.verify(password, hash)
	if err != nil {
		return "", err
	}
	if !ok || user.ID == 0 {
		s.limiter.fail(key, s.now())
		s.audit(ctx, Event{Kind: "login_failed", IP: c.IP, Detail: "username=" + truncate(username, maxUsernameLen)})
		return "", ErrInvalidCredentials
	}
	s.limiter.reset(key)
	s.audit(ctx, Event{Kind: "login", UserID: &user.ID, IP: c.IP})
	return s.openSession(ctx, user, c)
}

// Authenticate returns the session behind a cookie token and extends it.
func (s *Service) Authenticate(ctx context.Context, token string) (Session, User, error) {
	hash, ok := hashToken(token)
	if !ok {
		return Session{}, User{}, ErrUnauthenticated
	}
	now := s.now()
	sess, user, err := s.repo.SessionByToken(ctx, hash, now)
	if errors.Is(err, ErrNotFound) {
		return Session{}, User{}, ErrUnauthenticated
	} else if err != nil {
		return Session{}, User{}, err
	}
	if now.Sub(sess.LastSeenAt) >= touchEvery {
		sess.LastSeenAt, sess.ExpiresAt = now, now.Add(SessionTTL)
		if err := s.repo.TouchSession(ctx, sess.ID, sess.LastSeenAt, sess.ExpiresAt); err != nil {
			return Session{}, User{}, err
		}
	}
	return sess, user, nil
}

func (s *Service) Logout(ctx context.Context, sess Session, c Client) error {
	if err := s.repo.DeleteSession(ctx, sess.UserID, sess.ID); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	s.audit(ctx, Event{Kind: "logout", UserID: &sess.UserID, IP: c.IP})
	return nil
}

func (s *Service) Sessions(ctx context.Context, userID int64) ([]Session, error) {
	return s.repo.Sessions(ctx, userID, s.now())
}

// Revoke ends one of the user's sessions; ErrNotFound if it is not theirs.
func (s *Service) Revoke(ctx context.Context, userID, sessionID int64, c Client) error {
	if err := s.repo.DeleteSession(ctx, userID, sessionID); err != nil {
		return err
	}
	s.audit(ctx, Event{Kind: "session_revoked", UserID: &userID, IP: c.IP, Detail: fmt.Sprintf("session=%d", sessionID)})
	return nil
}

// ChangePassword replaces the password from inside a session. The current one is asked
// again, as for a new passkey, and every other session ends: whoever knew the old password
// loses the access it gave.
func (s *Service) ChangePassword(ctx context.Context, sess Session, current, next string, c Client) error {
	if err := s.confirmPassword(ctx, User{ID: sess.UserID}, current, c); err != nil {
		return err
	}
	if err := validatePassword(next); err != nil {
		return err
	}
	hash, err := s.hash(next)
	if err != nil {
		return err
	}
	ended, err := s.repo.SetPassword(ctx, sess.UserID, hash, sess.ID)
	if err != nil {
		return err
	}
	s.audit(ctx, Event{Kind: "password_changed", UserID: &sess.UserID, IP: c.IP, Detail: fmt.Sprintf("sessions_ended=%d", ended)})
	return nil
}

// ResetPassword sets a new password from the server's command line, when it is lost.
// Running it already requires access to the server, so nothing else is asked and no HTTP
// route exists for it. Every session ends; passkeys stay.
func (s *Service) ResetPassword(ctx context.Context, next string) (User, error) {
	user, err := s.repo.SoleUser(ctx)
	if err != nil {
		return User{}, err
	}
	if err := validatePassword(next); err != nil {
		return User{}, err
	}
	hash, err := s.hash(next)
	if err != nil {
		return User{}, err
	}
	ended, err := s.repo.SetPassword(ctx, user.ID, hash, 0)
	if err != nil {
		return User{}, err
	}
	s.audit(ctx, Event{Kind: "password_reset", UserID: &user.ID, IP: "command line", Detail: fmt.Sprintf("sessions_ended=%d", ended)})
	return user, nil
}

// confirmPassword checks the password again inside a session. It shares the login limit:
// it is the same password to guess.
func (s *Service) confirmPassword(ctx context.Context, user User, password string, c Client) error {
	key := "login:" + c.IP
	if err := s.checkLimit(key); err != nil {
		return err
	}
	hash, err := s.repo.PasswordHash(ctx, user.ID)
	if err != nil {
		return err
	}
	if len(password) > maxPasswordLen {
		password = ""
	}
	ok, err := s.verify(password, hash)
	if err != nil {
		return err
	}
	if !ok {
		s.limiter.fail(key, s.now())
		s.audit(ctx, Event{Kind: "password_confirm_failed", UserID: &user.ID, IP: c.IP})
		return ErrWrongPassword
	}
	return nil
}

// Audit records an event that happens outside this package, such as a settings change.
func (s *Service) Audit(ctx context.Context, e Event) { s.audit(ctx, e) }

func (s *Service) openSession(ctx context.Context, user User, c Client) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256(raw)
	now := s.now()
	_, err := s.repo.CreateSession(ctx, sum[:], Session{
		UserID:     user.ID,
		UserAgent:  truncate(c.UserAgent, 256),
		IP:         c.IP,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(SessionTTL),
	})
	return token, err
}

func (s *Service) checkLimit(key string) error {
	if blocked, until := s.limiter.blocked(key, s.now()); blocked {
		return &RateLimitedError{Until: until}
	}
	return nil
}

func (s *Service) hash(password string) (string, error) {
	s.hashing <- struct{}{}
	defer func() { <-s.hashing }()
	return HashPassword(password)
}

func (s *Service) verify(password, hash string) (bool, error) {
	s.hashing <- struct{}{}
	defer func() { <-s.hashing }()
	return VerifyPassword(password, hash)
}

// audit never fails the request; a lost audit line is logged instead.
func (s *Service) audit(ctx context.Context, e Event) {
	if err := s.repo.Audit(ctx, e); err != nil {
		slog.Error("audit event lost", "kind", e.Kind, "err", err)
	}
}

func hashToken(token string) ([]byte, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return sum[:], true
}

func validateCredentials(username, password string) error {
	if username == "" || utf8.RuneCountInString(username) > maxUsernameLen {
		return &InvalidInputError{Msg: fmt.Sprintf("username must be 1 to %d characters", maxUsernameLen)}
	}
	return validatePassword(password)
}

func validatePassword(password string) error {
	switch {
	case utf8.RuneCountInString(password) < MinPasswordLen:
		return &InvalidInputError{Msg: fmt.Sprintf("password must be at least %d characters", MinPasswordLen)}
	case len(password) > maxPasswordLen:
		return &InvalidInputError{Msg: fmt.Sprintf("password must be at most %d bytes", maxPasswordLen)}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Cut on a rune boundary.
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
