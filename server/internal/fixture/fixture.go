// Package fixture is the jalon 1 data source: fixed example data in the shape real
// discovery will produce, so the UI can be built and tested without touching any host.
// With a database, pgstore keeps the inventory and only borrows the live state from here
// until agents report it.
package fixture

import (
	"context"
	"strconv"
	"strings"

	"hub/server/internal/api"
	"hub/server/internal/domain"
)

type Store struct {
	agents   []api.AgentHost
	hosts    []api.Host
	services []api.ServiceDetail
	sessions []api.Session
	upcoming []api.UpcomingItem
}

func ms(v int) *int { return &v }

func New() *Store {
	s := &Store{
		hosts: []api.Host{
			{ID: "debian-mini", Name: "debian-mini", OS: "Debian 13", Containers: 22, Reachable: true, LastSeenLabel: "il y a 3 s",
				Metrics: []api.HostMetric{{Label: "CPU", Percent: 18}, {Label: "RAM", Percent: 41}, {Label: "Disque", Percent: 63}}},
			{ID: "qnap-ts464", Name: "qnap-ts464", OS: "QTS", Containers: 9, Reachable: true, LastSeenLabel: "il y a 5 s",
				Metrics: []api.HostMetric{{Label: "CPU", Percent: 9}, {Label: "RAM", Percent: 52}, {Label: "Volume", Percent: 71}}},
			{ID: "rpi5", Name: "rpi5", OS: "Raspberry Pi OS", Containers: 3, Reachable: false, LastSeenLabel: "il y a 6 min"},
		},
		sessions: []api.Session{
			{Title: "Dune : Deuxième partie", Device: "Salon · Apple TV 4K", Progress: 0.62, DurationLabel: "1:42 sur 2:46"},
			{Title: "Severance · S02E06", Device: "Chambre · iPad", Progress: 0.28, DurationLabel: "14 min sur 52"},
			{Title: "Interstellar", Device: "Bureau · navigateur", Progress: 0.81, DurationLabel: "2:20 sur 2:49", Transcoding: true},
		},
		upcoming: []api.UpcomingItem{
			{Title: "Severance", Detail: "S02E07 · série", When: "Ce soir · 21:00", Kind: "episode", Tone: "#2b3a4a"},
			{Title: "The Last of Us", Detail: "S02E03 · série", When: "Demain", Kind: "episode", Tone: "#3b3326"},
			{Title: "Andor", Detail: "S02E04 · série", When: "Mer. 5", Kind: "episode", Tone: "#213a33"},
			{Title: "Mickey 17", Detail: "Film · surveillé", When: "Sortie numérique", Kind: "movie", Tone: "#40262a"},
			{Title: "The Bear", Detail: "S04E01 · série", When: "Jeu. 6", Kind: "episode", Tone: "#2c2c3a"},
			{Title: "Sinners", Detail: "Film · Radarr", When: "48 % téléchargé", Kind: "movie", Tone: "#3a3122"},
		},
	}

	key := func(last4 string) []api.KeyValue {
		return []api.KeyValue{
			{Label: "Clé API", Value: "••••••••" + last4, Mono: true},
			{Label: "Stockage", Value: "chiffrée côté serveur, jamais envoyée au navigateur"},
			{Label: "Rafraîchir", Value: "toutes les 60 s"},
		}
	}

	s.services = []api.ServiceDetail{
		{
			ServiceSummary: api.ServiceSummary{Slug: "emby", Name: "Emby", Monogram: "E", Color: "#2e7d4f", Category: "Médias", HostID: "debian-mini", Status: domain.StatusOnline, LatencyMs: ms(42), Note: "3 lectures"},
			Version:        "v4.9", Tint: "#1e4f35",
			Stats:          []api.Stat{{Value: "3", Label: "lectures en cours"}, {Value: "1", Label: "transcodage"}},
			Identification: []api.KeyValue{{Label: "Reconnu par", Value: "image emby/embyserver"}, {Label: "Confiance", Value: "97 %"}, {Label: "Projet", Value: "Compose media"}},
			Access:         []api.KeyValue{{Label: "Conteneur", Value: "emby · en cours · 12 j"}, {Label: "URL", Value: "https://emby.lan", Mono: true}, {Label: "Vérifiée", Value: "il y a 2 min"}},
			Connection:     key("81c2"),
		},
		{
			ServiceSummary: api.ServiceSummary{Slug: "sonarr", Name: "Sonarr", Monogram: "S", Color: "#1f6fb2", Category: "Médias", HostID: "debian-mini", Status: domain.StatusOnline, LatencyMs: ms(18), Note: "4 cette semaine"},
			Version:        "v4 · 214 séries suivies", Tint: "#24466f",
			Stats:          []api.Stat{{Value: "4", Label: "épisodes cette semaine"}, {Value: "2", Label: "en téléchargement"}, {Value: "0", Label: "épisodes manquants"}},
			Upcoming:       s.upcoming[:3],
			Identification: []api.KeyValue{{Label: "Reconnu par", Value: "image linuxserver/sonarr"}, {Label: "Confiance", Value: "98 % · confirmé"}, {Label: "Projet", Value: "Compose media"}, {Label: "Catégorie", Value: "Médias · modifiable"}},
			Access:         []api.KeyValue{{Label: "Conteneur", Value: "sonarr · en cours · 12 j"}, {Label: "Hôte", Value: "debian-mini · agent connecté"}, {Label: "URL", Value: "https://sonarr.lan", Mono: true}, {Label: "Vérifiée", Value: "il y a 2 min · label homelab.url"}},
			Connection:     key("3f9a"),
		},
		{
			ServiceSummary: api.ServiceSummary{Slug: "radarr", Name: "Radarr", Monogram: "R", Color: "#b8860b", Category: "Médias", HostID: "debian-mini", Status: domain.StatusOnline, LatencyMs: ms(21), Note: "2 en file"},
			Version:        "v5", Tint: "#5a4512",
			Stats:          []api.Stat{{Value: "2", Label: "en file"}, {Value: "1", Label: "sortie cette semaine"}},
			Upcoming:       s.upcoming[3:4],
			Identification: []api.KeyValue{{Label: "Reconnu par", Value: "image linuxserver/radarr"}, {Label: "Confiance", Value: "98 %"}},
			Access:         []api.KeyValue{{Label: "Conteneur", Value: "radarr · en cours · 12 j"}, {Label: "URL", Value: "https://radarr.lan", Mono: true}},
			Connection:     key("0d17"),
		},
		{
			ServiceSummary: api.ServiceSummary{Slug: "qbittorrent", Name: "qBittorrent", Monogram: "qB", Color: "#3d5a80", Category: "Téléchargements", HostID: "qnap-ts464", Status: domain.StatusOnline, LatencyMs: ms(9), Note: "↓ 24,6 Mo/s"},
			Version:        "v5.0", Tint: "#1d2f44",
			Stats:          []api.Stat{{Value: "24,6 Mo/s", Label: "réception"}, {Value: "3", Label: "actifs"}},
			Identification: []api.KeyValue{{Label: "Reconnu par", Value: "image linuxserver/qbittorrent"}, {Label: "Confiance", Value: "95 %"}},
			Access:         []api.KeyValue{{Label: "Conteneur", Value: "qbittorrent · en cours · 4 j"}, {Label: "URL", Value: "https://qbit.lan", Mono: true}},
			Connection:     key("5e60"),
		},
		{
			ServiceSummary: api.ServiceSummary{Slug: "jellyseerr", Name: "Jellyseerr", Monogram: "J", Color: "#6b4e9b", Category: "Médias", HostID: "debian-mini", Status: domain.StatusDegraded, LatencyMs: ms(820), Note: "lent (820 ms)"},
			Version:        "v2.1", Tint: "#2e2442",
			Stats:          []api.Stat{{Value: "2", Label: "demandes en attente"}},
			Identification: []api.KeyValue{{Label: "Reconnu par", Value: "image fallenbagel/jellyseerr"}, {Label: "Confiance", Value: "72 % · à confirmer"}},
			Access:         []api.KeyValue{{Label: "Conteneur", Value: "jellyseerr · en cours · 1 j"}, {Label: "URL", Value: "https://requests.lan", Mono: true}},
			Connection:     []api.KeyValue{{Label: "Clé API", Value: "non configurée"}},
		},
		{
			ServiceSummary: api.ServiceSummary{Slug: "pihole", Name: "Pi-hole", Monogram: "Pi", Color: "#8c2f39", Category: "Réseau", HostID: "rpi5", Status: domain.StatusOnline, Note: "état inconnu"},
			Version:        "v6", Tint: "#3a1d22",
			Stats:          []api.Stat{{Value: "18 %", Label: "bloquées (il y a 6 min)"}},
			Identification: []api.KeyValue{{Label: "Reconnu par", Value: "image pihole/pihole"}, {Label: "Confiance", Value: "99 %"}},
			Access:         []api.KeyValue{{Label: "Conteneur", Value: "pihole · dernier état connu"}, {Label: "Hôte", Value: "rpi5 · agent injoignable"}},
			Connection:     key("aa42"),
		},
	}

	for i := range s.services {
		s.services[i].Favorite = true
	}

	// The rest of the inventory: summary only, no widget connected yet.
	type row struct {
		name, mono, color, category, host, note string
		status                                  domain.Status
		latency                                 int // 0 = no web UI to probe
	}
	online, cfg := domain.StatusOnline, domain.StatusConfigRequired
	for _, r := range []row{
		{"Bazarr", "B", "#4b6b3a", "Médias", "debian-mini", "", online, 25},
		{"Prowlarr", "P", "#5c5c66", "Médias", "debian-mini", "URL à confirmer", cfg, 0},
		{"Lidarr", "L", "#2f6b5e", "Médias", "debian-mini", "", online, 19},
		{"Readarr", "Rd", "#6b4a2a", "Médias", "debian-mini", "", online, 23},
		{"Tdarr", "T", "#3a4f6b", "Médias", "qnap-ts464", "", online, 31},
		{"SABnzbd", "Sa", "#6b5a2a", "Téléchargements", "qnap-ts464", "", online, 12},
		{"FlareSolverr", "F", "#44515c", "Téléchargements", "debian-mini", "sans interface", online, 0},
		{"Unpackerr", "U", "#4d4d55", "Téléchargements", "debian-mini", "sans interface", online, 0},
		{"Gluetun", "G", "#2a5c4a", "Téléchargements", "qnap-ts464", "VPN actif", online, 0},
		{"WireGuard", "W", "#6b2a2a", "Réseau", "rpi5", "", online, 0},
		{"Unbound", "Ub", "#3d3d6b", "Réseau", "rpi5", "", online, 0},
		{"Caddy", "C", "#1f5f6b", "Réseau", "debian-mini", "", online, 3},
		{"Tailscale", "Ts", "#3a3a40", "Réseau", "debian-mini", "sans interface", online, 0},
		{"Portainer", "Po", "#13708a", "Infrastructure", "debian-mini", "", online, 14},
		{"Dockhand", "D", "#2a4f6b", "Infrastructure", "qnap-ts464", "", online, 16},
		{"PostgreSQL", "Pg", "#335f8a", "Infrastructure", "debian-mini", "sans interface", online, 0},
		{"Redis", "Re", "#8a2f2f", "Infrastructure", "debian-mini", "sans interface", online, 0},
		{"Diun", "Di", "#555b6b", "Infrastructure", "debian-mini", "sans interface", online, 0},
		{"Vaultwarden", "V", "#1a5c8a", "Infrastructure", "debian-mini", "", online, 11},
		{"Nextcloud", "N", "#1f5f8a", "Infrastructure", "qnap-ts464", "", online, 48},
		{"Immich", "I", "#4a3a7a", "Infrastructure", "qnap-ts464", "", online, 37},
		{"Syncthing", "Sy", "#2a6b6b", "Infrastructure", "qnap-ts464", "", online, 22},
		{"Uptime Kuma", "K", "#2a7a4a", "Surveillance", "debian-mini", "", online, 15},
		{"Grafana", "Gr", "#8a5a1f", "Surveillance", "debian-mini", "", online, 27},
		{"Prometheus", "Pr", "#8a3f1f", "Surveillance", "debian-mini", "", online, 13},
		{"Loki", "Lo", "#5a4a2a", "Surveillance", "debian-mini", "sans interface", online, 0},
		{"cAdvisor", "cA", "#3f5a6b", "Surveillance", "debian-mini", "sans interface", online, 0},
		{"Gotify", "Go", "#2a5a7a", "Surveillance", "debian-mini", "", online, 10},
	} {
		sum := api.ServiceSummary{Slug: slugify(r.name), Name: r.name, Monogram: r.mono, Color: r.color, Category: r.category, HostID: r.host, Status: r.status, Note: r.note}
		if r.latency > 0 {
			sum.LatencyMs = ms(r.latency)
		}
		s.services = append(s.services, api.ServiceDetail{
			ServiceSummary: sum,
			Tint:           "#1f2a36",
			Stats:          []api.Stat{},
			Identification: []api.KeyValue{{Label: "Catégorie", Value: r.category}},
			Access:         []api.KeyValue{{Label: "Hôte", Value: r.host}},
			Connection:     []api.KeyValue{{Label: "Widget", Value: "non configuré"}},
		})
	}

	// Every sample service has a selfh.st icon of the same name, except Pi-hole (named
	// differently there) and Portainer (light variant: the standard one is unreadable on black).
	renamed := map[string]string{"pihole": "pi-hole", "portainer": "portainer-light"}
	for i := range s.services {
		icon := s.services[i].Slug
		if r, ok := renamed[icon]; ok {
			icon = r
		}
		s.services[i].Icon = &icon
	}

	reachable := map[string]bool{}
	for _, h := range s.hosts {
		reachable[h.ID] = h.Reachable
	}
	for i := range s.services {
		svc := &s.services[i]
		svc.Status = domain.EffectiveStatus(svc.Status, reachable[svc.HostID])
		if svc.Status == domain.StatusUnknown {
			svc.LatencyMs = nil
			if svc.Note == "" {
				svc.Note = "agent muet"
			}
		}
		if svc.Note == "" && svc.LatencyMs != nil {
			svc.Note = strconv.Itoa(*svc.LatencyMs) + " ms"
		}
	}

	s.agents = []api.AgentHost{
		{ID: "debian-mini", Name: "debian-mini", OS: "Debian 13", Arch: "amd64"},
		{ID: "qnap-ts464", Name: "qnap-ts464", OS: "QTS", Arch: "amd64"},
		{ID: "rpi5", Name: "rpi5", OS: "Raspberry Pi OS", Arch: "arm64"},
	}
	return s
}

