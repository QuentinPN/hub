package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"hub/server/internal/agentsrv"
)

// printEnrollment prints what a new agent needs: the one-time code (stdout, so it can be
// piped into a file) and the CA pin. Like the setup code, it requires a hand on the server.
func printEnrollment(ctx context.Context, agents *agentsrv.Service, hostID string) error {
	e, err := agents.CreateEnrollment(ctx, hostID, "", agentsrv.CommandLine)
	if err != nil {
		return err
	}
	fmt.Println(e.Code)
	fmt.Fprintf(os.Stderr, "\nEnrollment code for %q above: once only, until %s.\n",
		hostID, e.ExpiresAt.Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "On the host, put it in the file HUB_ENROLL_CODE_FILE points to, and set\n  HUB_CA_PIN=%s\n", agents.Pin())
	return nil
}

func revokeAgent(ctx context.Context, agents *agentsrv.Service, hostID string) error {
	n, err := agents.Revoke(ctx, hostID, agentsrv.CommandLine)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%d certificate(s) of %q revoked. Its agent is refused from its next request; enroll it again to reconnect.\n", n, hostID)
	return nil
}
