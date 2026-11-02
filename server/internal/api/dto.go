package api

import (
	"time"

	"hub/server/internal/domain"
)

// DTOs sent to the browser. They mirror web/src/lib/api/types.ts and must never carry a
// secret: credentials only ever appear as an already-masked string.

type ServiceSummary struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Monogram string `json:"monogram"`
	// Icon is a selfh.st icon reference; nil means the UI shows the monogram.
	Icon      *string       `json:"icon"`
	Color     string        `json:"color"`
	Category  string        `json:"category"`
	HostID    string        `json:"hostId"`
	Status    domain.Status `json:"status"`
	LatencyMs *int          `json:"latencyMs"`
	Note      string        `json:"note"`
	Favorite  bool          `json:"favorite"`
}

type Session struct {
	Title         string  `json:"title"`
	Subtitle      string  `json:"subtitle"`
	Device        string  `json:"device"`
	Progress      float64 `json:"progress"`
	DurationLabel string  `json:"durationLabel"`
	Transcoding   bool    `json:"transcoding"`
}

type UpcomingItem struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	When   string `json:"when"`
	Kind   string `json:"kind"`
	Tone   string `json:"tone"`
}

type HostMetric struct {
	Label   string `json:"label"`
	Percent int    `json:"percent"`
}

type Host struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	OS            string       `json:"os"`
	Containers    int          `json:"containers"`
	Reachable     bool         `json:"reachable"`
	LastSeenLabel string       `json:"lastSeenLabel"`
	Metrics       []HostMetric `json:"metrics"`
}

type Summary struct {
	Online int `json:"online"`
	Total  int `json:"total"`
}

type Home struct {
	Summary   Summary          `json:"summary"`
	Sessions  []Session        `json:"sessions"`
	Favorites []ServiceSummary `json:"favorites"`
	Upcoming  []UpcomingItem   `json:"upcoming"`
	Hosts     []Host           `json:"hosts"`
}

type KeyValue struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Mono  bool   `json:"mono,omitempty"`
}

type Stat struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// AgentHost is a host and its agent as the settings show them. Times are absolute: the UI
// writes « il y a 8 s » itself and keeps it current.
type AgentHost struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	OS                  string     `json:"os"`
	Arch                string     `json:"arch"`
	State               AgentState `json:"state"`
	AgentVersion        string     `json:"agentVersion"`
	DockerVersion       string     `json:"dockerVersion"`
	LastSeenAt          *time.Time `json:"lastSeenAt"`
	CertExpiresAt       *time.Time `json:"certExpiresAt"`
	EnrollmentExpiresAt *time.Time `json:"enrollmentExpiresAt"`
	// Containers counts those present now; absent ones stay in the database.
	Containers int `json:"containers"`
}

type HostsResponse struct {
	// AgentsEnabled is false while the agent listener is off (no HUB_AGENT_CA_FILE).
	AgentsEnabled bool        `json:"agentsEnabled"`
	ServerURL     string      `json:"serverUrl"`
	Pin           string      `json:"pin"`
	Hosts         []AgentHost `json:"hosts"`
}

// Container is what an agent reported, without label values: a count of labels and of
// those masked on the host.
type Container struct {
	ID             string `json:"id"` // the 12 characters Docker shows
	Name           string `json:"name"`
	Image          string `json:"image"`
	State          string `json:"state"`
	Status         string `json:"status"`
	ComposeProject string `json:"composeProject"`
	ComposeService string `json:"composeService"`
	ComposeDir     string `json:"composeDir"`
	Labels         int    `json:"labels"`
	MaskedLabels   int    `json:"maskedLabels"`
}

type EnrollmentRequest struct {
	HostID string `json:"hostId"`
	Name   string `json:"name"`
}

// EnrollmentResponse carries the code once: the server keeps only its hash.
type EnrollmentResponse struct {
	HostID    string    `json:"hostId"`
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
	Pin       string    `json:"pin"`
	ServerURL string    `json:"serverUrl"`
}

// RemovalPolicy is what happens to a service whose containers disappear. Its settings are
// kept in every case (PRODUCT.md, acceptance criterion 3).
type RemovalPolicy string

const (
	RemovalAbsentThenArchive RemovalPolicy = "absent_then_archive"
	RemovalAbsentOnly        RemovalPolicy = "absent_only"
	RemovalArchiveNow        RemovalPolicy = "archive_now"
)

type Discovery struct {
	FollowEvents        bool          `json:"followEvents"`
	RescanMinutes       int           `json:"rescanMinutes"`
	OnRemoval           RemovalPolicy `json:"onRemoval"`
	ArchiveAfterDays    int           `json:"archiveAfterDays"`
	ConfirmBelowPercent int           `json:"confirmBelowPercent"`
	// InferURLFromPorts is always false: a published port is never assumed to be a usable
	// URL (ARCHITECTURE.md, discovery step 7). It is exposed so the UI can explain why.
	InferURLFromPorts bool `json:"inferUrlFromPorts"`
}

// Validate mirrors the database constraints and returns a message for the user, or "".
func (d Discovery) Validate() string {
	switch {
	case d.InferURLFromPorts:
		return "inferring a URL from a published port is not allowed"
	case d.RescanMinutes < 1 || d.RescanMinutes > 1440:
		return "rescanMinutes must be between 1 and 1440"
	case d.OnRemoval != RemovalAbsentThenArchive && d.OnRemoval != RemovalAbsentOnly && d.OnRemoval != RemovalArchiveNow:
		return "unknown onRemoval policy"
	case d.ArchiveAfterDays < 1 || d.ArchiveAfterDays > 365:
		return "archiveAfterDays must be between 1 and 365"
	case d.ConfirmBelowPercent < 50 || d.ConfirmBelowPercent > 100:
		return "confirmBelowPercent must be between 50 and 100"
	}
	return ""
}

type Settings struct {
	// Editable is false until settings are persisted in PostgreSQL.
	Editable  bool      `json:"editable"`
	Discovery Discovery `json:"discovery"`
}

type ServiceDetail struct {
	ServiceSummary
	Version        string         `json:"version"`
	Tint           string         `json:"tint"`
	Stats          []Stat         `json:"stats"`
	Upcoming       []UpcomingItem `json:"upcoming"`
	Identification []KeyValue     `json:"identification"`
	Access         []KeyValue     `json:"access"`
	Connection     []KeyValue     `json:"connection"`
}
