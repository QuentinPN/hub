package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"hub/server/internal/auth"
	"hub/server/internal/auth/authtest"
)

var client = auth.Client{IP: "192.0.2.50", UserAgent: "test"}

const goodPassword = "correct horse battery"

func newService(t *testing.T) (*auth.Service, *authtest.Repo, *time.Time) {
	t.Helper()
	repo := authtest.New()
	svc, err := auth.New(repo)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	return svc, repo, &now
}

func setup(t *testing.T, svc *auth.Service) string {
	t.Helper()
	ctx := context.Background()
	code, err := svc.PrepareSetup(ctx)
	if err != nil || code == "" {
		t.Fatalf("code %q, err %v", code, err)
	}
	// Accepted whatever the case and surrounding spaces.
	token, err := svc.Setup(ctx, " "+strings.ToLower(code)+" ", "quentin", goodPassword, client)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := auth.HashPassword(goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=2,p=2$") {
		t.Fatalf("unexpected format: %s", hash)
	}
	if ok, _ := auth.VerifyPassword(goodPassword, hash); !ok {
		t.Fatal("right password refused")
	}
	if ok, _ := auth.VerifyPassword(goodPassword+"!", hash); ok {
		t.Fatal("wrong password accepted")
	}
	if _, err := auth.VerifyPassword(goodPassword, "$argon2id$garbage"); err == nil {
		t.Fatal("malformed hash accepted")
	}
}

func TestSetupNeedsTheCodeAndHappensOnce(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.PrepareSetup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Setup(ctx, "WRONG-CODE", "quentin", goodPassword, client); !errors.Is(err, auth.ErrInvalidSetupCode) {
		t.Fatalf("err = %v", err)
	}
	setup(t, svc)

	// The code is spent and no new one is issued once the account exists.
	if code, _ := svc.PrepareSetup(ctx); code != "" {
		t.Fatal("a setup code was issued with an existing account")
	}
	if required, _ := svc.SetupRequired(ctx); required {
		t.Fatal("setup still required")
	}
}

func TestSetupRejectsShortPassword(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	code, _ := svc.PrepareSetup(ctx)
	var invalid *auth.InvalidInputError
	if _, err := svc.Setup(ctx, code, "quentin", "short", client); !errors.As(err, &invalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoginAndSessionLifecycle(t *testing.T) {
	svc, repo, now := newService(t)
	ctx := context.Background()
	setup(t, svc)

	token, err := svc.Login(ctx, "quentin", goodPassword, client)
	if err != nil {
		t.Fatal(err)
	}
	sess, user, err := svc.Authenticate(ctx, token)
	if err != nil || user.Username != "quentin" {
		t.Fatalf("user %+v, err %v", user, err)
	}

	// Used after 29 days, the session is extended; unused for 30 more days, it expires.
	*now = now.Add(29 * 24 * time.Hour)
	if _, _, err := svc.Authenticate(ctx, token); err != nil {
		t.Fatalf("session expired too early: %v", err)
	}
	*now = now.Add(auth.SessionTTL)
	if _, _, err := svc.Authenticate(ctx, token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("expired session accepted: %v", err)
	}

	// Logout of another, fresh session.
	token, _ = svc.Login(ctx, "quentin", goodPassword, client)
	sess, _, _ = svc.Authenticate(ctx, token)
	if err := svc.Logout(ctx, sess, client); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("session still valid after logout")
	}

	want := []string{"account_created", "login", "login", "logout"}
	if got := strings.Join(repo.Kinds(), ","); got != strings.Join(want, ",") {
		t.Fatalf("audit = %s", got)
	}
}

func TestLoginRateLimit(t *testing.T) {
	svc, repo, now := newService(t)
	ctx := context.Background()
	setup(t, svc)

	for i := 0; i < 5; i++ {
		if _, err := svc.Login(ctx, "quentin", "wrong password!", client); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	// Locked out, even with the right password.
	var limited *auth.RateLimitedError
	if _, err := svc.Login(ctx, "quentin", goodPassword, client); !errors.As(err, &limited) {
		t.Fatalf("err = %v, want rate limited", err)
	}
	// Another client is not affected.
	if _, err := svc.Login(ctx, "quentin", goodPassword, auth.Client{IP: "10.0.0.2"}); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(11 * time.Minute)
	if _, err := svc.Login(ctx, "quentin", goodPassword, client); err != nil {
		t.Fatalf("still locked after the window: %v", err)
	}
	if n := strings.Count(strings.Join(repo.Kinds(), ","), "login_failed"); n != 5 {
		t.Fatalf("%d login_failed events, want 5", n)
	}
}

func TestUnknownUserAndForgedTokens(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	setup(t, svc)
	if _, err := svc.Login(ctx, "nobody", goodPassword, client); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("err = %v", err)
	}
	// The dummy hash's password must not open a session for an unknown user.
	if _, err := svc.Login(ctx, "nobody", "not-a-real-password", client); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("err = %v", err)
	}
	for _, token := range []string{"", "abc", strings.Repeat("A", 43)} {
		if _, _, err := svc.Authenticate(ctx, token); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("token %q accepted", token)
		}
	}
}

func TestRevokeOnlyOwnSessions(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	first := setup(t, svc)
	second, _ := svc.Login(ctx, "quentin", goodPassword, client)
	sess, user, _ := svc.Authenticate(ctx, second)

	list, err := svc.Sessions(ctx, user.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("%d sessions, err %v", len(list), err)
	}
	if err := svc.Revoke(ctx, user.ID+1, sess.ID, client); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("revoked someone else's session: %v", err)
	}
	if err := svc.Revoke(ctx, user.ID, sess.ID, client); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, second); err == nil {
		t.Fatal("revoked session still valid")
	}
	if _, _, err := svc.Authenticate(ctx, first); err != nil {
		t.Fatal("the other session was revoked too")
	}
}
