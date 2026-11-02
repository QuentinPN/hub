// Package agentproto is what an agent sends to the server, shared by both sides. The
// agent filters before sending; the server filters again, so an old or altered agent
// cannot fill the database with secrets (AGENT-STUDY.md, 1.f).
package agentproto

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Paths of the agent API, served on the agent listener only.
const (
	PathEnroll     = "/agent/v1/enroll"
	PathRenew      = "/agent/v1/renew"
	PathSnapshot   = "/agent/v1/snapshot"
	PathContainers = "/agent/v1/containers"
	PathHeartbeat  = "/agent/v1/heartbeat"
)

// Limits on what the server accepts.
const (
	MaxBody       = 2 << 20
	MaxContainers = 1000
	MaxLabels     = 200
	maxLabelKey   = 256
	maxLabelValue = 1024
	maxText       = 512
	maxPorts      = 100
)

// EnrollRequest trades a one-time code for a certificate. CSR is DER.
type EnrollRequest struct {
	Code     string `json:"code"`
	CSR      []byte `json:"csr"`
	Hostname string `json:"hostname"`
}

// RenewRequest asks for a new certificate for a new key, over the current mTLS connection.
type RenewRequest struct {
	CSR []byte `json:"csr"`
}

// CertResponse carries the new certificate (DER).
type CertResponse struct {
	Cert []byte `json:"cert"`
}

// Snapshot is the full list of a host's containers. Every container missing from it is
// marked absent on the server.
type Snapshot struct {
	AgentVersion  string      `json:"agent_version"`
	DockerVersion string      `json:"docker_version"`
	DockerAPI     string      `json:"docker_api"`
	Containers    []Container `json:"containers"`
}

// Changes is what events bring between two snapshots.
type Changes struct {
	Upserted []Container `json:"upserted"`
	Removed  []string    `json:"removed"`
}

// Heartbeat tells the server the agent is alive and whether Docker answers.
type Heartbeat struct {
	DockerOK bool   `json:"docker_ok"`
	Error    string `json:"error,omitempty"`
}

// HeartbeatResponse gives the agent the rescan interval from the discovery settings.
type HeartbeatResponse struct {
	RescanSeconds int `json:"rescan_seconds"`
}

// Container is the part of Docker's container summary the server keeps. Never the
// command line or the environment.
type Container struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Image   string    `json:"image"`
	State   string    `json:"state"`
	Status  string    `json:"status"`
	Created time.Time `json:"created"`
	// Labels holds the values of allowed labels; RedactedLabels the keys of the others.
	Labels         map[string]string `json:"labels"`
	RedactedLabels []string          `json:"redacted_labels"`
	Ports          []Port            `json:"ports"`
}

type Port struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort int    `json:"private"`
	PublicPort  int    `json:"public,omitempty"`
	Type        string `json:"type"`
}

// Compose labels, read by the server to group containers.
const (
	LabelComposeProject = "com.docker.compose.project"
	LabelComposeService = "com.docker.compose.service"
)

// allowedPrefixes are the labels whose values may be kept (DECISIONS.md, « Keep defaults »).
var allowedPrefixes = []string{"com.docker.compose.", "org.opencontainers.image.", "hub.", "homepage."}

// sensitiveRe catches key-like fields even under an allowed prefix, e.g.
// homepage.widget.key or homepage.widget.password.
var sensitiveRe = regexp.MustCompile(`(?i)(pass|secret|token|key|auth|cred|users|cookie|session|private)`)

// FilterLabels splits Docker labels into the values that may be kept and the keys whose
// value is dropped.
func FilterLabels(in map[string]string) (kept map[string]string, redacted []string) {
	kept = map[string]string{}
	for k, v := range in {
		if LabelAllowed(k) {
			kept[k] = v
		} else {
			redacted = append(redacted, k)
		}
	}
	sort.Strings(redacted)
	return kept, redacted
}

// LabelAllowed reports whether a label's value may leave the host.
func LabelAllowed(key string) bool {
	for _, p := range allowedPrefixes {
		if strings.HasPrefix(key, p) {
			return !sensitiveRe.MatchString(key[len(p):])
		}
	}
	return false
}

var (
	containerIDRe = regexp.MustCompile(`^[0-9a-f]{64}$`)
	ErrInvalid    = errors.New("invalid agent payload")
)

// ValidContainerID reports whether id is a full Docker container id.
func ValidContainerID(id string) bool { return containerIDRe.MatchString(id) }

// Sanitize checks c against the limits and filters its labels again. It never trusts the
// agent's own split: a value under RedactedLabels is not possible, and a kept label that
// is not allowed moves to RedactedLabels.
func (c *Container) Sanitize() error {
	if !ValidContainerID(c.ID) {
		return fmt.Errorf("%w: container id %q", ErrInvalid, c.ID)
	}
	for _, s := range []string{c.Name, c.Image, c.State, c.Status} {
		if len(s) > maxText {
			return fmt.Errorf("%w: text over %d bytes", ErrInvalid, maxText)
		}
	}
	if len(c.Labels)+len(c.RedactedLabels) > MaxLabels || len(c.Ports) > maxPorts {
		return fmt.Errorf("%w: too many labels or ports", ErrInvalid)
	}
	kept, redacted := FilterLabels(c.Labels)
	seen := map[string]bool{}
	for _, k := range append(redacted, c.RedactedLabels...) {
		if len(k) > maxLabelKey {
			return fmt.Errorf("%w: label key over %d bytes", ErrInvalid, maxLabelKey)
		}
		if _, ok := kept[k]; !ok {
			seen[k] = true
		}
	}
	for k, v := range kept {
		if len(k) > maxLabelKey || len(v) > maxLabelValue {
			return fmt.Errorf("%w: label over the limits", ErrInvalid)
		}
	}
	c.Labels = kept
	c.RedactedLabels = c.RedactedLabels[:0]
	for k := range seen {
		c.RedactedLabels = append(c.RedactedLabels, k)
	}
	sort.Strings(c.RedactedLabels)
	for _, p := range c.Ports {
		if p.PrivatePort < 0 || p.PrivatePort > 65535 || p.PublicPort < 0 || p.PublicPort > 65535 || len(p.IP) > 64 || len(p.Type) > 8 {
			return fmt.Errorf("%w: port", ErrInvalid)
		}
	}
	return nil
}
