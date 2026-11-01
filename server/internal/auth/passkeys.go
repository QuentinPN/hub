package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const (
	// ceremonyTTL is how long a passkey prompt stays valid between begin and finish.
	ceremonyTTL = 5 * time.Minute
	// maxCeremonies bounds the pending prompts anyone can open without a session.
	maxCeremonies  = 64
	maxPasskeyName = 64
)

var (
	ErrPasskeysDisabled = errors.New("passkeys disabled")
	// ErrWrongPassword is a failed password confirmation inside a session, kept apart from
	// ErrInvalidCredentials so the client does not take it for an expired session.
	ErrWrongPassword   = errors.New("wrong password")
	ErrPasskeyRejected = errors.New("passkey rejected")
	ErrPasskeyExists   = errors.New("passkey already registered")
)

// Passkey is a WebAuthn credential registered on the account.
type Passkey struct {
	ID         int64
	UserID     int64
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	Credential webauthn.Credential
}

// PasskeyRepo is the storage passkeys need on top of Repo.
type PasskeyRepo interface {
	PasswordHash(ctx context.Context, userID int64) (string, error)
	// UserHandle is the account's random WebAuthn user handle.
	UserHandle(ctx context.Context, userID int64) ([]byte, error)
	// UserByHandle returns ErrNotFound for an unknown handle.
	UserByHandle(ctx context.Context, handle []byte) (User, error)
	Passkeys(ctx context.Context, userID int64) ([]Passkey, error)
	// CreatePasskey returns ErrPasskeyExists if the credential is already registered.
	CreatePasskey(ctx context.Context, p Passkey) (Passkey, error)
	// UsePasskey stores the credential's new counter and flags after a login.
	UsePasskey(ctx context.Context, id int64, cred webauthn.Credential, usedAt time.Time) error
	// DeletePasskey returns ErrNotFound if the passkey is not the user's.
	DeletePasskey(ctx context.Context, userID, id int64) error
}

// EnablePasskeys turns passkeys on for one relying party: rpID is the host name the
// passkeys are bound to, origins the exact URLs the dashboard is opened from. Passkeys
// cannot move to another name later; the password stays as the way back in.
func (s *Service) EnablePasskeys(rpID string, origins []string) error {
	w, err := webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: "Hub",
		RPOrigins:     origins,
		// Discoverable credentials with user verification (Windows Hello, Touch ID…): the
		// passkey alone signs in, so it must prove the user, not just a touch.
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		},
		AttestationPreference: protocol.PreferNoAttestation,
	})
	if err != nil {
		return err
	}
	s.webauthn = w
	s.ceremonies = &ceremonies{m: map[string]ceremony{}}
	return nil
}

func (s *Service) PasskeysEnabled() bool { return s.webauthn != nil }

// BeginPasskeyRegistration asks for the password again: a stolen session cookie alone must
// not be enough to plant a lasting way in.
func (s *Service) BeginPasskeyRegistration(ctx context.Context, user User, password string, c Client) (*protocol.CredentialCreation, error) {
	repo, err := s.passkeyRepo()
	if err != nil {
		return nil, err
	}
	if err := s.confirmPassword(ctx, repo, user, password, c); err != nil {
		return nil, err
	}
	wu, _, err := s.webauthnUser(ctx, repo, user)
	if err != nil {
		return nil, err
	}
	creation, data, err := s.webauthn.BeginRegistration(wu,
		webauthn.WithExclusions(webauthn.Credentials(wu.creds).CredentialDescriptors()))
	if err != nil {
		return nil, err
	}
	s.ceremonies.put(*data, user.ID, s.now())
	return creation, nil
}

