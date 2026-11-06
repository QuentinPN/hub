package reconcile

// signature describes an image the catalogue recognizes.
type signature struct {
	slug, name, icon, color, category string
}

const (
	categoryMedia     = "Médias"
	categoryDownloads = "Téléchargements"
	categoryAdmin     = "Administration"
	categoryOwn       = "Mes applications"
	categoryOther     = "Autres"
)

// applications is keyed by the last segment of the image name, without its tag.
var applications = map[string]signature{
	"emby":            {"emby", "Emby", "emby", "#2e7d4f", categoryMedia},
	"embyserver":      {"emby", "Emby", "emby", "#2e7d4f", categoryMedia},
	"seerr":           {"seerr", "Seerr", "seerr", "#6b4e9b", categoryMedia},
	"jellyseerr":      {"jellyseerr", "Jellyseerr", "jellyseerr", "#6b4e9b", categoryMedia},
	"suwayomi-server": {"suwayomi", "Suwayomi", "suwayomi", "#8c5a2f", categoryMedia},

	"sonarr":       {"sonarr", "Sonarr", "sonarr", "#1f6fb2", categoryDownloads},
	"radarr":       {"radarr", "Radarr", "radarr", "#b8860b", categoryDownloads},
	"prowlarr":     {"prowlarr", "Prowlarr", "prowlarr", "#a0522d", categoryDownloads},
	"flaresolverr": {"flaresolverr", "FlareSolverr", "flaresolverr", "#5b6b7a", categoryDownloads},
	"qbittorrent":  {"qbittorrent", "qBittorrent", "qbittorrent", "#3d5a80", categoryDownloads},

	"beszel":         {"beszel", "Beszel", "beszel", "#3f6f5f", categoryAdmin},
	"dockhand":       {"dockhand", "Dockhand", "dockhand", "#3a5f8a", categoryAdmin},
	"graphql-engine": {"hasura", "Hasura", "hasura", "#2a4a8a", categoryAdmin},
	"caddy":          {"caddy", "Caddy", "caddy", "#1f7a8c", categoryAdmin},
}

// ownProjects names the Compose projects of the owner's own applications, which no image
// signature can recognize.
var ownProjects = map[string]signature{
	"animeschedule":  {"animeschedule", "AnimeSchedule", "", "#7a4e9b", categoryOwn},
	"tzmasters-prod": {"tzmasters", "TZ Masters", "", "#a23b4f", categoryOwn},
	"tzmasters-dev":  {"tzmasters-dev", "TZ Masters Dev", "", "#7c3545", categoryOwn},
	"downloader": {"downloader", "Downloader", "", "#2f7f86", categoryOwn},
}

var (
	// Tools running on a host (agents, builders): never a service of their own.
	agentImages = map[string]bool{"beszel-agent": true, "hawser": true, "socket-proxy": true, "hub-agent": true, "buildkit": true}
	// A project with nothing else is a shared database, not a service.
	databaseImages = map[string]bool{
		"postgres": true, "postgresql": true, "timescaledb": true, "mariadb": true, "mysql": true,
		"mongo": true, "redis": true, "valkey": true,
	}
	// Hub's own server marks its project, hidden from its own catalogue.
	selfImages = map[string]bool{"hub-server": true}
)

// fallbackColors gives an unrecognized service a stable color.
var fallbackColors = []string{"#2f6f8f", "#6b4e9b", "#2e7d4f", "#a0522d", "#8c2f39", "#3d5a80"}
