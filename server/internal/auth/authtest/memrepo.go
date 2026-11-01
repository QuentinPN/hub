// Package authtest provides an in-memory auth.Repo for tests that do not need PostgreSQL.
package authtest

import (
	"context"
	"sort"
	"sync"
	"time"

	"hub/server/internal/auth"
)

type Repo struct {
	mu       sync.Mutex
	user     *auth.User
	hash     string
	sessions map[string]auth.Session // key: token hash
	nextID   int64
	Events   []auth.Event
}

func New() *Repo { return &Repo{sessions: map[string]auth.Session{}} }

func (r *Repo) HasUser(context.Context) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.user != nil, nil
}

func (r *Repo) CreateUser(_ context.Context, username, hash string) (auth.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.user != nil {
		return auth.User{}, auth.ErrSetupDone
	}
	r.user = &auth.User{ID: 1, Username: username}
	r.hash = hash
	return *r.user, nil
}

func (r *Repo) UserByName(_ context.Context, username string) (auth.User, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.user == nil || r.user.Username != username {
		return auth.User{}, "", auth.ErrNotFound
	}
	return *r.user, r.hash, nil
}

func (r *Repo) CreateSession(_ context.Context, tokenHash []byte, s auth.Session) (auth.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	s.ID = r.nextID
	r.sessions[string(tokenHash)] = s
	return s, nil
}

func (r *Repo) SessionByToken(_ context.Context, tokenHash []byte, now time.Time) (auth.Session, auth.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[string(tokenHash)]
	if !ok || !now.Before(s.ExpiresAt) || r.user == nil {
		return auth.Session{}, auth.User{}, auth.ErrNotFound
	}
	return s, *r.user, nil
}

func (r *Repo) TouchSession(_ context.Context, id int64, lastSeen, expires time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, s := range r.sessions {
		if s.ID == id {
			s.LastSeenAt, s.ExpiresAt = lastSeen, expires
			r.sessions[k] = s
		}
	}
	return nil
}

func (r *Repo) DeleteSession(_ context.Context, userID, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, s := range r.sessions {
		if s.ID == id && s.UserID == userID {
			delete(r.sessions, k)
			return nil
		}
	}
	return auth.ErrNotFound
}

func (r *Repo) Sessions(_ context.Context, userID int64, now time.Time) ([]auth.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []auth.Session{}
	for _, s := range r.sessions {
		if s.UserID == userID && now.Before(s.ExpiresAt) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *Repo) Audit(_ context.Context, e auth.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Events = append(r.Events, e)
	return nil
}

// Kinds lists the recorded event kinds, in order.
func (r *Repo) Kinds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.Events))
	for i, e := range r.Events {
		out[i] = e.Kind
	}
	return out
}
