package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"hub/server/internal/auth"
	"hub/server/internal/backup"
)

// Backups runs the backups; *backup.Service is the real one.
type Backups interface {
	Status(ctx context.Context) (backup.Status, error)
	SaveSchedule(ctx context.Context, enabled bool, frequency, retention string) error
	SetPassphrase(ctx context.Context, passphrase string) error
	Start(ctx context.Context) error
}

func registerBackups(mux *http.ServeMux, b Backups, authSvc *auth.Service) {
	audit := func(r *http.Request, detail string) {
		p, _ := current(r.Context())
		authSvc.Audit(r.Context(), auth.Event{Kind: "settings_updated", UserID: &p.user.ID, IP: clientOf(r).IP, Detail: detail})
	}
	mux.HandleFunc("GET /api/v1/backup", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(b.Status(r.Context()))
	})
	mux.HandleFunc("PUT /api/v1/backup/schedule", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled   bool   `json:"enabled"`
			Frequency string `json:"frequency"`
			Retention string `json:"retention"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if err := b.SaveSchedule(r.Context(), body.Enabled, body.Frequency, body.Retention); errors.Is(err, backup.ErrOff) {
			writeError(w, http.StatusConflict, err.Error())
			return
		} else if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		audit(r, "backup_schedule")
		respond(w, r)(b.Status(r.Context()))
	})
	// The passphrase only passes through: it seals a new key, then is forgotten.
	mux.HandleFunc("PUT /api/v1/backup/passphrase", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Passphrase string `json:"passphrase"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if len([]rune(body.Passphrase)) < backup.MinPassphraseLen {
			writeError(w, http.StatusBadRequest, "phrase de passe trop courte")
			return
		}
		if len(body.Passphrase) > 1024 {
			writeError(w, http.StatusBadRequest, "phrase de passe trop longue")
			return
		}
		if err := b.SetPassphrase(r.Context(), body.Passphrase); errors.Is(err, backup.ErrOff) {
			writeError(w, http.StatusConflict, err.Error())
			return
		} else if err != nil {
			internalError(w, r, err)
			return
		}
		audit(r, "backup_passphrase")
		respond(w, r)(b.Status(r.Context()))
	})
	mux.HandleFunc("POST /api/v1/backup/run", func(w http.ResponseWriter, r *http.Request) {
		err := b.Start(r.Context())
		switch {
		case errors.Is(err, backup.ErrRunning):
			writeError(w, http.StatusConflict, err.Error())
			return
		case errors.Is(err, backup.ErrNoKey), errors.Is(err, backup.ErrOff):
			writeError(w, http.StatusConflict, err.Error())
			return
		case err != nil:
			internalError(w, r, err)
			return
		}
		audit(r, "backup_run")
		st, err := b.Status(r.Context())
		if err != nil {
			internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, st)
	})
}

// backupCheck is the « Sauvegarde » line of « État de Hub ».
func backupCheck(ctx context.Context, b Backups, now time.Time) StatusCheck {
	c := StatusCheck{ID: "backup", Name: "Sauvegarde", Level: LevelOK, Link: "backup"}
	st, err := b.Status(ctx)
	if err != nil {
		c.Level, c.Detail = LevelBad, "État illisible"
		return c
	}
	var last, lastOK *backup.Run
	for i := range st.Runs {
		r := &st.Runs[i]
		if r.FinishedAt == nil && st.Running && i == 0 {
			continue
		}
		if last == nil {
			last = r
		}
		if r.OK {
			lastOK = r
			break
		}
	}
	// Late once a slot has been missed: two days for daily, eight for weekly.
	limit := 50 * time.Hour
	if st.Frequency == backup.Weekly {
		limit = 8*24*time.Hour + 2*time.Hour
	}
	switch {
	// Chosen when deploying: said, not counted as a problem.
	case st.Off:
		c.Detail = "Désactivées sur ce serveur (HUB_BACKUP=off)"
	case !st.Configured:
		c.Level, c.Detail = LevelWarn, "Pas configurée : choisir une phrase de passe"
	case st.DirError != "":
		c.Level, c.Detail = LevelBad, st.DirError
	case last != nil && !last.OK:
		c.Level, c.Since = LevelBad, &last.StartedAt
		c.Detail = "Dernière en échec"
		if last.Error != "" {
			c.Detail += " : " + last.Error
		}
	case !st.Enabled:
		c.Level, c.Detail = LevelWarn, "Automatique désactivée"
		if lastOK != nil {
			c.Since = lastOK.FinishedAt
		}
	case lastOK == nil:
		c.Level, c.Detail = LevelWarn, "Aucune sauvegarde encore"
	case now.Sub(lastOK.StartedAt) > limit:
		c.Level, c.Detail, c.Since = LevelBad, "Aucune réussie depuis plus d'un créneau", lastOK.FinishedAt
	default:
		c.Detail, c.Since = plural(st.Kept, "sauvegarde gardée", "sauvegardes gardées"), lastOK.FinishedAt
	}
	return c
}
