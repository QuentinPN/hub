package api

import (
	"encoding/json"
	"time"

	"hub/server/internal/domain"
	"hub/server/internal/widgets"
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
	// URL is the address to open, once the user gave one.
	URL *string `json:"url"`
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
	// At replaces When and Tone for what a connection read: the page words the date itself,
	// in the viewer's time zone.
	At string `json:"at,omitempty"`
}

// DownloadCard is a card of the home page's "Téléchargements".
type DownloadCard struct {
	Slug     string     `json:"slug"`
	Label    string     `json:"label"`
	Headline string     `json:"headline"`
	Note     string     `json:"note"`
	State    string     `json:"state"`
	DataAt   *time.Time `json:"dataAt"`
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
	// Rows lists the visible rows, in the user's order.
	Rows     []string  `json:"rows"`
	Summary  Summary   `json:"summary"`
	Sessions []Session `json:"sessions"`
	// SessionsSlug is the service they come from, once a real connection reads them.
	SessionsSlug string           `json:"sessionsSlug,omitempty"`
	Favorites    []ServiceSummary `json:"favorites"`
	// Services is every service while no favorite is chosen, empty otherwise.
	Services []ServiceSummary `json:"services"`
	Upcoming []UpcomingItem   `json:"upcoming"`
	// Downloads holds the cards of the connections that read a queue.
	Downloads []DownloadCard `json:"downloads,omitempty"`
	Hosts     []Host         `json:"hosts"`
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
	// The rest only exists for a service found by discovery.
	Discovered bool         `json:"discovered,omitempty"`
	Hidden     bool         `json:"hidden,omitempty"`
	Ports      []int        `json:"ports,omitempty"`
	URLCheck   *URLCheck    `json:"urlCheck,omitempty"`
	Project    *ProjectRef  `json:"project,omitempty"`
	Links      []ProjectRef `json:"links,omitempty"`
	Linkable   []ProjectRef `json:"linkable,omitempty"`
	Widget     *WidgetInfo  `json:"widget,omitempty"`
}

// Widget states the UI words itself: those of the last read, plus two it decides from the
// service as it is now.
const (
	WidgetStopped        = "stopped"         // its containers are stopped: its API is not asked
	WidgetAddressChanged = "address_changed" // the key was tested on another address: it stays home
)

// WidgetInfo is what a service offers as widgets, and its connection if there is one.
type WidgetInfo struct {
	Support        widgets.Support   `json:"support"`
	Manifest       *widgets.Manifest `json:"manifest,omitempty"`
	RefreshChoices []int             `json:"refreshChoices,omitempty"`
	Connection     *WidgetConnection `json:"connection,omitempty"`
}

// WidgetConnection never carries the key: only its last characters.
type WidgetConnection struct {
	State          string            `json:"state"`
	KeyHint        string            `json:"keyHint"`
	RefreshSeconds int               `json:"refreshSeconds"`
	Enabled        []string          `json:"enabled"`
	Options        map[string]string `json:"options"`
	DataAt         *time.Time        `json:"dataAt"`
	CheckedAt      *time.Time        `json:"checkedAt"`
	FailingSince   *time.Time        `json:"failingSince"`
	// Data holds the last successful read, widget by widget, while it still means something.
	Data map[string]json.RawMessage `json:"data"`
}

// WidgetRow is a line of the settings' "Widgets et connexions".
type WidgetRow struct {
	Slug     string          `json:"slug"`
	Name     string          `json:"name"`
	Monogram string          `json:"monogram"`
	Icon     *string         `json:"icon"`
	Color    string          `json:"color"`
	HostID   string          `json:"hostId"`
	HasURL   bool            `json:"hasUrl"`
	Support  widgets.Support `json:"support"`
	// State is empty while not connected.
	State        string     `json:"state"`
	DataAt       *time.Time `json:"dataAt"`
	FailingSince *time.Time `json:"failingSince"`
}

// WidgetTest answers a key test. The options only exist for a key that works.
type WidgetTest struct {
	Result    widgets.State    `json:"result"`
	LatencyMs int              `json:"latencyMs"`
	Note      string           `json:"note,omitempty"`
	Options   []widgets.Option `json:"options,omitempty"`
}

// WidgetSave is what the connection panel sends. An empty key keeps the stored one.
type WidgetSave struct {
	APIKey         string            `json:"apiKey"`
	Widgets        []string          `json:"widgets"`
	RefreshSeconds int               `json:"refreshSeconds"`
	Options        map[string]string `json:"options"`
}

// WidgetSaved answers a save: a refused key leaves everything as it was.
type WidgetSaved struct {
	Saved   bool           `json:"saved"`
	Result  widgets.State  `json:"result"`
	Service *ServiceDetail `json:"service,omitempty"`
}

// URLCheck is the last verdict on a service's address.
type URLCheck struct {
	Result     string     `json:"result"`
	Status     int        `json:"status"`
	LatencyMs  int        `json:"latencyMs"`
	CheckedAt  *time.Time `json:"checkedAt"`
	CertPinned bool       `json:"certPinned"`
}

// URLCheckResponse answers an attempt to set an address. The fingerprint is for the user
// to accept that very certificate.
type URLCheckResponse struct {
	Saved      bool   `json:"saved"`
	Result     string `json:"result"`
	Status     int    `json:"status"`
	LatencyMs  int    `json:"latencyMs"`
	CertSHA256 string `json:"certSha256,omitempty"`
	CertName   string `json:"certName,omitempty"`
}

// ProjectRef is a Compose project (or lone container) of the service's host.
type ProjectRef struct {
	Key        string `json:"key"`
	Containers int    `json:"containers"`
}

// ServiceEdit is what the "Modifier" panel sends.
type ServiceEdit struct {
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Icon     string   `json:"icon"`
	Hidden   bool     `json:"hidden"`
	Links    []string `json:"links"`
}

// HiddenGroup is a project the catalogue leaves out. Reason: agent, database, self or user.
type HiddenGroup struct {
	HostID     string `json:"hostId"`
	Key        string `json:"key"`
	Reason     string `json:"reason"`
	Containers int    `json:"containers"`
}
