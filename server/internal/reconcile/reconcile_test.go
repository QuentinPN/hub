package reconcile_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"hub/server/internal/domain"
	"hub/server/internal/reconcile"
)

const initDepends = "postgres:service_healthy:false,init-db:service_completed_successfully:false"

// homelab reads the real inventory of 2026-11-06, per host. Compose's depends_on labels
// were not part of that reading: the containers waiting for init-db get theirs here.
func homelab(t *testing.T) map[string][]reconcile.Container {
	t.Helper()
	data, err := os.ReadFile("testdata/homelab.txt")
	if err != nil {
		t.Fatal(err)
	}
	hosts := map[string][]reconcile.Container{}
	for line := range strings.Lines(string(data)) {
		f := strings.Split(strings.TrimSpace(line), " |")
		for i := range f {
			f[i] = strings.TrimSpace(f[i])
		}
		c := reconcile.Container{ComposeProject: f[1], ComposeService: f[2], Name: f[3], Image: f[4], State: f[5]}
		if len(f) > 6 && f[6] != "" {
			for p := range strings.SplitSeq(f[6], ",") {
				n, err := strconv.Atoi(p)
				if err != nil {
					t.Fatal(err)
				}
				c.PublicPorts = append(c.PublicPorts, n)
			}
		}
		if (f[1] == "tzmasters-prod" || f[1] == "tzmasters-dev") && f[2] != "init-db" {
			c.Labels = map[string]string{"com.docker.compose.depends_on": initDepends}
		}
		hosts[f[0]] = append(hosts[f[0]], c)
	}
	return hosts
}

func TestHomelabGroups(t *testing.T) {
	type want struct{ name, category, main, note string }
	wantServices := map[string]map[string]want{
		"mini-pc": {
			"emby":           {"Emby", "Médias", "emby", "ports 8096, 8920"},
			"seer":           {"Seerr", "Médias", "seerr", "port 5055"},
			"suwayomi":       {"Suwayomi", "Médias", "suwayomi", "port 5567"},
			"sonarr":         {"Sonarr", "Téléchargements", "sonarr", "port 8989"},
			"radarr":         {"Radarr", "Téléchargements", "radarr", "port 7878"},
			"prowlarr":       {"Prowlarr", "Téléchargements", "prowlarr", "port 9696"},
			"flaresolverr":   {"FlareSolverr", "Téléchargements", "flaresolverr", "aucun port publié"},
			"beszel":         {"Beszel", "Administration", "beszel", "port 8090"},
			"dockhand":       {"Dockhand", "Administration", "dockhand", "port 3000"},
			"hasura":         {"Hasura", "Administration", "hasura", "port 8080"},
			"caddy":          {"Caddy", "Administration", "caddy", "ports 80, 443"},
			"animeschedule":  {"AnimeSchedule", "Mes applications", "animeschedule-anime-admin-1", "port 5178"},
			"tzmasters-prod": {"TZ Masters", "Mes applications", "tzmasters", "aucun port publié"},
			"tzmasters-dev":  {"TZ Masters Dev", "Mes applications", "tzmasters-dev", "aucun port publié"},
			"downloader": {"Downloader", "Mes applications", "downloader_back", "aucun port publié"},
		},
		"qnap":         {"qbitglue": {"qBittorrent", "Téléchargements", "qbittorrent", "port 8181"}},
		"raspberry-pi": {},
	}
	wantHidden := map[string]map[string]reconcile.Kind{
		"mini-pc": {
			"hub": reconcile.KindSelf, "hub-agent": reconcile.KindAgent,
			"animethemes-postgres": reconcile.KindDatabase, "timescaledb": reconcile.KindDatabase,
			"tzmasters-db-prod": reconcile.KindDatabase, "tzmasters-db-dev": reconcile.KindDatabase,
		},
		"qnap":         {"hub-agent": reconcile.KindAgent, "beszel-agent": reconcile.KindAgent, "hawser": reconcile.KindAgent},
		"raspberry-pi": {"hub-agent": reconcile.KindAgent, "beszel-agent": reconcile.KindAgent, "hawser": reconcile.KindAgent},
	}

	total := 0
	for host, list := range homelab(t) {
		total += len(list)
		seen := map[string]bool{}
		for _, g := range reconcile.Groups(list) {
			seen[g.Key] = true
			if g.Kind != reconcile.KindService {
				if wantHidden[host][g.Key] != g.Kind {
					t.Errorf("%s/%s: kind %s, want %q", host, g.Key, g.Kind, wantHidden[host][g.Key])
				}
				continue
			}
			w, ok := wantServices[host][g.Key]
			if !ok {
				t.Errorf("%s/%s: unexpected service %q", host, g.Key, g.Name)
				continue
			}
			var main string
			for _, m := range g.Members {
				if m.Role == reconcile.RoleMain {
					main = m.Name
				}
			}
			status, note := g.Status()
			if g.Name != w.name || g.Category != w.category || main != w.main || note != w.note || status != domain.StatusConfigRequired {
				t.Errorf("%s/%s: name %q category %q main %q status %q note %q, want %+v", host, g.Key, g.Name, g.Category, main, status, note, w)
			}
		}
		for key := range wantServices[host] {
			if !seen[key] {
				t.Errorf("%s/%s: missing", host, key)
			}
		}
		for key := range wantHidden[host] {
			if !seen[key] {
				t.Errorf("%s/%s: missing", host, key)
			}
		}
	}
	if total != 46 {
		t.Fatalf("read %d containers, want 46", total)
	}
}

