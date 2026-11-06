package pgstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"hub/server/internal/api"
	"hub/server/internal/auth"
	"hub/server/internal/auth/authtest"
	"hub/server/internal/db/dbtest"
	"hub/server/internal/domain"
	"hub/server/internal/fixture"
	"hub/server/internal/icons"
	"hub/server/internal/pgstore"
)

func TestSeedOnceThenKeepCustomizations(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	if _, err := pgstore.New(ctx, pool, fixture.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE services SET favorite = true, category = 'Perso', icon = 'lidarr' WHERE slug = 'bazarr'"); err != nil {
		t.Fatal(err)
	}

	// A restart must not re-seed over the user's changes.
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	list, err := store.Services(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 34 {
		t.Fatalf("got %d services, want 34", len(list))
	}
	svc, err := store.Service(ctx, "bazarr")
	if err != nil {
		t.Fatal(err)
	}
	if !svc.Favorite || svc.Category != "Perso" || svc.Icon == nil || *svc.Icon != "lidarr" {
		t.Fatalf("customization lost: %+v", svc.ServiceSummary)
	}
	home, err := store.Home(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(home.Favorites) != 7 {
		t.Fatalf("got %d favorites, want 7", len(home.Favorites))
	}
}

func TestSeededServicesHaveIcons(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	list, err := store.Services(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, svc := range list {
		want := map[string]string{"pihole": "pi-hole", "portainer": "portainer-light"}[svc.Slug]
		if want == "" {
			want = svc.Slug
		}
		if svc.Icon == nil || *svc.Icon != want {
			t.Errorf("%s: icon = %v, want %q", svc.Slug, svc.Icon, want)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE services SET icon = '../x' WHERE slug = 'emby'"); err == nil {
		t.Fatal("an icon reference with a path was accepted")
	}
}

func TestServiceNeverObservedIsUnknown(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO services (slug, name, monogram, color, category, host_id)
		VALUES ('hasura', 'Hasura', 'H', '#1f2a36', 'Infrastructure', 'debian-mini')`); err != nil {
		t.Fatal(err)
	}
	svc, err := store.Service(ctx, "hasura")
	if err != nil {
		t.Fatal(err)
	}
	if svc.Status != domain.StatusUnknown || svc.LatencyMs != nil {
		t.Fatalf("status = %q, latency = %v", svc.Status, svc.LatencyMs)
	}
	if _, err := store.Service(ctx, "nope"); err != api.ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDiscoverySettingsPersist(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	want := api.Discovery{FollowEvents: false, RescanMinutes: 15, OnRemoval: api.RemovalAbsentOnly, ArchiveAfterDays: 30, ConfirmBelowPercent: 90}
	if err := store.UpdateDiscovery(ctx, want); err != nil {
		t.Fatal(err)
	}

	restarted, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Discovery != want || !got.Editable {
		t.Fatalf("got %+v, editable %v", got.Discovery, got.Editable)
	}
}

func TestDatabaseRejectsOutOfRangeSettings(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	bad := api.Discovery{RescanMinutes: 0, OnRemoval: api.RemovalAbsentOnly, ArchiveAfterDays: 7, ConfirmBelowPercent: 80}
	if err := store.UpdateDiscovery(ctx, bad); err == nil {
		t.Fatal("rescan of 0 minutes was stored")
	}
}

// The same scenario as the in-memory tests, on PostgreSQL: one account only, sessions
// found by token hash, revocation limited to the owner.
func TestAuthRepo(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := auth.New(store)
	if err != nil {
		t.Fatal(err)
	}
	code, err := svc.PrepareSetup(ctx)
	if err != nil || code == "" {
		t.Fatalf("code %q, err %v", code, err)
	}
	client := auth.Client{IP: "192.0.2.50", UserAgent: "test"}
	token, err := svc.Setup(ctx, code, "quentin", "correct horse battery", client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUser(ctx, "intrus", "$argon2id$x"); err != auth.ErrSetupDone {
		t.Fatalf("second account: err = %v", err)
	}

	sess, user, err := svc.Authenticate(ctx, token)
	if err != nil || user.Username != "quentin" {
		t.Fatalf("user %+v, err %v", user, err)
	}
	if _, err := svc.Login(ctx, "quentin", "wrong password!!", client); err != auth.ErrInvalidCredentials {
		t.Fatalf("err = %v", err)
	}
	if err := svc.Revoke(ctx, user.ID+1, sess.ID, client); err != auth.ErrNotFound {
		t.Fatalf("revoked someone else's session: %v", err)
	}
	if err := svc.Revoke(ctx, user.ID, sess.ID, client); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, token); err != auth.ErrUnauthenticated {
		t.Fatalf("revoked session: err = %v", err)
	}

	var kinds []string
	rows, _ := pool.Query(ctx, "SELECT kind FROM audit_events ORDER BY id")
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		kinds = append(kinds, k)
	}
	if strings.Join(kinds, ",") != "account_created,login_failed,session_revoked" {
		t.Fatalf("audit = %v", kinds)
	}
}

func TestPasskeyRepo(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := auth.New(store)
	if err != nil {
		t.Fatal(err)
	}
	const origin = "https://hub.test"
	if err := svc.EnablePasskeys("hub.test", []string{origin}); err != nil {
		t.Fatal(err)
	}
	code, _ := svc.PrepareSetup(ctx)
	client := auth.Client{IP: "192.0.2.50", UserAgent: "test"}
	token, err := svc.Setup(ctx, code, "quentin", "correct horse battery", client)
	if err != nil {
		t.Fatal(err)
	}
	_, user, err := svc.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}

	key := authtest.NewSoftKey(t, origin)
	register := func() (auth.Passkey, error) {
		creation, err := svc.BeginPasskeyRegistration(ctx, user, "correct horse battery", client)
		if err != nil {
			t.Fatal(err)
		}
		options, _ := json.Marshal(creation)
		return svc.FinishPasskeyRegistration(ctx, user, "Windows Hello", key.Create(t, options), client)
	}
	pk, err := register()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := register(); err != auth.ErrPasskeyExists {
		t.Fatalf("duplicate: err = %v", err)
	}

	// The credential goes through jsonb and still verifies, with its counter stored.
	key.Counter = 7
	assertion, err := svc.BeginPasskeyLogin(client)
	if err != nil {
		t.Fatal(err)
	}
	options, _ := json.Marshal(assertion)
	if _, err := svc.FinishPasskeyLogin(ctx, key.Get(t, options), client); err != nil {
		t.Fatal(err)
	}
	list, err := svc.Passkeys(ctx, user.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list %+v, err %v", list, err)
	}
	got := list[0]
	if got.ID != pk.ID || got.LastUsedAt == nil || got.Credential.Authenticator.SignCount != 7 || !got.Credential.Flags.BackupState {
		t.Fatalf("stored %+v", got)
	}

	if err := svc.DeletePasskey(ctx, user.ID+1, pk.ID, client); err != auth.ErrNotFound {
		t.Fatalf("someone else's passkey: err = %v", err)
	}
	if err := svc.DeletePasskey(ctx, user.ID, pk.ID, client); err != nil {
		t.Fatal(err)
	}
	if list, _ := svc.Passkeys(ctx, user.ID); len(list) != 0 {
		t.Fatalf("after delete %+v", list)
	}
}

func TestIconCache(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CachedIcon(ctx, "audiobookshelf"); !errors.Is(err, icons.ErrNotFound) {
		t.Fatalf("empty cache: %v", err)
	}
	at := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	missing := icons.CachedIcon{Ref: "audiobookshelf", FetchedAt: at}
	if err := store.PutCachedIcon(ctx, missing); err != nil {
		t.Fatal(err)
	}
	got, err := store.CachedIcon(ctx, "audiobookshelf")
	if err != nil || got.Icon != nil || !got.FetchedAt.Equal(at) {
		t.Fatalf("missing row: %+v, %v", got, err)
	}
	found := icons.CachedIcon{Ref: "audiobookshelf", Icon: &icons.Icon{ContentType: "image/svg+xml", Body: []byte("<svg/>")}, FetchedAt: at.Add(time.Hour)}
	if err := store.PutCachedIcon(ctx, found); err != nil {
		t.Fatal(err)
	}
	got, err = store.CachedIcon(ctx, "audiobookshelf")
	if err != nil || got.Icon == nil || string(got.Icon.Body) != "<svg/>" || got.Icon.ContentType != "image/svg+xml" {
		t.Fatalf("found row: %+v, %v", got, err)
	}
	bad := icons.CachedIcon{Ref: "x", Icon: &icons.Icon{ContentType: "text/html", Body: []byte("<p>")}, FetchedAt: at}
	if err := store.PutCachedIcon(ctx, bad); err == nil {
		t.Fatal("an HTML body was cached")
	}
}

func TestSetPassword(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SoleUser(ctx); err != auth.ErrNotFound {
		t.Fatalf("before setup: err = %v", err)
	}
	svc, err := auth.New(store)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := svc.PrepareSetup(ctx)
	client := auth.Client{IP: "192.0.2.50", UserAgent: "test"}
	mine, err := svc.Setup(ctx, code, "quentin", "correct horse battery", client)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := svc.Login(ctx, "quentin", "correct horse battery", client)
	sess, user, _ := svc.Authenticate(ctx, mine)
	if got, err := store.SoleUser(ctx); err != nil || got != user {
		t.Fatalf("sole user %+v, err %v", got, err)
	}

	if _, err := store.SetPassword(ctx, user.ID+1, "$argon2id$x", 0); err != auth.ErrNotFound {
		t.Fatalf("unknown user: err = %v", err)
	}
	// The CHECK on the hash format makes the whole transaction fail: no session ends.
	if _, err := store.SetPassword(ctx, user.ID, "plain text", 0); err == nil {
		t.Fatal("a non-Argon2id hash was stored")
	}
	if _, _, err := svc.Authenticate(ctx, other); err != nil {
		t.Fatalf("a failed change ended a session: %v", err)
	}

	if err := svc.ChangePassword(ctx, sess, "correct horse battery", "a brand new passphrase", client); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, mine); err != nil {
		t.Fatalf("current session ended: %v", err)
	}
	if _, _, err := svc.Authenticate(ctx, other); err != auth.ErrUnauthenticated {
		t.Fatalf("other session: err = %v", err)
	}
	if _, err := svc.ResetPassword(ctx, "yet another passphrase"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, mine); err != auth.ErrUnauthenticated {
		t.Fatalf("session after reset: err = %v", err)
	}
	if _, err := svc.Login(ctx, "quentin", "yet another passphrase", client); err != nil {
		t.Fatalf("login after reset: %v", err)
	}
}

func TestDeleteSessions(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := auth.New(store)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := svc.PrepareSetup(ctx)
	client := auth.Client{IP: "192.0.2.50", UserAgent: "test"}
	mine, err := svc.Setup(ctx, code, "quentin", "correct horse battery", client)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := svc.Login(ctx, "quentin", "correct horse battery", client)
	sess, _, _ := svc.Authenticate(ctx, mine)

	if n, err := svc.RevokeAll(ctx, sess, false, client); err != nil || n != 1 {
		t.Fatalf("others: %d ended, err %v", n, err)
	}
	if _, _, err := svc.Authenticate(ctx, other); err != auth.ErrUnauthenticated {
		t.Fatalf("other session: err = %v", err)
	}
	if n, err := svc.RevokeAll(ctx, sess, true, client); err != nil || n != 1 {
		t.Fatalf("all: %d ended, err %v", n, err)
	}
	if _, _, err := svc.Authenticate(ctx, mine); err != auth.ErrUnauthenticated {
		t.Fatalf("this session: err = %v", err)
	}
}

// Without demo data, a new database holds nothing invented.
func TestEmptyDataSourceSeedsNothing(t *testing.T) {
	ctx := context.Background()
	store, err := pgstore.New(ctx, dbtest.Pool(t), fixture.Empty())
	if err != nil {
		t.Fatal(err)
	}
	services, err := store.Services(ctx)
	if err != nil || len(services) != 0 {
		t.Fatalf("services %v, err %v", services, err)
	}
	hosts, err := store.AgentHosts(ctx, time.Now())
	if err != nil || len(hosts) != 0 {
		t.Fatalf("hosts %v, err %v", hosts, err)
	}
	home, err := store.Home(ctx)
	if err != nil || len(home.Sessions)+len(home.Upcoming)+len(home.Hosts)+len(home.Favorites) != 0 || home.Summary.Total != 0 {
		t.Fatalf("home %+v, err %v", home, err)
	}
	// The browser relies on empty lists, never null.
	body, err := json.Marshal(home)
	if err != nil || strings.Contains(string(body), "null") {
		t.Fatalf("home JSON %s, err %v", body, err)
	}
}
