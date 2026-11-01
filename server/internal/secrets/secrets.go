// Package secrets encrypts the connectors' credentials (API keys, tokens) before they reach
// the database. The keys live in a file outside the database (HUB_SECRET_KEY_FILE), so a
// database dump alone reveals nothing. Each ciphertext records the id of the key that
// sealed it, which lets a new key be added and the old rows re-encrypted (rotation).
package secrets

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"regexp"
	"runtime"
	"time"
)

// MaxSize bounds a secret's plaintext; an API key or token is far smaller.
const MaxSize = 8 << 10

var (
	ErrNotFound   = errors.New("secret not found")
	ErrUnknownKey = errors.New("secret sealed with a key missing from the key file")
	ErrDecrypt    = errors.New("secret cannot be decrypted: wrong key or altered data")
	ErrInvalid    = errors.New("invalid secret")
)

var (
	nameRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{0,127}$`)
	keyIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)
)

// ValidName reports whether name can identify a secret, such as "emby:api_key".
func ValidName(name string) bool { return nameRe.MatchString(name) }

// Keyring holds every key of the key file. The last one seals; the others only open
// what they sealed before a rotation.
type Keyring struct {
	keys    map[string]cipher.AEAD
	current string
}

// ParseKeyring reads lines of "<id> <base64 of 32 bytes>". Blank lines and lines starting
// with # are ignored. The last key is the current one.
func ParseKeyring(data []byte) (*Keyring, error) {
	kr := &Keyring{keys: map[string]cipher.AEAD{}}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		fields := bytes.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("key file line %d: want \"<id> <base64 key>\"", n)
		}
		id := string(fields[0])
		if !keyIDRe.MatchString(id) {
			return nil, fmt.Errorf("key file line %d: invalid key id %q", n, id)
		}
		if _, dup := kr.keys[id]; dup {
			return nil, fmt.Errorf("key file line %d: duplicate key id %q", n, id)
		}
		key, err := base64.StdEncoding.DecodeString(string(fields[1]))
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("key file line %d: the key must be 32 bytes in base64", n)
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		aead, err := cipher.NewGCMWithRandomNonce(block)
		if err != nil {
			return nil, err
		}
		kr.keys[id] = aead
		kr.current = id
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if kr.current == "" {
		return nil, errors.New("key file has no key")
	}
	return kr, nil
}

// LoadKeyFile reads the key file. A missing file returns an error matching fs.ErrNotExist.
func LoadKeyFile(path string) (*Keyring, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	// Windows has no mode bits to check; elsewhere, the file must stay private.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		slog.Warn("secret key file is readable by other users; restrict it with chmod 600", "path", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	kr, err := ParseKeyring(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return kr, nil
}

// CreateKeyFile writes a new key file with a single key. It never replaces an existing
// file: losing the key makes every stored secret unreadable.
func CreateKeyFile(path string) (*Keyring, error) {
	line, err := NewKeyLine(time.Now())
	if err != nil {
		return nil, err
	}
	data := []byte("# Clé de chiffrement des secrets Hub. À sauvegarder à part, jamais avec la base.\n" + line + "\n")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return ParseKeyring(data)
}

// NewKeyLine returns a fresh key-file line, named after the time it was made.
func NewKeyLine(now time.Time) (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return "k" + now.UTC().Format("20060102T150405Z") + " " + base64.StdEncoding.EncodeToString(key), nil
}

// IsNotExist reports whether err means the key file is missing.
func IsNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }

func (kr *Keyring) CurrentID() string { return kr.current }

// Seal encrypts v with the current key. The name and the key id are authenticated with
// it, so a ciphertext copied onto another row no longer opens.
func (kr *Keyring) Seal(name string, v Value) (keyID string, ciphertext []byte, err error) {
	plain := v.Reveal()
	if !ValidName(name) || plain == "" || len(plain) > MaxSize {
		return "", nil, ErrInvalid
	}
	return kr.current, kr.keys[kr.current].Seal(nil, nil, []byte(plain), aad(name, kr.current)), nil
}

func (kr *Keyring) Open(name, keyID string, ciphertext []byte) (Value, error) {
	aead, ok := kr.keys[keyID]
	if !ok {
		return Value{}, fmt.Errorf("%w (key id %q)", ErrUnknownKey, keyID)
	}
	plain, err := aead.Open(nil, nil, ciphertext, aad(name, keyID))
	if err != nil {
		return Value{}, fmt.Errorf("%w (key id %q)", ErrDecrypt, keyID)
	}
	return NewValue(string(plain)), nil
}

func aad(name, keyID string) []byte { return []byte("hub-secret\x00" + keyID + "\x00" + name) }

// Value is a secret in clear. Printing, logging or encoding it to JSON shows a mask:
// only Reveal gives the content, at the one place that sends it to the application.
// The content sits behind a pointer, so even a struct printed field by field through
// reflection (an unexported field, where fmt cannot call Format) shows an address only.
type Value struct{ p *string }

const masked = "********"

func NewValue(s string) Value { return Value{p: &s} }

func (v Value) Reveal() string {
	if v.p == nil {
		return ""
	}
	return *v.p
}

func (v Value) IsZero() bool { return v.Reveal() == "" }

func (Value) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, masked) }
func (Value) LogValue() slog.Value         { return slog.StringValue(masked) }
func (Value) MarshalText() ([]byte, error) { return []byte(masked), nil }
func (Value) MarshalJSON() ([]byte, error) { return []byte(`"` + masked + `"`), nil }

// Info describes a stored secret without its content.
type Info struct {
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}
