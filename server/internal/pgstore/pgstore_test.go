package pgstore_test

import (
	"context"
	"strings"
	"testing"

	"hub/server/internal/api"
	"hub/server/internal/auth"
	"hub/server/internal/db/dbtest"
	"hub/server/internal/domain"
	"hub/server/internal/fixture"
	"hub/server/internal/pgstore"
)

func TestSeedOnceThenKeepCustomizations(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	if _, err := pgstore.New(ctx, pool, fixture.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE services SET favorite = true, category = 'Perso' WHERE slug = 'bazarr'"); err != nil {
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
	if !svc.Favorite || svc.Category != "Perso" {
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
