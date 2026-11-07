// Package backup writes and restores Hub's backups: the database and the key files, in one
// file, encrypted so that only the passphrase opens it.
//
// The passphrase is never stored. Choosing it makes an age key pair: Hub keeps the public
// half, enough to encrypt at night with nobody there, and the private half sealed with the
// passphrase. Each backup carries that sealed half, so the file and the passphrase are all
// a restore needs.
//
// A file is a line of text naming the format, the sealed private key in base64 on one
// line, then the age stream of a gzipped tar: manifest.json first, keys/<role>, then
// db/<table> in an order the foreign keys accept, each in PostgreSQL's COPY text format.
package backup

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"filippo.io/age"
)

// magic is the first line of every backup file.
const magic = "hub-backup 1"

// FormatVersion is the version of the contents, in the manifest.
const FormatVersion = 1

// MinPassphraseLen is the shortest passphrase accepted.
const MinPassphraseLen = 12

// Key roles: which key file of the server each keys/ entry is.
const (
	KeySecrets = "secrets"
	KeyAgentCA = "agent-ca"
)

// Manifest describes a backup's contents.
type Manifest struct {
	Format    int       `json:"format"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	// Migration is the schema the data fits: the newest migration applied when it was made.
	Migration int      `json:"migration"`
	Tables    []Table  `json:"tables"`
	Keys      []string `json:"keys"`
}

// Table is one table of the dump, with its columns in COPY order.
type Table struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Rows    int64    `json:"rows"`
}

// ErrPassphrase is returned when the passphrase does not open a backup.
var ErrPassphrase = errors.New("phrase de passe incorrecte")

// ErrNotBackup is returned for a file that is not a Hub backup.
var ErrNotBackup = errors.New("ce fichier n'est pas une sauvegarde de Hub")

// Key is what Hub keeps to write backups: the public half, and the private half sealed with
// the passphrase.
type Key struct {
	Recipient string
	Sealed    []byte
}

// NewKey makes the key pair for a passphrase.
func NewKey(passphrase string) (Key, error) {
	if len([]rune(passphrase)) < MinPassphraseLen {
		return Key{}, fmt.Errorf("phrase de passe trop courte : %d caractères au moins", MinPassphraseLen)
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return Key{}, err
	}
	r, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return Key{}, err
	}
	var sealed bytes.Buffer
	w, err := age.Encrypt(&sealed, r)
	if err != nil {
		return Key{}, err
	}
	if _, err := io.WriteString(w, id.String()); err != nil {
		return Key{}, err
	}
	if err := w.Close(); err != nil {
		return Key{}, err
	}
	return Key{Recipient: id.Recipient().String(), Sealed: sealed.Bytes()}, nil
}

// Writer builds a backup's contents. Call Manifest, the keys, then the tables, then Close.
type Writer struct {
	enc io.WriteCloser
	gz  *gzip.Writer
	tw  *tar.Writer
	at  time.Time
}

// NewWriter starts a backup on w, encrypted for key.
func NewWriter(w io.Writer, key Key, at time.Time) (*Writer, error) {
	r, err := age.ParseX25519Recipient(key.Recipient)
	if err != nil {
		return nil, fmt.Errorf("clé de sauvegarde : %w", err)
	}
	if _, err := fmt.Fprintf(w, "%s\n%s\n", magic, base64.StdEncoding.EncodeToString(key.Sealed)); err != nil {
		return nil, err
	}
	enc, err := age.Encrypt(w, r)
	if err != nil {
		return nil, err
	}
	gz := gzip.NewWriter(enc)
	return &Writer{enc: enc, gz: gz, tw: tar.NewWriter(gz), at: at}, nil
}

func (w *Writer) entry(name string, body []byte) error {
	if err := w.tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), ModTime: w.at, Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	_, err := w.tw.Write(body)
	return err
}

// Manifest writes the manifest: it must come first, a restore reads it before anything else.
func (w *Writer) Manifest(m Manifest) error {
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return w.entry("manifest.json", body)
}

// Key writes one key file.
func (w *Writer) Key(role string, body []byte) error { return w.entry("keys/"+role, body) }

// Table writes one table's rows.
func (w *Writer) Table(name string, body []byte) error { return w.entry("db/"+name, body) }

// Close finishes the tar, the compression and the encryption, in that order.
func (w *Writer) Close() error {
	return errors.Join(w.tw.Close(), w.gz.Close(), w.enc.Close())
}

// Reader reads a backup's contents in the order they were written.
type Reader struct {
	Manifest Manifest
	tr       *tar.Reader
}

// Peek checks a file starts like a backup, without the passphrase: what a check after
// writing can do, since Hub never keeps the private key.
func Peek(r io.Reader) error {
	br := bufio.NewReader(r)
	if _, err := header(br); err != nil {
		return err
	}
	// age's own header line, then the rest of the stream.
	line, err := br.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "age-encryption.org/") {
		return ErrNotBackup
	}
	_, err = io.Copy(io.Discard, br)
	return err
}

func header(br *bufio.Reader) ([]byte, error) {
	first, err := br.ReadString('\n')
	if err != nil || strings.TrimRight(first, "\n") != magic {
		return nil, ErrNotBackup
	}
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, ErrNotBackup
	}
	sealed, err := base64.StdEncoding.DecodeString(strings.TrimRight(line, "\n"))
	if err != nil {
		return nil, ErrNotBackup
	}
	return sealed, nil
}

// Open decrypts a backup with its passphrase and reads its manifest.
func Open(r io.Reader, passphrase string) (*Reader, error) {
	br := bufio.NewReader(r)
	sealed, err := header(br)
	if err != nil {
		return nil, err
	}
	sid, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, err
	}
	plain, err := age.Decrypt(bytes.NewReader(sealed), sid)
	if err != nil {
		var wrong *age.NoIdentityMatchError
		if errors.As(err, &wrong) {
			return nil, ErrPassphrase
		}
		return nil, fmt.Errorf("clé de la sauvegarde : %w", err)
	}
	text, err := io.ReadAll(plain)
	if err != nil {
		return nil, err
	}
	id, err := age.ParseX25519Identity(string(text))
	if err != nil {
		return nil, fmt.Errorf("clé de la sauvegarde : %w", err)
	}
	data, err := age.Decrypt(br, id)
	if err != nil {
		return nil, fmt.Errorf("déchiffrement : %w", err)
	}
	gz, err := gzip.NewReader(data)
	if err != nil {
		return nil, fmt.Errorf("décompression : %w", err)
	}
	out := &Reader{tr: tar.NewReader(gz)}
	name, body, err := out.Next()
	if err != nil || name != "manifest.json" {
		return nil, fmt.Errorf("manifeste absent : %w", errors.Join(err, ErrNotBackup))
	}
	if err := json.Unmarshal(body, &out.Manifest); err != nil {
		return nil, fmt.Errorf("manifeste illisible : %w", err)
	}
	if out.Manifest.Format != FormatVersion {
		return nil, fmt.Errorf("format %d inconnu de cette version de Hub", out.Manifest.Format)
	}
	return out, nil
}

// maxEntry bounds one entry read in memory: Hub's tables and keys are small.
const maxEntry = 512 << 20

// Next returns the next entry, io.EOF after the last.
func (r *Reader) Next() (string, []byte, error) {
	h, err := r.tr.Next()
	if err != nil {
		return "", nil, err
	}
	if h.Size > maxEntry {
		return "", nil, fmt.Errorf("%s : trop grand", h.Name)
	}
	body, err := io.ReadAll(io.LimitReader(r.tr, maxEntry))
	return h.Name, body, err
}
