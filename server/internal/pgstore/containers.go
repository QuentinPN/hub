package pgstore

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"hub/server/internal/agentproto"
	"hub/server/internal/api"
	"hub/server/internal/reconcile"
)

// maxRemoved caps the containers gone from the host that the tab lists: each recreation
// leaves one behind.
const maxRemoved = 20

var _ api.ContainerReader = (*Store)(nil)

// ServiceContainers lists the containers of slug's projects, present ones in project order
// then by name, removed ones newest first. Labels are counted as in HostContainers.
func (s *Store) ServiceContainers(ctx context.Context, slug string, now time.Time) (api.ServiceContainers, error) {
	scope, err := s.ServiceScope(ctx, slug)
	if err != nil {
		return api.ServiceContainers{}, err
	}
	res := api.ServiceContainers{HostID: scope.HostID, Containers: []api.ServiceContainer{}, Removed: []api.ServiceContainer{}}
	var f api.AgentFacts
	if err := s.pool.QueryRow(ctx, `SELECT h.name, `+agentFactsColumns+` FROM hosts h WHERE h.id = $2`, now, scope.HostID).
		Scan(&res.HostName, &f.DockerOK, &f.LastSeenAt, &f.CertExpiresAt, &f.EnrollmentExpiresAt, &f.EverIssued, &f.EverRevoked); err != nil {
		return api.ServiceContainers{}, err
	}
	res.HostState, res.LastSeenAt = f.State(now), f.LastSeenAt
	if res.Split, err = s.splitInfo(ctx, scope.HostID, scope.Keys[0]); err != nil {
		return api.ServiceContainers{}, err
	}

	roles, err := s.roles(ctx, scope)
	if err != nil {
		return api.ServiceContainers{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT left(id, 12), name, image, state, status, compose_project, compose_service,
			`+containerKey+`, ports,
			(SELECT count(*) FROM jsonb_object_keys(labels)) + cardinality(redacted_labels), cardinality(redacted_labels),
			first_seen_at, removed_at
		FROM containers WHERE host_id = $1 AND `+containerKey+` = ANY($2)
		ORDER BY removed_at IS NOT NULL, removed_at DESC, array_position($2, `+containerKey+`), name`, scope.HostID, scope.Keys)
	if err != nil {
		return api.ServiceContainers{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var c api.ServiceContainer
		var ports []byte
		if err := rows.Scan(&c.ID, &c.Name, &c.Image, &c.State, &c.Status, &c.ComposeProject, &c.ComposeService,
			&c.Project, &ports, &c.Labels, &c.MaskedLabels, &c.FirstSeenAt, &c.RemovedAt); err != nil {
			return api.ServiceContainers{}, err
		}
		if c.Ports, err = publishedPorts(ports); err != nil {
			return api.ServiceContainers{}, fmt.Errorf("container %s ports: %w", c.Name, err)
		}
		c.StateLabel, c.Health = reconcile.StateLabel(c.State), healthOf(c.Status)
		c.Linked = c.Project != scope.Keys[0]
		if c.RemovedAt != nil {
			res.RemovedTotal++
			if len(res.Removed) < maxRemoved {
				res.Removed = append(res.Removed, c)
			}
			continue
		}
		c.Role = string(roles[c.Name])
		res.Containers = append(res.Containers, c)
	}
	return res, rows.Err()
}

// roles gives each present container its role as the service sees it: a linked project's
// main container only supports the service, as reconcile.Group.Link has it.
func (s *Store) roles(ctx context.Context, scope ServiceScope) (map[string]reconcile.Role, error) {
	byHost, err := liveContainers(ctx, s.pool, scope.HostID)
	if err != nil {
		return nil, err
	}
	roles := map[string]reconcile.Role{}
	for _, g := range reconcile.Groups(byHost[scope.HostID]) {
		i := slices.Index(scope.Keys, g.Key)
		if i < 0 {
			continue
		}
		for _, m := range g.Members {
			if i > 0 && m.Role == reconcile.RoleMain {
				m.Role = reconcile.RoleSupport
			}
			roles[m.Name] = m.Role
		}
	}
	return roles, nil
}

// publishedPorts keeps the ports published on the host, once each: Docker lists a port
// bound on IPv4 and IPv6 twice.
func publishedPorts(raw []byte) ([]api.PublishedPort, error) {
	var list []agentproto.Port
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	ports := []api.PublishedPort{}
	for _, p := range list {
		if p.PublicPort == 0 {
			continue
		}
		pp := api.PublishedPort{IP: p.IP, Public: p.PublicPort, Private: p.PrivatePort, Type: p.Type}
		if i := slices.IndexFunc(ports, func(o api.PublishedPort) bool {
			return o.Public == pp.Public && o.Private == pp.Private && o.Type == pp.Type
		}); i >= 0 {
			if pp.IP == "0.0.0.0" {
				ports[i].IP = pp.IP
			}
			continue
		}
		ports = append(ports, pp)
	}
	slices.SortFunc(ports, func(a, b api.PublishedPort) int { return a.Public - b.Public })
	return ports, nil
}

// healthOf reads the healthcheck Docker appends to the status: "Up 3 days (healthy)",
// "Up 5 seconds (health: starting)", "Up 2 minutes (unhealthy)".
func healthOf(status string) string {
	switch {
	case strings.HasSuffix(status, "(healthy)"):
		return "healthy"
	case strings.HasSuffix(status, "(unhealthy)"):
		return "unhealthy"
	case strings.HasSuffix(status, "(health: starting)"):
		return "starting"
	}
	return ""
}
