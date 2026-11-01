package secrets

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Row is a secret as the database keeps it: sealed, never in clear.
type Row struct {
	Name       string
	KeyID      string
	Ciphertext []byte
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Repo stores sealed secrets. SecretRow and DeleteSecret return ErrNotFound for an unknown
// name.
type Repo interface {
	SecretRow(ctx context.Context, name string) (Row, error)
	// PutSecretRow creates the secret or replaces its content, keeping its creation date.
	PutSecretRow(ctx context.Context, r Row) error
	DeleteSecret(ctx context.Context, name string) error
	SecretInfos(ctx context.Context) ([]Info, error)
	// SecretRowsNotSealedWith lists the rows sealed with any key other than keyID.
	SecretRowsNotSealedWith(ctx context.Context, keyID string) ([]Row, error)
	// ResealSecretRow replaces the ciphertext only if the row still carries oldKeyID, so a
	// value written meanwhile is never overwritten by an older one. It reports whether it did.
	ResealSecretRow(ctx context.Context, name, oldKeyID string, r Row) (bool, error)
}

// Store seals secrets before they reach the repository and opens them on the way back.
type Store struct {
	kr   *Keyring
	repo Repo
	now  func() time.Time
}

func NewStore(kr *Keyring, repo Repo) *Store {
	return &Store{kr: kr, repo: repo, now: time.Now}
}

func (s *Store) Put(ctx context.Context, name string, v Value) error {
	keyID, ct, err := s.kr.Seal(name, v)
	if err != nil {
		return err
	}
	now := s.now()
	return s.repo.PutSecretRow(ctx, Row{Name: name, KeyID: keyID, Ciphertext: ct, CreatedAt: now, UpdatedAt: now})
}

func (s *Store) Get(ctx context.Context, name string) (Value, error) {
	r, err := s.repo.SecretRow(ctx, name)
	if err != nil {
		return Value{}, err
	}
	return s.kr.Open(r.Name, r.KeyID, r.Ciphertext)
}

func (s *Store) Delete(ctx context.Context, name string) error { return s.repo.DeleteSecret(ctx, name) }

func (s *Store) List(ctx context.Context) ([]Info, error) { return s.repo.SecretInfos(ctx) }

// Rotate re-seals with the current key every secret sealed with an older one. A secret
// that cannot be opened (its key left the file, or its data was altered) is left as it is
// and reported in the returned error; the others are still re-sealed.
func (s *Store) Rotate(ctx context.Context) (resealed int, err error) {
	rows, err := s.repo.SecretRowsNotSealedWith(ctx, s.kr.CurrentID())
	if err != nil {
		return 0, err
	}
	var failed []error
	for _, r := range rows {
		v, err := s.kr.Open(r.Name, r.KeyID, r.Ciphertext)
		if err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", r.Name, err))
			continue
		}
		keyID, ct, err := s.kr.Seal(r.Name, v)
		if err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", r.Name, err))
			continue
		}
		ok, err := s.repo.ResealSecretRow(ctx, r.Name, r.KeyID, Row{Name: r.Name, KeyID: keyID, Ciphertext: ct})
		if err != nil {
			return resealed, err
		}
		if ok {
			resealed++
		}
	}
	return resealed, errors.Join(failed...)
}
