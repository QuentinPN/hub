package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"hub/server/internal/auth"
)

// resetPassword sets a new password for the account, typed on the terminal without echo
// (or read as one line from a pipe). It runs next to the live server, on the same
// database: sessions end at once, passkeys stay.
func resetPassword(ctx context.Context, svc *auth.Service) error {
	password, err := readNewPassword()
	if err != nil {
		return err
	}
	user, err := svc.ResetPassword(ctx, password)
	if errors.Is(err, auth.ErrNotFound) {
		return errors.New("no account yet: start the server and create it with the setup code")
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Password changed for %q. Every session has ended; passkeys still work.\n", user.Username)
	return nil
}

func readNewPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprintf(os.Stderr, "New password (%d characters minimum): ", auth.MinPasswordLen)
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Again: ")
	second, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(first) != string(second) {
		return "", errors.New("the two passwords differ")
	}
	return string(first), nil
}
