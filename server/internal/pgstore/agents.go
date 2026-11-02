package pgstore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"hub/server/internal/agentproto"
	"hub/server/internal/agentsrv"
)

var _ agentsrv.Repo = (*Store)(nil)

// EnsureHost names a new host after its id; the name can be changed later.
func (s *Store) EnsureHost(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, "INSERT INTO hosts (id, name) VALUES ($1, $1) ON CONFLICT (id) DO NOTHING", id)
	return err
}

func (s *Store) CreateEnrollment(ctx context.Context, hostID string, codeHash []byte, expires time.Time) error {
	_, err := s.pool.Exec(ctx, "INSERT INTO agent_enrollments (host_id, code_hash, expires_at) VALUES ($1, $2, $3)",
		hostID, codeHash, expires)
	return err
}

// ConsumeEnrollment uses the code in one statement: two agents racing with the same code
// cannot both get it.
func (s *Store) ConsumeEnrollment(ctx context.Context, codeHash []byte, now time.Time) (string, error) {
	var hostID string
	err := s.pool.QueryRow(ctx, `UPDATE agent_enrollments SET used_at = $2
		WHERE code_hash = $1 AND used_at IS NULL AND expires_at > $2 RETURNING host_id`, codeHash, now).Scan(&hostID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", agentsrv.ErrNotFound
	}
	return hostID, err
}

func (s *Store) AddAgentCert(ctx context.Context, c agentsrv.CertRecord) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO agent_certs (serial, host_id, fingerprint, not_before, not_after)
		VALUES ($1, $2, $3, $4, $5)`, c.Serial, c.HostID, c.Fingerprint, c.NotBefore, c.NotAfter)
	return err
}

func (s *Store) AgentCert(ctx context.Context, serial string) (agentsrv.CertRecord, error) {
	c := agentsrv.CertRecord{Serial: serial}
	err := s.pool.QueryRow(ctx, `SELECT host_id, fingerprint, not_before, not_after, revoked_at
		FROM agent_certs WHERE serial = $1`, serial).Scan(&c.HostID, &c.Fingerprint, &c.NotBefore, &c.NotAfter, &c.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return agentsrv.CertRecord{}, agentsrv.ErrNotFound
	}
	return c, err
}

func (s *Store) RevokeHostCerts(ctx context.Context, hostID string, now time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx, "UPDATE agent_certs SET revoked_at = $2 WHERE host_id = $1 AND revoked_at IS NULL", hostID, now)
	return int(tag.RowsAffected()), err
}

// ReplaceContainers writes the snapshot and marks absent every container it does not list.
func (s *Store) ReplaceContainers(ctx context.Context, hostID string, snap agentproto.Snapshot, now time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE hosts SET agent_version = $2, docker_version = $3, docker_ok = true, last_seen_at = $4
			WHERE id = $1`, hostID, snap.AgentVersion, snap.DockerVersion, now); err != nil {
			return err
		}
		ids := make([]string, 0, len(snap.Containers))
		for _, c := range snap.Containers {
			if err := upsertContainer(ctx, tx, hostID, c, now); err != nil {
				return err
			}
			ids = append(ids, c.ID)
		}
		_, err := tx.Exec(ctx, `UPDATE containers SET removed_at = $3
			WHERE host_id = $1 AND removed_at IS NULL AND NOT (id = ANY($2))`, hostID, ids, now)
		return err
	})
}

func (s *Store) ApplyContainerChanges(ctx context.Context, hostID string, c agentproto.Changes, now time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, ct := range c.Upserted {
			if err := upsertContainer(ctx, tx, hostID, ct, now); err != nil {
				return err
			}
		}
		if len(c.Removed) > 0 {
			if _, err := tx.Exec(ctx, `UPDATE containers SET removed_at = $3
				WHERE host_id = $1 AND id = ANY($2) AND removed_at IS NULL`, hostID, c.Removed, now); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, "UPDATE hosts SET last_seen_at = $2 WHERE id = $1", hostID, now)
		return err
	})
}

func upsertContainer(ctx context.Context, tx pgx.Tx, hostID string, c agentproto.Container, now time.Time) error {
	labels, err := json.Marshal(c.Labels)
	if err != nil {
		return err
	}
	ports := c.Ports
	if ports == nil {
		ports = []agentproto.Port{}
	}
	portsJSON, err := json.Marshal(ports)
	if err != nil {
		return err
	}
	redacted := c.RedactedLabels
	if redacted == nil {
		redacted = []string{}
	}
	var created *time.Time
	if !c.Created.IsZero() {
		created = &c.Created
	}
	_, err = tx.Exec(ctx, `INSERT INTO containers (host_id, id, name, image, state, status, created,
			compose_project, compose_service, labels, redacted_labels, ports, first_seen_at, seen_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13)
		ON CONFLICT (host_id, id) DO UPDATE SET name = excluded.name, image = excluded.image,
			state = excluded.state, status = excluded.status, created = excluded.created,
			compose_project = excluded.compose_project, compose_service = excluded.compose_service,
			labels = excluded.labels, redacted_labels = excluded.redacted_labels, ports = excluded.ports,
			seen_at = excluded.seen_at, removed_at = NULL`,
		hostID, c.ID, c.Name, c.Image, c.State, c.Status, created,
		c.Labels[agentproto.LabelComposeProject], c.Labels[agentproto.LabelComposeService],
		labels, redacted, portsJSON, now)
	return err
}

func (s *Store) RecordHeartbeat(ctx context.Context, hostID string, h agentproto.Heartbeat, now time.Time) error {
	_, err := s.pool.Exec(ctx, "UPDATE hosts SET docker_ok = $2, last_seen_at = $3 WHERE id = $1", hostID, h.DockerOK, now)
	return err
}

func (s *Store) RescanMinutes(ctx context.Context) (int, error) {
	var m int
	err := s.pool.QueryRow(ctx, "SELECT rescan_minutes FROM discovery_settings").Scan(&m)
	return m, err
}
