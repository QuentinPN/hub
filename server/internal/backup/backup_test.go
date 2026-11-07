package backup

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"hub/server/internal/db/dbtest"
)

const pass = "une phrase assez longue"

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	src := dbtest.Pool(t)
	// Rows across foreign keys and an identity column.
	for _, q := range []string{
		"INSERT INTO hosts (id, name, os, arch) VALUES ('h1', 'Mini-PC', 'linux', 'amd64')",
		"INSERT INTO services (slug, name, monogram, icon, color, category, host_id, favorite, position) VALUES ('emby', 'Emby', 'E', 'emby', '#52b54b', 'media', 'h1', true, 0)",
		"INSERT INTO users (username, password_hash) VALUES ('quentin', '$argon2id$x')",
		"INSERT INTO audit_events (kind, user_id) SELECT 'login', id FROM users",
		"INSERT INTO home_layout (layout) VALUES ('[{\"id\":\"latest\",\"visible\":false,\"excluded\":[]}]')",
	} {
		if _, err := src.Exec(ctx, q); err != nil {
			t.Fatal(q, err)
		}
	}

	key, err := NewKey(pass)
	if err != nil {
		t.Fatal(err)
	}
	var file bytes.Buffer
	w, err := NewWriter(&file, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string][]byte{KeySecrets: []byte("clé"), KeyAgentCA: []byte("ac")}
	if _, err := Dump(ctx, src, w, Manifest{Format: FormatVersion, Version: "test"}, keys); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Peek(bytes.NewReader(file.Bytes())); err != nil {
		t.Fatalf("Peek: %v", err)
	}

	if _, err := Open(bytes.NewReader(file.Bytes()), "pas la bonne phrase"); !errors.Is(err, ErrPassphrase) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	r, err := Open(bytes.NewReader(file.Bytes()), pass)
	if err != nil {
		t.Fatal(err)
	}
	dst := dbtest.Empty(t)
	got := map[string]string{}
	if err := Restore(ctx, dst, r, func(role string, body []byte) error {
		got[role] = string(body)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got[KeySecrets] != "clé" || got[KeyAgentCA] != "ac" {
		t.Fatalf("keys: %v", got)
	}

	var name, layout string
	var favorite bool
	if err := dst.QueryRow(ctx, "SELECT s.name, s.favorite, (SELECT layout::text FROM home_layout) FROM services s JOIN hosts h ON h.id = s.host_id").Scan(&name, &favorite, &layout); err != nil {
		t.Fatal(err)
	}
	if name != "Emby" || !favorite || layout == "" {
		t.Fatalf("restored %q %v %q", name, favorite, layout)
	}
	// The identity sequence continues after the rows restored.
	var id int64
	if err := dst.QueryRow(ctx, "INSERT INTO audit_events (kind) VALUES ('logout') RETURNING id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != 2 {
		t.Fatalf("next id %d", id)
	}

	// Never over a database in use.
	r, _ = Open(bytes.NewReader(file.Bytes()), pass)
	if err := Restore(ctx, dst, r, func(string, []byte) error { return nil }); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("second restore: %v", err)
	}
}

func TestShortPassphrase(t *testing.T) {
	if _, err := NewKey("courte"); err == nil {
		t.Fatal("short passphrase accepted")
	}
}

func TestPeekRejectsOtherFiles(t *testing.T) {
	if err := Peek(bytes.NewReader([]byte("autre chose\n"))); !errors.Is(err, ErrNotBackup) {
		t.Fatal(err)
	}
}