// FinishPasskeyRegistration checks the authenticator's answer and stores the passkey.
func (s *Service) FinishPasskeyRegistration(ctx context.Context, user User, name string, response []byte, c Client) (Passkey, error) {
	repo, err := s.passkeyRepo()
	if err != nil {
		return Passkey{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxPasskeyName {
		return Passkey{}, &InvalidInputError{Msg: fmt.Sprintf("name must be 1 to %d characters", maxPasskeyName)}
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return Passkey{}, ErrPasskeyRejected
	}
	data, ok := s.ceremonies.take(parsed.Response.CollectedClientData.Challenge, user.ID, s.now())
	if !ok {
		return Passkey{}, ErrPasskeyRejected
	}
	wu, _, err := s.webauthnUser(ctx, repo, user)
	if err != nil {
		return Passkey{}, err
	}
	cred, err := s.webauthn.CreateCredential(wu, data, parsed)
	if err != nil {
		return Passkey{}, ErrPasskeyRejected
	}
	pk, err := repo.CreatePasskey(ctx, Passkey{UserID: user.ID, Name: name, CreatedAt: s.now(), Credential: *cred})
	if err != nil {
		return Passkey{}, err
	}
	s.audit(ctx, Event{Kind: "passkey_added", UserID: &user.ID, IP: c.IP, Detail: fmt.Sprintf("passkey=%d", pk.ID)})
	return pk, nil
}

// BeginPasskeyLogin opens a discoverable login: the browser offers the passkeys it knows
// for this site, without asking for a username first.
func (s *Service) BeginPasskeyLogin(c Client) (*protocol.CredentialAssertion, error) {
	if _, err := s.passkeyRepo(); err != nil {
		return nil, err
	}
	if err := s.checkLimit("passkey:" + c.IP); err != nil {
		return nil, err
	}
	assertion, data, err := s.webauthn.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, err
	}
	s.ceremonies.put(*data, 0, s.now())
	return assertion, nil
}

// FinishPasskeyLogin checks the signed assertion and opens a session. Failures count
// toward their own limit, so a faulty authenticator cannot lock out the password.
func (s *Service) FinishPasskeyLogin(ctx context.Context, response []byte, c Client) (string, error) {
	repo, err := s.passkeyRepo()
	if err != nil {
		return "", err
	}
	key := "passkey:" + c.IP
	if err := s.checkLimit(key); err != nil {
		return "", err
	}
	fail := func(reason string) (string, error) {
		s.limiter.fail(key, s.now())
		s.audit(ctx, Event{Kind: "login_failed", IP: c.IP, Detail: "passkey: " + reason})
		return "", ErrInvalidCredentials
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return fail("malformed response")
	}
	data, ok := s.ceremonies.take(parsed.Response.CollectedClientData.Challenge, 0, s.now())
	if !ok {
		return fail("unknown or expired challenge")
	}

	// The handler runs inside the library; storage errors are kept apart so they are
	// reported as server errors, not as a failed login.
	var (
		user     User
		passkeys []Passkey
		repoErr  error
	)
	handler := func(_, userHandle []byte) (webauthn.User, error) {
		user, repoErr = repo.UserByHandle(ctx, userHandle)
		if repoErr != nil {
			if errors.Is(repoErr, ErrNotFound) {
				repoErr = nil
				return nil, ErrNotFound
			}
			return nil, repoErr
		}
		var wu *webauthnUser
		wu, passkeys, repoErr = s.webauthnUser(ctx, repo, user)
		if repoErr != nil {
			return nil, repoErr
		}
		return wu, nil
	}
	_, cred, err := s.webauthn.ValidatePasskeyLogin(handler, data, parsed)
	if repoErr != nil {
		return "", repoErr
	}
	if err != nil {
		return fail("verification failed")
	}
	// A counter that goes backwards means two copies of the same key exist.
	if cred.Authenticator.CloneWarning {
		return fail("signature counter went backwards")
	}
	var pk Passkey
	for _, p := range passkeys {
		if bytes.Equal(p.Credential.ID, cred.ID) {
			pk = p
		}
	}
	if pk.ID == 0 {
		return fail("unknown credential")
	}
	if err := repo.UsePasskey(ctx, pk.ID, *cred, s.now()); err != nil {
		return "", err
	}
	s.limiter.reset(key)
	s.audit(ctx, Event{Kind: "login", UserID: &user.ID, IP: c.IP, Detail: fmt.Sprintf("passkey=%d", pk.ID)})
	return s.openSession(ctx, user, c)
}

func (s *Service) Passkeys(ctx context.Context, userID int64) ([]Passkey, error) {
	repo, err := s.passkeyRepo()
	if err != nil {
		return nil, err
	}
	return repo.Passkeys(ctx, userID)
}

// DeletePasskey removes one of the user's passkeys; ErrNotFound if it is not theirs.
func (s *Service) DeletePasskey(ctx context.Context, userID, id int64, c Client) error {
	repo, err := s.passkeyRepo()
	if err != nil {
		return err
	}
	if err := repo.DeletePasskey(ctx, userID, id); err != nil {
		return err
	}
	s.audit(ctx, Event{Kind: "passkey_deleted", UserID: &userID, IP: c.IP, Detail: fmt.Sprintf("passkey=%d", id)})
	return nil
}

func (s *Service) passkeyRepo() (PasskeyRepo, error) {
	repo, ok := s.repo.(PasskeyRepo)
	if !ok || s.webauthn == nil {
		return nil, ErrPasskeysDisabled
	}
	return repo, nil
}

// confirmPassword shares the login limit: it is the same password to guess.
func (s *Service) confirmPassword(ctx context.Context, repo PasskeyRepo, user User, password string, c Client) error {
	key := "login:" + c.IP
	if err := s.checkLimit(key); err != nil {
		return err
	}
	hash, err := repo.PasswordHash(ctx, user.ID)
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

func (s *Service) webauthnUser(ctx context.Context, repo PasskeyRepo, user User) (*webauthnUser, []Passkey, error) {
	handle, err := repo.UserHandle(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}
	list, err := repo.Passkeys(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}
	wu := &webauthnUser{user: user, handle: handle}
	for _, p := range list {
		wu.creds = append(wu.creds, p.Credential)
	}
	return wu, list, nil
}

// webauthnUser adapts the account to the library's User interface.
type webauthnUser struct {
	user   User
	handle []byte
	creds  []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte                         { return u.handle }
func (u *webauthnUser) WebAuthnName() string                       { return u.user.Username }
func (u *webauthnUser) WebAuthnDisplayName() string                { return u.user.Username }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// ceremonies keeps the server half of pending passkey prompts in memory, keyed by
// challenge. Each is single use; a restart simply cancels the prompts in flight.
type ceremonies struct {
	mu sync.Mutex
	m  map[string]ceremony
}

type ceremony struct {
	data    webauthn.SessionData
	userID  int64 // 0 for a login, the account for a registration
	expires time.Time
}

func (c *ceremonies) put(data webauthn.SessionData, userID int64, now time.Time) {
	// Expiry is checked here with the service clock, not by the library.
	data.Expires = time.Time{}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= maxCeremonies {
		var oldest string
		for k, v := range c.m {
			if !now.Before(v.expires) {
				delete(c.m, k)
			} else if oldest == "" || v.expires.Before(c.m[oldest].expires) {
				oldest = k
			}
		}
		if len(c.m) >= maxCeremonies {
			delete(c.m, oldest)
		}
	}
	c.m[data.Challenge] = ceremony{data: data, userID: userID, expires: now.Add(ceremonyTTL)}
}

// take returns and forgets the ceremony for a challenge if it is still valid and was
// opened for the same account (0 for a login).
func (c *ceremonies) take(challenge string, userID int64, now time.Time) (webauthn.SessionData, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cer, ok := c.m[challenge]
	if !ok {
		return webauthn.SessionData{}, false
	}
	delete(c.m, challenge)
	if cer.userID != userID || !now.Before(cer.expires) {
		return webauthn.SessionData{}, false
	}
	return cer.data, true
}
