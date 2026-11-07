package main

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hub/server/internal/backup"
	"hub/server/internal/db/dbtest"
)

// withStdin feeds the passphrase as a pipe would.
func withStdin(t *testing.T, line string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old; _ = r.Close() })
}

func TestRestoreCommand(t *testing.T) {
	ctx := context.Background()
	src := dbtest.Pool(t)
	if _, err := src.Exec(ctx, "INSERT INTO hosts (id, name, os, arch) VALUES ('qnap', 'QNAP', 'linux', 'amd64')"); err != nil {
		t.Fatal(err)
	}
	key, err := backup.NewKey("une phrase assez longue")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "hub.hubbackup")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	w, err := backup.NewWriter(f, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backup.Dump(ctx, src, w, backup.Manifest{Format: backup.FormatVersion, Version: "test"},
		map[string][]byte{backup.KeySecrets: []byte("k1"), backup.KeyAgentCA: []byte("ca")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	withStdin(t, "une phrase assez longue")
	if err := verifyBackup(file); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// The empty database the restore targets, by URL like the real command.
	dst := dbtest.Empty(t)
	var schema string
	if err := dst.QueryRow(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("HUB_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	keys := t.TempDir()
	paths := map[string]string{
		backup.KeySecrets: filepath.Join(keys, "secrets.key"),
		backup.KeyAgentCA: filepath.Join(keys, "agent-ca.pem"),
	}
	withStdin(t, "pas la bonne phrase")
	if err := restoreBackup(ctx, file, u.String(), paths); err == nil {
		t.Fatal("wrong passphrase restored")
	}
	withStdin(t, "une phrase assez longue")
	if err := restoreBackup(ctx, file, u.String(), paths); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(paths[backup.KeySecrets]); string(b) != "k1" {
		t.Fatalf("secrets key %q", b)
	}
	var name string
	if err := dst.QueryRow(ctx, "SELECT name FROM hosts WHERE id = 'qnap'").Scan(&name); err != nil || name != "QNAP" {
		t.Fatalf("host %q %v", name, err)
	}
	// Key files already there: refused before anything is read.
	withStdin(t, "une phrase assez longue")
	if err := restoreBackup(ctx, file, u.String(), paths); err == nil {
		t.Fatal("restored over existing keys")
	}
}
