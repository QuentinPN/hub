package pgstore_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"hub/server/internal/db/dbtest"
	"hub/server/internal/fixture"
	"hub/server/internal/pgstore"
	"hub/server/internal/secrets"
)

func TestSecretsSealedInPostgres(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	store, err := pgstore.New(ctx, pool, fixture.New())
	if err != nil {
		t.Fatal(err)
	}
	line := func(id string) string {
		l, err := secrets.NewKeyLine(time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return id + " " + strings.Fields(l)[1]
	}
	k1, k2 := line("k1"), line("k2")
	kr1, err := secrets.ParseKeyring([]byte(k1))
	if err != nil {
		t.Fatal(err)
	}
	s1 := secrets.NewStore(kr1, store)

	if err := s1.Put(ctx, "emby:api_key", secrets.NewValue("first")); err != nil {
		t.Fatal(err)
	}
	if err := s1.Put(ctx, "emby:api_key", secrets.NewValue("second")); err != nil {
		t.Fatal(err)
	}
	if v, err := s1.Get(ctx, "emby:api_key"); err != nil || v.Reveal() != "second" {
		t.Fatalf("Get = %q, %v", v.Reveal(), err)
	}
	var stored []byte
	if err := pool.QueryRow(ctx, "SELECT ciphertext FROM secrets WHERE name = 'emby:api_key'").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "second") {
		t.Fatal("the database holds the secret in clear")
	}
	infos, err := s1.List(ctx)
	if err != nil || len(infos) != 1 || infos[0].Name != "emby:api_key" {
		t.Fatalf("List = %+v, %v", infos, err)
	}

	kr12, err := secrets.ParseKeyring([]byte(k1 + "\n" + k2))
	if err != nil {
		t.Fatal(err)
	}
	if n, err := secrets.NewStore(kr12, store).Rotate(ctx); n != 1 || err != nil {
		t.Fatalf("Rotate = %d, %v", n, err)
	}
	kr2, err := secrets.ParseKeyring([]byte(k2))
	if err != nil {
		t.Fatal(err)
	}
	if v, err := secrets.NewStore(kr2, store).Get(ctx, "emby:api_key"); err != nil || v.Reveal() != "second" {
		t.Fatalf("Get after rotation = %q, %v", v.Reveal(), err)
	}

	if err := s1.Delete(ctx, "emby:api_key"); err != nil {
		t.Fatal(err)
	}
	if _, err := s1.Get(ctx, "emby:api_key"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("Get after delete: %v", err)
	}
	if err := s1.Delete(ctx, "emby:api_key"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO secrets (name, key_id, ciphertext) VALUES ('Bad Name', 'k1', $1)", make([]byte, 40)); err == nil {
		t.Fatal("the database accepted an invalid name")
	}
}
