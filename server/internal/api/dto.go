package api

import "hub/server/internal/domain"

// DTOs sent to the browser. They mirror web/src/lib/api/types.ts and must never carry a
// secret: credentials only ever appear as an already-masked string.

type ServiceSummary struct {
	Slug      string        `json:"slug"`
	Name      string        `json:"name"`
	Monogram  string        `json:"monogram"`
	Color     string        `json:"color"`
	Category  string        `json:"category"`
	HostID    string        `json:"hostId"`
	Status    domain.Status `json:"status"`
	LatencyMs *int          `json:"latencyMs"`
	Note      string        `json:"note"`
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
