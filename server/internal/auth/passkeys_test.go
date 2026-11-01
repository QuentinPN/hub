package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"hub/server/internal/auth"
	"hub/server/internal/auth/authtest"
)

const origin = "http://localhost:5173"

// passkeyService returns a service with an account and passkeys on for localhost.
func passkeyService(t *testing.T) (*auth.Service, *authtest.Repo, *time.Time, auth.User) {
	t.Helper()
	svc, repo, now := newService(t)
	if err := svc.EnablePasskeys("localhost", []string{origin}); err != nil {
		t.Fatal(err)
	}
	_, user, err := svc.Authenticate(context.Background(), setup(t, svc))
	if err != nil {
		t.Fatal(err)
	}
	return svc, repo, now, user
}

func toJSON(t *testing.T, v any) []byte {
	t.Helper()
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// register adds key to the account and returns the stored passkey.
func register(t *testing.T, svc *auth.Service, user auth.User, key *authtest.SoftKey) auth.Passkey {
	t.Helper()
	ctx := context.Background()
	creation, err := svc.BeginPasskeyRegistration(ctx, user, goodPassword, client)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := svc.FinishPasskeyRegistration(ctx, user, " PC du salon ", key.Create(t, toJSON(t, creation)), client)
	if err != nil {
		t.Fatal(err)
	}
	return pk
}

// loginResponse runs the browser half of a passkey login.
func loginResponse(t *testing.T, svc *auth.Service, key *authtest.SoftKey) []byte {
	t.Helper()
	assertion, err := svc.BeginPasskeyLogin(client)
	if err != nil {
		t.Fatal(err)
	}
	return key.Get(t, toJSON(t, assertion))
}

func TestPasskeyRegisterAndLogin(t *testing.T) {
	svc, repo, now, user := passkeyService(t)
	ctx := context.Background()
	key := authtest.NewSoftKey(t, origin)
	pk := register(t, svc, user, key)
	if pk.Name != "PC du salon" || pk.LastUsedAt != nil || !pk.Credential.Flags.BackupState {
		t.Fatalf("stored passkey %+v", pk)
	}

	*now = now.Add(time.Minute)
	token, err := svc.FinishPasskeyLogin(ctx, loginResponse(t, svc, key), client)
	if err != nil {
		t.Fatal(err)
	}
	if _, got, err := svc.Authenticate(ctx, token); err != nil || got.ID != user.ID {
		t.Fatalf("session user %+v, err %v", got, err)
	}
	list, _ := svc.Passkeys(ctx, user.ID)
	if len(list) != 1 || list[0].LastUsedAt == nil || !list[0].LastUsedAt.Equal(*now) {
		t.Fatalf("passkeys after login %+v", list)
	}

	kinds := map[string]bool{}
	for _, e := range repo.Events {
		kinds[e.Kind] = true
	}
	if !kinds["passkey_added"] || !kinds["login"] {
		t.Fatalf("audit %+v", repo.Events)
	}

	// The same key cannot be registered twice: the browser is told to exclude it.
	creation, err := svc.BeginPasskeyRegistration(ctx, user, goodPassword, client)
	if err != nil {
		t.Fatal(err)
	}
	if len(creation.Response.CredentialExcludeList) != 1 {
		t.Fatalf("exclude list %+v", creation.Response.CredentialExcludeList)
	}
	if _, err := svc.FinishPasskeyRegistration(ctx, user, "encore", key.Create(t, toJSON(t, creation)), client); !errors.Is(err, auth.ErrPasskeyExists) {
		t.Fatalf("duplicate: err = %v", err)
	}
}

func TestPasskeyRegistrationNeedsPassword(t *testing.T) {
	svc, repo, _, user := passkeyService(t)
	ctx := context.Background()
	if _, err := svc.BeginPasskeyRegistration(ctx, user, "wrong password!", client); !errors.Is(err, auth.ErrWrongPassword) {
		t.Fatalf("err = %v", err)
	}
	if last := repo.Events[len(repo.Events)-1]; last.Kind != "password_confirm_failed" {
		t.Fatalf("audit %+v", last)
	}
	// It is the login password: guesses share the login limit.
	for range 4 {
		_, _ = svc.BeginPasskeyRegistration(ctx, user, "wrong password!", client)
	}
	var limited *auth.RateLimitedError
	if _, err := svc.BeginPasskeyRegistration(ctx, user, goodPassword, client); !errors.As(err, &limited) {
		t.Fatalf("err = %v", err)
	}
}

func TestPasskeyRegistrationRejects(t *testing.T) {
	svc, _, now, user := passkeyService(t)
	ctx := context.Background()
	begin := func() []byte {
		creation, err := svc.BeginPasskeyRegistration(ctx, user, goodPassword, client)
		if err != nil {
			t.Fatal(err)
		}
		return toJSON(t, creation)
	}

	if _, err := svc.FinishPasskeyRegistration(ctx, user, "", authtest.NewSoftKey(t, origin).Create(t, begin()), client); err == nil {
		t.Fatal("empty name accepted")
	}

	cases := map[string]func() []byte{
		"other origin": func() []byte { return authtest.NewSoftKey(t, "https://evil.example").Create(t, begin()) },
		"no user verification": func() []byte {
			k := authtest.NewSoftKey(t, origin)
			k.SkipUV = true
			return k.Create(t, begin())
		},
		"expired prompt": func() []byte {
			opts := begin()
			*now = now.Add(6 * time.Minute)
			return authtest.NewSoftKey(t, origin).Create(t, opts)
		},
		"unknown challenge": func() []byte {
			opts := begin()
			resp := authtest.NewSoftKey(t, origin).Create(t, opts)
			// Spent by a first answer.
			if _, err := svc.FinishPasskeyRegistration(ctx, user, "first", resp, client); err != nil {
				t.Fatal(err)
			}
			return authtest.NewSoftKey(t, origin).Create(t, opts)
		},
		"garbage": func() []byte { return []byte(`{"id":"x"}`) },
	}
	for name, response := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.FinishPasskeyRegistration(ctx, user, "clé", response(), client); !errors.Is(err, auth.ErrPasskeyRejected) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestPasskeyLoginRejects(t *testing.T) {
	svc, repo, now, user := passkeyService(t)
	ctx := context.Background()
	soft := authtest.NewSoftKey(t, origin)
	register(t, svc, user, soft)

	replayed := loginResponse(t, svc, soft)
	if _, err := svc.FinishPasskeyLogin(ctx, replayed, client); err != nil {
		t.Fatal(err)
	}

	type rejected struct {
		response func() []byte
		reason   string // audited detail
	}
	cases := map[string]rejected{
		"replayed answer": {func() []byte { return replayed }, "unknown or expired challenge"},
		"no user verification": {func() []byte {
			soft.SkipUV = true
			defer func() { soft.SkipUV = false }()
			return loginResponse(t, svc, soft)
		}, "verification failed"},
		"other origin": {func() []byte {
			soft.Origin = "https://evil.example"
			defer func() { soft.Origin = origin }()
			return loginResponse(t, svc, soft)
		}, "verification failed"},
		"unregistered key": {func() []byte {
			stranger := authtest.NewSoftKey(t, origin)
			// Registered elsewhere, so it signs for the same user handle.
			register(t, svc, user, stranger)
			ctx := context.Background()
			list, _ := svc.Passkeys(ctx, user.ID)
			if err := svc.DeletePasskey(ctx, user.ID, list[len(list)-1].ID, client); err != nil {
				t.Fatal(err)
			}
			return loginResponse(t, svc, stranger)
		}, "verification failed"},
		"expired prompt": {func() []byte {
			resp := loginResponse(t, svc, soft)
			*now = now.Add(6 * time.Minute)
			return resp
		}, "unknown or expired challenge"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.FinishPasskeyLogin(ctx, c.response(), client); !errors.Is(err, auth.ErrInvalidCredentials) {
				t.Fatalf("err = %v", err)
			}
			if last := repo.Events[len(repo.Events)-1]; last.Kind != "login_failed" || last.Detail != "passkey: "+c.reason {
				t.Fatalf("audit %+v", last)
			}
		})
	}
}

func TestPasskeyCloneIsRejected(t *testing.T) {
	svc, repo, _, user := passkeyService(t)
	ctx := context.Background()
	key := authtest.NewSoftKey(t, origin)
	key.Counter = 5
	register(t, svc, user, key)
	key.Counter = 6
	if _, err := svc.FinishPasskeyLogin(ctx, loginResponse(t, svc, key), client); err != nil {
		t.Fatal(err)
	}
	// A copy of the key that has not moved since.
	if _, err := svc.FinishPasskeyLogin(ctx, loginResponse(t, svc, key), client); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("err = %v", err)
	}
	if last := repo.Events[len(repo.Events)-1]; last.Detail != "passkey: signature counter went backwards" {
		t.Fatalf("audit %+v", last)
	}
}

