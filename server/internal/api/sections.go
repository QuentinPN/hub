package api

import (
	"context"
	"net/http"
	"time"

	"hub/server/internal/widgets"
)

// SectionReader is implemented by stores that can lay out the top-level pages.
type SectionReader interface {
	MediaPage(ctx context.Context) (MediaPage, error)
	DownloadsPage(ctx context.Context) (DownloadsPage, error)
	InfraPage(ctx context.Context) (InfraPage, error)
}

// SectionNotice says a connection is not answering as it should.
type SectionNotice struct {
	Slug   string     `json:"slug"`
	Name   string     `json:"name"`
	State  string     `json:"state"`
	DataAt *time.Time `json:"dataAt"`
}

// SectionBase is what every connected page shares. ToConnect fills the page while nothing
// of its own is connected yet.
type SectionBase struct {
	Connected int             `json:"connected"`
	Notices   []SectionNotice `json:"notices"`
	ToConnect []WidgetRow     `json:"toConnect"`
}

// MediaPage gathers what plays, what is coming and what was added.
type MediaPage struct {
	SectionBase
	Sessions     []Session           `json:"sessions"`
	SessionsSlug string              `json:"sessionsSlug,omitempty"`
	Upcoming     []UpcomingItem      `json:"upcoming"`
	Latest       []widgets.MediaItem `json:"latest"`
	LatestSlug   string              `json:"latestSlug,omitempty"`
	Requests     *widgets.Requests   `json:"requests"`
	RequestsSlug string              `json:"requestsSlug,omitempty"`
	RequestsURL  *string             `json:"requestsUrl"`
	Library      *widgets.Library    `json:"library"`
	Services     []ServiceSummary    `json:"services"`
}

// DownloadsPage is the one place where names of torrents and releases reach the browser.
type DownloadsPage struct {
	SectionBase
	Transfer     *widgets.Transfer   `json:"transfer"`
	TransferSlug string              `json:"transferSlug,omitempty"`
	TransferHost string              `json:"transferHost,omitempty"`
	Torrents     []widgets.Torrent   `json:"torrents"`
	Queue        []widgets.QueueItem `json:"queue"`
	Cards        []DownloadCard      `json:"cards"`
}

// InfraHost is a host with the number of services it runs.
type InfraHost struct {
	AgentHost
	Services int `json:"services"`
}

// InfraGroup lists a host's services.
type InfraGroup struct {
	HostID   string           `json:"hostId"`
	Name     string           `json:"name"`
	Services []ServiceSummary `json:"services"`
}

// InfraPage is the state of the machines and of what they run.
type InfraPage struct {
	Hosts  []InfraHost      `json:"hosts"`
	Total  int              `json:"total"`
	Watch  []ServiceSummary `json:"watch"`
	Groups []InfraGroup     `json:"groups"`
}

func registerSections(mux *http.ServeMux, sections SectionReader) {
	mux.HandleFunc("GET /api/v1/sections/media", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(sections.MediaPage(r.Context()))
	})
	mux.HandleFunc("GET /api/v1/sections/downloads", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(sections.DownloadsPage(r.Context()))
	})
	mux.HandleFunc("GET /api/v1/sections/infra", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(sections.InfraPage(r.Context()))
	})
}
