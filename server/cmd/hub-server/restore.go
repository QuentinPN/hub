package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"hub/server/internal/backup"
	"hub/server/internal/db"
)

// readPassphrase reads the backup passphrase on the terminal without echo, or as one line
// from a pipe. Never from the environment or the command line, where it would linger.
func readPassphrase() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprint(os.Stderr, "Phrase de passe de la sauvegarde : ")
	p, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	return string(p), err
}

func openBackup(path string) (*backup.Reader, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	pass, err := readPassphrase()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	r, err := backup.Open(bufio.NewReader(f), pass)
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return r, func() { _ = f.Close() }, nil
}

// verifyBackup opens a backup and reads it to the end, writing nothing: proof that the file
// and the passphrase restore, without stopping Hub.
func verifyBackup(path string) error {
	r, done, err := openBackup(path)
	if err != nil {
		return err
	}
	defer done()
	for {
		_, _, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("lecture : %w", err)
		}
	}
	var rows int64
	for _, t := range r.Manifest.Tables {
		rows += t.Rows
	}
	fmt.Fprintf(os.Stderr, "Sauvegarde lisible : faite le %s par Hub %s, %d tables, %d lignes, clés : %s.\n",
		r.Manifest.CreatedAt.Local().Format("02/01/2006 15:04"), r.Manifest.Version, len(r.Manifest.Tables), rows, strings.Join(r.Manifest.Keys, ", "))
	return nil
}

// restoreBackup loads a backup into an empty database and writes its key files where this
// server reads them. It refuses a database in use and a key file already there.
func restoreBackup(ctx context.Context, path, databaseURL string, keyFiles map[string]string) error {
	if databaseURL == "" {
		return errors.New("HUB_DATABASE_URL is required")
	}
	for role, p := range keyFiles {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("%s existe déjà (%s) : restaurer sur des volumes neufs", p, role)
		}
	}
	r, done, err := openBackup(path)
	if err != nil {
		return err
	}
	defer done()
	pool, err := db.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	var written []string
	err = backup.Restore(ctx, pool, r, func(role string, body []byte) error {
		p := keyFiles[role]
		if p == "" {
			fmt.Fprintf(os.Stderr, "Clé %s ignorée : aucun chemin configuré sur ce serveur.\n", role)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, err = f.Write(body)
		if err = errors.Join(err, f.Sync(), f.Close()); err != nil {
			return err
		}
		written = append(written, p)
		return nil
	})
	if err != nil {
		// Keys without their database are of no use, and would block the next try.
		for _, p := range written {
			_ = os.Remove(p)
		}
		return err
	}
	var rows int64
	for _, t := range r.Manifest.Tables {
		rows += t.Rows
	}
	fmt.Fprintf(os.Stderr, "Restauré : sauvegarde du %s, %d lignes, %d fichiers de clés. Démarrer Hub normalement.\n",
		r.Manifest.CreatedAt.Local().Format("02/01/2006 15:04"), rows, len(written))
	return nil
}