func slugify(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "-")
}

func (s *Store) Services(context.Context) ([]api.ServiceSummary, error) {
	out := make([]api.ServiceSummary, 0, len(s.services))
	for _, svc := range s.services {
		out = append(out, svc.ServiceSummary)
	}
	return out, nil
}

func (s *Store) Settings(context.Context) (api.Settings, error) {
	return api.Settings{
		Editable: false,
		Discovery: api.Discovery{
			FollowEvents:        true,
			RescanMinutes:       5,
			OnRemoval:           api.RemovalAbsentThenArchive,
			ArchiveAfterDays:    7,
			ConfirmBelowPercent: 80,
			InferURLFromPorts:   false,
		},
	}, nil
}

func (s *Store) Home(context.Context) (api.Home, error) {
	favorites := make([]api.ServiceSummary, 0, len(s.services))
	online := 0
	for _, svc := range s.services {
		if svc.Favorite {
			favorites = append(favorites, svc.ServiceSummary)
		}
		if svc.Status == domain.StatusOnline {
			online++
		}
	}
	return api.Home{
		Summary:   api.Summary{Online: online, Total: len(s.services)},
		Sessions:  s.sessions,
		Favorites: favorites,
		Upcoming:  s.upcoming,
		Hosts:     s.hosts,
	}, nil
}

func (s *Store) Service(_ context.Context, slug string) (api.ServiceDetail, error) {
	for _, svc := range s.services {
		if svc.Slug == slug {
			if svc.Upcoming == nil {
				svc.Upcoming = []api.UpcomingItem{}
			}
			return svc, nil
		}
	}
	return api.ServiceDetail{}, api.ErrNotFound
}

// Seed is the example inventory written to an empty database: hosts, then services in
// display order.
func (s *Store) Seed() ([]api.AgentHost, []api.ServiceSummary) {
	services, _ := s.Services(context.Background())
	return s.agents, services
}
