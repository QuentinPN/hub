package pgstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"hub/server/internal/agentproto"
	"hub/server/internal/api"
	"hub/server/internal/reconcile"
)

// querier is what both the pool and a transaction offer for reading.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// liveContainers returns the containers present, per host; every host for hostID "".
func liveContainers(ctx context.Context, q querier, hostID string) (map[string][]reconcile.Container, error) {
	rows, err := q.Query(ctx, `SELECT host_id, name, image, state, compose_project, compose_service, labels, ports
		FROM containers WHERE removed_at IS NULL AND ($1 = '' OR host_id = $1)`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byHost := map[string][]reconcile.Container{}
	for rows.Next() {
		var host string
		var c reconcile.Container
		var labels, ports []byte
		if err := rows.Scan(&host, &c.Name, &c.Image, &c.State, &c.ComposeProject, &c.ComposeService, &labels, &ports); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(labels, &c.Labels); err != nil {
			return nil, fmt.Errorf("container %s labels: %w", c.Name, err)
		}
		var list []agentproto.Port
		if err := json.Unmarshal(ports, &list); err != nil {
			return nil, fmt.Errorf("container %s ports: %w", c.Name, err)
		}
		for _, p := range list {
			if p.PublicPort != 0 {
				c.PublicPorts = append(c.PublicPorts, p.PublicPort)
			}
		}
		byHost[host] = append(byHost[host], c)
	}
	return byHost, rows.Err()
}

// syncServices brings hostID's services in line with its containers, in the transaction
// that wrote them. A service keeps its name, category, icon and favorite once created;
// what disappears is only marked, following the discovery settings.
func syncServices(ctx context.Context, tx pgx.Tx, hostID string, now time.Time) error {
	byHost, err := liveContainers(ctx, tx, hostID)
	if err != nil {
		return err
	}
	keys := []string{}
	for _, g := range reconcile.Groups(byHost[hostID]) {
		if g.Kind != reconcile.KindService {
			continue
		}
		keys = append(keys, g.Key)
		if err := upsertDiscovered(ctx, tx, hostID, g); err != nil {
			return fmt.Errorf("service %s: %w", g.Key, err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE services SET absent_since = COALESCE(absent_since, $3)
		WHERE host_id = $1 AND discovery_key IS NOT NULL AND NOT (discovery_key = ANY($2))`, hostID, keys, now); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE services s SET archived_at = $2
		FROM discovery_settings d
		WHERE s.host_id = $1 AND s.absent_since IS NOT NULL AND s.archived_at IS NULL AND (d.on_removal = 'archive_now'
			OR (d.on_removal = 'absent_then_archive' AND s.absent_since <= $2::timestamptz - make_interval(days => d.archive_after_days)))`, hostID, now)
	return err
}

// upsertDiscovered brings a known service back, or creates it under the first free slug.
func upsertDiscovered(ctx context.Context, tx pgx.Tx, hostID string, g reconcile.Group) error {
	tag, err := tx.Exec(ctx, `UPDATE services SET absent_since = NULL, archived_at = NULL
		WHERE host_id = $1 AND discovery_key = $2`, hostID, g.Key)
	if err != nil || tag.RowsAffected() > 0 {
		return err
	}
	for _, slug := range slugCandidates(g.Slug, hostID) {
		tag, err := tx.Exec(ctx, `INSERT INTO services (slug, name, monogram, icon, color, category, host_id, discovery_key, recognized_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT DO NOTHING`,
			slug, g.Name, g.Monogram, g.Icon, g.Color, g.Category, hostID, g.Key, g.RecognizedBy)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			return nil
		}
	}
	return fmt.Errorf("no free slug for %q", g.Slug)
}

// slugCandidates tries the plain slug, then the host's name: the same application on two
// hosts gives two services.
func slugCandidates(slug, hostID string) []string {
	withHost := slug + "-" + hostID
	if len(withHost) > 60 {
		withHost = strings.TrimRight(withHost[:60], "-")
	}
	list := []string{slug, withHost}
	for i := 2; i <= 5; i++ {
		list = append(list, withHost+"-"+strconv.Itoa(i))
	}
	return list
}

// discoveredDetail describes a service from its containers.
func discoveredDetail(sum api.ServiceSummary, key, recognizedBy string, g *reconcile.Group) api.ServiceDetail {
	d := api.ServiceDetail{
		ServiceSummary: sum,
		Tint:           "#1f2a36",
		Stats:          []api.Stat{},
		Upcoming:       []api.UpcomingItem{},
		Identification: []api.KeyValue{
			{Label: "Reconnu par", Value: recognizedBy},
			{Label: "Projet", Value: key},
			{Label: "Catégorie", Value: sum.Category},
		},
		Access: []api.KeyValue{
			{Label: "Hôte", Value: sum.HostID},
			{Label: "URL", Value: "à renseigner"},
		},
		Connection: []api.KeyValue{{Label: "Widget", Value: "non configuré"}},
	}
	if g == nil {
		d.Access = append(d.Access, api.KeyValue{Label: "Conteneurs", Value: "aucun présent"})
		return d
	}
	if len(g.Ports) > 0 {
		ports := make([]string, len(g.Ports))
		for i, p := range g.Ports {
			ports[i] = strconv.Itoa(p)
		}
		d.Access = append(d.Access, api.KeyValue{Label: "Ports publiés", Value: strings.Join(ports, ", ")})
	}
	for _, m := range g.Members {
		value := reconcile.StateLabel(m.State)
		switch m.Role {
		case reconcile.RoleMain:
			d.Version = imageVersion(m.Container)
			value += " · principal"
		case reconcile.RoleOneShot:
			value = "tâche ponctuelle · " + value
		}
		d.Access = append(d.Access, api.KeyValue{Label: m.Name, Value: value})
	}
	return d
}

// imageVersion prefers the image's own label; "latest" says nothing.
func imageVersion(c reconcile.Container) string {
	if v := c.Labels["org.opencontainers.image.version"]; v != "" {
		return v
	}
	image, _, _ := strings.Cut(c.Image, "@")
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") && image[i+1:] != "latest" {
		return image[i+1:]
	}
	return ""
}

// lastSeenLabel words a last contact the way the home page says it.
func lastSeenLabel(seen *time.Time, now time.Time) string {
	if seen == nil {
		return "jamais"
	}
	d := now.Sub(*seen)
	switch {
	case d < time.Minute:
		return "il y a " + strconv.Itoa(int(d.Seconds())) + " s"
	case d < time.Hour:
		return "il y a " + strconv.Itoa(int(d.Minutes())) + " min"
	case d < 24*time.Hour:
		return "il y a " + strconv.Itoa(int(d.Hours())) + " h"
	}
	return "il y a " + strconv.Itoa(int(d.Hours()/24)) + " j"
}