func TestPasskeyLoginLimitIsSeparate(t *testing.T) {
	svc, _, _, user := passkeyService(t)
	ctx := context.Background()
	for range 5 {
		if _, err := svc.FinishPasskeyLogin(ctx, []byte(`{}`), client); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("err = %v", err)
		}
	}
	var limited *auth.RateLimitedError
	if _, err := svc.BeginPasskeyLogin(client); !errors.As(err, &limited) {
		t.Fatalf("begin: err = %v", err)
	}
	// The password still works.
	if _, err := svc.Login(ctx, user.Username, goodPassword, client); err != nil {
		t.Fatalf("password login: %v", err)
	}
}

func TestDeletePasskeyOnlyOwn(t *testing.T) {
	svc, repo, _, user := passkeyService(t)
	ctx := context.Background()
	key := authtest.NewSoftKey(t, origin)
	pk := register(t, svc, user, key)
	if err := svc.DeletePasskey(ctx, user.ID+1, pk.ID, client); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("other user: err = %v", err)
	}
	if err := svc.DeletePasskey(ctx, user.ID, pk.ID, client); err != nil {
		t.Fatal(err)
	}
	if last := repo.Events[len(repo.Events)-1]; last.Kind != "passkey_deleted" {
		t.Fatalf("audit %+v", last)
	}
	if _, err := svc.FinishPasskeyLogin(ctx, loginResponse(t, svc, key), client); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("deleted key: err = %v", err)
	}
}

func TestPasskeysDisabledByDefault(t *testing.T) {
	svc, _, _ := newService(t)
	if svc.PasskeysEnabled() {
		t.Fatal("enabled without EnablePasskeys")
	}
	if _, err := svc.BeginPasskeyLogin(client); !errors.Is(err, auth.ErrPasskeysDisabled) {
		t.Fatalf("err = %v", err)
	}
}
