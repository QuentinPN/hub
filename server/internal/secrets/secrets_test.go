package secrets_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"hub/server/internal/secrets"
)

func keyring(t *testing.T, lines ...string) *secrets.Keyring {
	t.Helper()
	kr, err := secrets.ParseKeyring([]byte(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	return kr
}

func keyLine(t *testing.T, id string) string {
	t.Helper()
	line, err := secrets.NewKeyLine(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return id + " " + strings.Fields(line)[1]
}

func TestSealOpenRoundTrip(t *testing.T) {
	kr := keyring(t, "# comment", "", keyLine(t, "k1"))
	keyID, ct, err := kr.Seal("emby:api_key", secrets.NewValue("abc123"))
	if err != nil {
		t.Fatal(err)
	}
	if keyID != "k1" || bytes.Contains(ct, []byte("abc123")) {
		t.Fatalf("keyID = %q, ciphertext holds the plaintext: %v", keyID, bytes.Contains(ct, []byte("abc123")))
	}
	v, err := kr.Open("emby:api_key", keyID, ct)
	if err != nil || v.Reveal() != "abc123" {
		t.Fatalf("Open = %q, %v", v.Reveal(), err)
	}
	// A random nonce: the same value never gives the same ciphertext twice.
	_, ct2, _ := kr.Seal("emby:api_key", secrets.NewValue("abc123"))
	if bytes.Equal(ct, ct2) {
		t.Fatal("two seals gave the same ciphertext")
	}
}

func TestOpenRejectsMovedOrAlteredData(t *testing.T) {
	kr := keyring(t, keyLine(t, "k1"))
	_, ct, _ := kr.Seal("emby:api_key", secrets.NewValue("abc123"))
	if _, err := kr.Open("jellyfin:api_key", "k1", ct); !errors.Is(err, secrets.ErrDecrypt) {
		t.Fatalf("copied onto another name: %v", err)
	}
	altered := slices.Clone(ct)
	altered[len(altered)-1] ^= 1
	if _, err := kr.Open("emby:api_key", "k1", altered); !errors.Is(err, secrets.ErrDecrypt) {
		t.Fatalf("altered: %v", err)
	}
	if _, err := kr.Open("emby:api_key", "k2", ct); !errors.Is(err, secrets.ErrUnknownKey) {
		t.Fatalf("unknown key: %v", err)
	}
	other := keyring(t, keyLine(t, "k1"))
	if _, err := other.Open("emby:api_key", "k1", ct); !errors.Is(err, secrets.ErrDecrypt) {
		t.Fatalf("other key with the same id: %v", err)
	}
}

func TestSealRejectsInvalidInput(t *testing.T) {
	kr := keyring(t, keyLine(t, "k1"))
	for name, tc := range map[string]struct {
		name string
		v    secrets.Value
	}{
		"empty value": {"emby:api_key", secrets.NewValue("")},
		"zero value":  {"emby:api_key", secrets.Value{}},
		"too large":   {"emby:api_key", secrets.NewValue(strings.Repeat("x", secrets.MaxSize+1))},
		"upper case":  {"Emby", secrets.NewValue("x")},
		"space":       {"emby key", secrets.NewValue("x")},
		"empty name":  {"", secrets.NewValue("x")},
	} {
		if _, _, err := kr.Seal(tc.name, tc.v); !errors.Is(err, secrets.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestParseKeyringErrors(t *testing.T) {
	good := keyLine(t, "k1")
	for name, data := range map[string]string{
		"empty":        "# nothing\n",
		"one field":    "k1",
		"bad id":       "k/1 " + strings.Fields(good)[1],
		"short key":    "k1 AAAA",
		"not base64":   "k1 !!!",
		"duplicate id": good + "\n" + good,
	} {
		if _, err := secrets.ParseKeyring([]byte(data)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestValueNeverPrintsItsContent(t *testing.T) {
	v := secrets.NewValue("abc123")
	type withExported struct{ Key secrets.Value }
	type withUnexported struct{ key secrets.Value }

	var logged bytes.Buffer
	slog.New(slog.NewTextHandler(&logged, nil)).Info("x", "key", v)
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("x", "key", v)
	encoded, err := json.Marshal(withExported{v})
	if err != nil {
		t.Fatal(err)
	}
	outputs := []string{
		fmt.Sprint(v), fmt.Sprintf("%s %q %v %+v %#v %x", v, v, v, v, v, v),
		fmt.Sprintf("%v %+v %#v", withExported{v}, withExported{v}, withExported{v}),
		fmt.Sprintf("%v %+v %#v", withUnexported{v}, withUnexported{v}, withUnexported{v}),
		fmt.Sprint(&v), logged.String(), string(encoded),
	}
	for _, out := range outputs {
		if strings.Contains(out, "abc123") {
			t.Errorf("secret leaked: %s", out)
		}
	}
	if v.Reveal() != "abc123" || v.IsZero() || !(secrets.Value{}).IsZero() {
		t.Fatal("Reveal or IsZero wrong")
	}
}

func TestCreateKeyFileNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.key")
	if _, err := secrets.LoadKeyFile(path); !secrets.IsNotExist(err) {
		t.Fatalf("missing file: %v", err)
	}
	kr, err := secrets.CreateKeyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secrets.CreateKeyFile(path); err == nil {
		t.Fatal("an existing key file was replaced")
	}
	loaded, err := secrets.LoadKeyFile(path)
	if err != nil || loaded.CurrentID() != kr.CurrentID() {
		t.Fatalf("reload: %v", err)
	}
	keyID, ct, _ := kr.Seal("emby:api_key", secrets.NewValue("abc123"))
	if v, err := loaded.Open("emby:api_key", keyID, ct); err != nil || v.Reveal() != "abc123" {
		t.Fatalf("reloaded key cannot open: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("stat: %v", err)
	}
}

// memRepo is an in-memory secrets.Repo.
type memRepo struct{ rows map[string]secrets.Row }

func (m *memRepo) SecretRow(_ context.Context, name string) (secrets.Row, error) {
	r, ok := m.rows[name]
	if !ok {
		return secrets.Row{}, secrets.ErrNotFound
	}
	return r, nil
}

func (m *memRepo) PutSecretRow(_ context.Context, r secrets.Row) error {
	if old, ok := m.rows[r.Name]; ok {
		r.CreatedAt = old.CreatedAt
	}
	m.rows[r.Name] = r
	return nil
}

func (m *memRepo) DeleteSecret(_ context.Context, name string) error {
	if _, ok := m.rows[name]; !ok {
		return secrets.ErrNotFound
	}
	delete(m.rows, name)
	return nil
}

func (m *memRepo) SecretInfos(context.Context) ([]secrets.Info, error) {
	var out []secrets.Info
	for _, r := range m.rows {
		out = append(out, secrets.Info{Name: r.Name, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
	}
	return out, nil
}

func (m *memRepo) SecretRowsNotSealedWith(_ context.Context, keyID string) ([]secrets.Row, error) {
	var out []secrets.Row
	for _, r := range m.rows {
		if r.KeyID != keyID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memRepo) ResealSecretRow(_ context.Context, name, oldKeyID string, r secrets.Row) (bool, error) {
	old, ok := m.rows[name]
	if !ok || old.KeyID != oldKeyID {
		return false, nil
	}
	old.KeyID, old.Ciphertext = r.KeyID, r.Ciphertext
	m.rows[name] = old
	return true, nil
}

func TestRotateReSealsWithTheNewestKey(t *testing.T) {
	ctx := context.Background()
	k1, k2 := keyLine(t, "k1"), keyLine(t, "k2")
	repo := &memRepo{rows: map[string]secrets.Row{}}
	before := secrets.NewStore(keyring(t, k1), repo)
	for _, name := range []string{"emby:api_key", "sonarr:api_key"} {
		if err := before.Put(ctx, name, secrets.NewValue("value of "+name)); err != nil {
			t.Fatal(err)
		}
	}
	// A row sealed with a key that is no longer in the file.
	lost := keyring(t, keyLine(t, "k0"))
	keyID, ct, _ := lost.Seal("radarr:api_key", secrets.NewValue("x"))
	repo.rows["radarr:api_key"] = secrets.Row{Name: "radarr:api_key", KeyID: keyID, Ciphertext: ct}

	after := secrets.NewStore(keyring(t, k1, k2), repo)
	n, err := after.Rotate(ctx)
	if n != 2 || !errors.Is(err, secrets.ErrUnknownKey) || !strings.Contains(err.Error(), "radarr:api_key") {
		t.Fatalf("Rotate = %d, %v", n, err)
	}
	for _, name := range []string{"emby:api_key", "sonarr:api_key"} {
		if repo.rows[name].KeyID != "k2" {
			t.Errorf("%s still sealed with %s", name, repo.rows[name].KeyID)
		}
		// Only the new key is needed from now on.
		v, err := secrets.NewStore(keyring(t, k2), repo).Get(ctx, name)
		if err != nil || v.Reveal() != "value of "+name {
			t.Errorf("%s after rotation: %q, %v", name, v.Reveal(), err)
		}
	}
	if n, err := after.Rotate(ctx); n != 0 || !errors.Is(err, secrets.ErrUnknownKey) {
		t.Fatalf("second Rotate = %d, %v", n, err)
	}
}