func TestStatusFollowsTheContainers(t *testing.T) {
	app := func(service, state string, labels map[string]string) reconcile.Container {
		return reconcile.Container{Name: service, Image: "org/" + service + ":1", State: state, ComposeProject: "app", ComposeService: service, Labels: labels}
	}
	waits := map[string]string{"com.docker.compose.depends_on": "init:service_completed_successfully:false"}
	cases := []struct {
		name string
		list []reconcile.Container
		want domain.Status
	}{
		{"all running", []reconcile.Container{app("app", "running", nil), app("worker", "running", nil)}, domain.StatusConfigRequired},
		{"main stopped", []reconcile.Container{app("app", "exited", nil), app("worker", "running", nil)}, domain.StatusDown},
		{"main paused", []reconcile.Container{app("app", "paused", nil)}, domain.StatusDegraded},
		{"main restarting", []reconcile.Container{app("app", "restarting", nil)}, domain.StatusDegraded},
		{"support stopped", []reconcile.Container{app("app", "running", nil), app("worker", "exited", nil)}, domain.StatusDegraded},
		{"finished task", []reconcile.Container{app("app", "running", waits), app("init", "exited", nil)}, domain.StatusConfigRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			groups := reconcile.Groups(c.list)
			if len(groups) != 1 {
				t.Fatalf("got %d groups", len(groups))
			}
			if got, _ := groups[0].Status(); got != c.want {
				t.Fatalf("status %q, want %q", got, c.want)
			}
		})
	}
}

func TestOutsideComposeAndUnknown(t *testing.T) {
	groups := reconcile.Groups([]reconcile.Container{
		{Name: "my_tool.v2", Image: "example.org:5000/team/thing@sha256:abc", State: "running"},
		{Name: "other", Image: "Radarr:latest", State: "exited"},
	})
	if len(groups) != 2 {
		t.Fatalf("got %d groups", len(groups))
	}
	if g := groups[0]; g.Key != "my_tool.v2" || g.Slug != "my-tool-v2" || g.Name != "My Tool V2" || g.Monogram != "M" || g.Icon != nil || g.Category != "Autres" {
		t.Fatalf("unknown container: %+v", g)
	}
	if g := groups[1]; g.Name != "Radarr" || g.Key != "other" {
		t.Fatalf("image case ignored: %+v", g)
	}
	if status, _ := groups[1].Status(); status != domain.StatusDown {
		t.Fatalf("status %q", status)
	}
}

func TestGroupsAreStable(t *testing.T) {
	list := homelab(t)["mini-pc"]
	first := reconcile.Groups(list)
	reversed := make([]reconcile.Container, 0, len(list))
	for i := len(list) - 1; i >= 0; i-- {
		reversed = append(reversed, list[i])
	}
	second := reconcile.Groups(reversed)
	for i := range first {
		if first[i].Key != second[i].Key || first[i].Slug != second[i].Slug || first[i].Color != second[i].Color {
			t.Fatalf("%+v differs from %+v", first[i], second[i])
		}
	}
}

func TestSplitServiceIsAGroupOfItsOwn(t *testing.T) {
	admin := reconcile.Container{Name: "anime-admin-1", Image: "org/anime-admin:1", State: "running", ComposeProject: "anime", ComposeService: "admin", PublicPorts: []int{5178}}
	web := reconcile.Container{Name: "anime-web-1", Image: "org/anime-web:1", State: "running", ComposeProject: "anime", ComposeService: "web", Split: true}
	lone := reconcile.Container{Name: "solo", Image: "org/solo:1", State: "running", Split: true}
	groups := reconcile.Groups([]reconcile.Container{admin, web, lone})
	keys := []string{}
	for _, g := range groups {
		keys = append(keys, g.Key)
	}
	if strings.Join(keys, ",") != "anime,anime/web,solo" {
		t.Fatalf("keys %v", keys)
	}
	if g := groups[1]; len(g.Members) != 1 || g.Members[0].Role != reconcile.RoleMain || g.Name != "Anime Web" || g.Slug != "anime-web" {
		t.Fatalf("split group %+v", g)
	}
	if p, s, ok := reconcile.SplitOf("anime/web"); !ok || p != "anime" || s != "web" {
		t.Fatalf("SplitOf: %q %q %v", p, s, ok)
	}
	if _, _, ok := reconcile.SplitOf("anime"); ok {
		t.Fatal("a project's key read as a split")
	}
}

// Request's own Compose file runs its image beside a Caddy of its own: the application is
// the main container, by its image.
func TestRequestIsRecognizedByItsImage(t *testing.T) {
	groups := reconcile.Groups([]reconcile.Container{
		{Name: "request-caddy-1", Image: "ghcr.io/quentinpn/request-caddy:latest", State: "running", ComposeProject: "request", ComposeService: "caddy"},
		{Name: "request-request-1", Image: "ghcr.io/quentinpn/request:latest", State: "running", ComposeProject: "request", ComposeService: "request"},
	})
	if len(groups) != 1 {
		t.Fatalf("got %d groups", len(groups))
	}
	if g := groups[0]; g.Slug != "request" || g.Name != "Request" || g.Category != "Médias" || g.RecognizedBy != "image request" {
		t.Errorf("%+v", g)
	}
}
