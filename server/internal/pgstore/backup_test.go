package pgstore_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hub/server/internal/backup"
	"hub/server/internal/db/dbtest"
	"hub/server/internal/fixture"
	"hub/server/internal/pgstore"
)

func TestBackupService(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Pool(t)
	store, err := pgstore.New(ctx, pool, fixture.Empty())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	keyFile := filepath.Join(t.TempDir(), "secrets.key")
	if err := os.WriteFile(keyFile, []byte("clé"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := backup.NewService(ctx, store, pool, dir, map[string]string{backup.KeySecrets: keyFile, backup.KeyAgentCA: ""}, "test")

	st, err := svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Configured || !st.Enabled || st.Frequency != backup.Daily || st.Next != nil {
		t.Fatalf("defaults: %+v", st)
	}
	if err := svc.Start(ctx); err != backup.ErrNoKey {
		t.Fatalf("start without key: %v", err)
	}

	if err := svc.SetPassphrase(ctx, "une phrase assez longue"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if st, err = svc.Status(ctx); err != nil {
			t.Fatal(err)
		}
		if !st.Running && len(st.Runs) == 1 && st.Runs[0].FinishedAt != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("backup did not finish: %+v", st)
		}
		time.Sleep(100 * time.Millisecond)
	}
	run := st.Runs[0]
	if !run.OK || !run.Manual || run.Size == 0 || st.Kept != 1 || st.Next == nil {
		t.Fatalf("run %+v, status %+v", run, st)
	}
	f, err := os.Open(filepath.Join(dir, run.File))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	r, err := backup.Open(f, "une phrase assez longue")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Manifest.Keys) != 1 || r.Manifest.Keys[0] != backup.KeySecrets || r.Manifest.Version != "test" {
		t.Fatalf("manifest %+v", r.Manifest)
	}

	if err := svc.SaveSchedule(ctx, false, backup.Weekly, backup.KeepLast3); err != nil {
		t.Fatal(err)
	}
	if st, _ = svc.Status(ctx); st.Enabled || st.Next != nil || st.Frequency != backup.Weekly {
		t.Fatalf("disabled: %+v", st)
	}
	if err := svc.SaveSchedule(ctx, true, "monthly", backup.KeepLast3); err == nil {
		t.Fatal("unknown frequency accepted")
	}
}

func TestBackupRefusesMissingDir(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Pool(t)
	store, err := pgstore.New(ctx, pool, fixture.Empty())
	if err != nil {
		t.Fatal(err)
	}
	svc := backup.NewService(ctx, store, pool, filepath.Join(t.TempDir(), "absent"), nil, "test")
	st, err := svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.DirError == "" {
		t.Fatal("missing folder not reported")
	}
}
