package pgstore_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"hub/server/internal/api"
	"hub/server/internal/domain"
	"hub/server/internal/urlcheck"
	"hub/server/internal/widgets"
)

func (e *discoveryEnv) connect(t *testing.T, slug, kind string, enabled []string, data map[string]string) {
	t.Helper()
	address := "http://" + slug
	if err := e.store.SaveServiceURL(e.ctx, slug, address, "", urlcheck.Result{Kind: urlcheck.KindOK, Status: 200}); err != nil {
		t.Fatal(err)
	}
	raw := map[string]json.RawMessage{}
	for k, v := range data {
		raw[k] = json.RawMessage(v)
	}
	err := e.store.SaveWidgetConnection(e.ctx, slug, api.NewConnection{Kind: kind, URL: address, KeyHint: "abcd", Options: map[string]string{},
		Enabled: enabled, RefreshSeconds: 30, State: widgets.StateOK, At: time.Now(), Data: raw})
	if err != nil {
		t.Fatal(err)
	}
}

func sectionsEnv(t *testing.T) *discoveryEnv {
	t.Helper()
	e := newDiscoveryEnv(t, "mini", "qnap")
	e.snapshot(t, "mini",
		e.container("emby", "emby", "linuxserver/emby", "running", 8096),
		e.container("sonarr", "sonarr", "linuxserver/sonarr", "running", 8989),
		e.container("radarr", "radarr", "linuxserver/radarr", "running", 7878),
		e.container("seerr", "seerr", "seerr-team/seerr", "running", 5055))
	e.snapshot(t, "qnap", e.container("qbittorrent", "qbittorrent", "linuxserver/qbittorrent", "running", 8181))
	return e
}

func TestDownloadsPageIsTheOnlyOneWithNames(t *testing.T) {
	e := sectionsEnv(t)
	transfer := `{"downSpeed":12400000,"upSpeed":1100000,"downloading":1,"connection":"connected"}`
	e.connect(t, "qbittorrent", "qbittorrent", []string{"transfer"}, map[string]string{
		"transfer": transfer,
		"torrents": `[{"name":"Un-nom-de-torrent","progress":0.5,"downSpeed":12400000,"eta":240,"state":"downloading"}]`,
	})
	e.connect(t, "sonarr", "sonarr", []string{"queue"}, map[string]string{
		"queue":      `{"total":1,"downloading":1,"problems":0}`,
		"queueItems": `[{"title":"Une-serie-privee","detail":"S02E06 · Sonarr","status":"downloading"}]`,
	})

	page, err := e.store.DownloadsPage(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if page.Connected != 2 || page.Transfer == nil || page.Transfer.DownSpeed != 12400000 || page.TransferHost != "qnap" {
		t.Errorf("%+v", page)
	}
	if len(page.Torrents) != 1 || page.Torrents[0].Name != "Un-nom-de-torrent" || len(page.Queue) != 1 {
		t.Errorf("%+v %+v", page.Torrents, page.Queue)
	}
	if len(page.Cards) != 1 || page.Cards[0].Label != "Sonarr · file d’attente" {
		t.Errorf("transfers belong at the top, not in a card: %+v", page.Cards)
	}
	if len(page.ToConnect) != 1 || page.ToConnect[0].Slug != "radarr" {
		t.Errorf("to connect: %+v", page.ToConnect)
	}

	// Neither the home page nor a fiche carries a name.
	home, err := e.store.Home(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(home)
	if strings.Contains(string(raw), "Un-nom-de-torrent") || strings.Contains(string(raw), "Une-serie-privee") {
		t.Errorf("name on the home page: %s", raw)
	}
	for _, slug := range []string{"qbittorrent", "sonarr"} {
		d, err := e.store.Service(e.ctx, slug)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(d)
		if strings.Contains(string(raw), "Un-nom-de-torrent") || strings.Contains(string(raw), "Une-serie-privee") {
			t.Errorf("name on the fiche of %s: %s", slug, raw)
		}
	}
}

func TestDownloadsPageSaysWhatIsRefused(t *testing.T) {
	e := sectionsEnv(t)
	e.connect(t, "sonarr", "sonarr", []string{"queue"}, map[string]string{"queue": `{"total":1,"downloading":0,"problems":0}`})
	list, err := e.store.DueConnections(e.ctx, time.Now().Add(time.Hour))
	if err != nil || len(list) != 1 {
		t.Fatalf("%+v %v", list, err)
	}
	if err := e.store.RecordRead(e.ctx, list[0], nil, widgets.StateAuth, time.Now()); err != nil {
		t.Fatal(err)
	}
	page, err := e.store.DownloadsPage(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Cards) != 0 || len(page.Notices) != 1 || page.Notices[0].State != "auth" {
		t.Errorf("%+v", page)
	}
}

func TestMediaPageGathersItsWidgets(t *testing.T) {
	e := sectionsEnv(t)
	e.connect(t, "emby", "emby", []string{"sessions", "latest", "library"}, map[string]string{
		"sessions": `[{"title":"Le film","device":"Salon","progress":0.5}]`,
		"latest":   `[{"title":"Un film","detail":"Film","kind":"movie","addedAt":"2026-11-05T10:00:00Z"}]`,
		"library":  `{"movies":412,"series":86,"episodes":3120}`,
	})
	e.connect(t, "seerr", "seerr", []string{"requests"}, map[string]string{"requests": `{"total":9,"pending":4}`})
	e.connect(t, "radarr", "radarr", []string{"calendar"}, map[string]string{"calendar": `[{"title":"Un film","detail":"En salle · Radarr","kind":"movie","at":"2030-01-08T00:00:00Z"}]`})

	page, err := e.store.MediaPage(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if page.Connected != 3 || page.SessionsSlug != "emby" || len(page.Sessions) != 1 || len(page.Latest) != 1 || len(page.Upcoming) != 1 {
		t.Errorf("%+v", page)
	}
	if page.Library == nil || page.Library.Movies != 412 || page.Requests == nil || page.Requests.Pending != 4 || page.RequestsSlug != "seerr" {
		t.Errorf("%+v", page)
	}
	if page.RequestsURL == nil || *page.RequestsURL != "http://seerr" {
		t.Errorf("%v", page.RequestsURL)
	}
	if len(page.Services) != 2 { // Emby and Seerr are the catalogue's media services
		t.Errorf("services: %+v", page.Services)
	}
}

func TestInfraPageListsHostsAndWhatToWatch(t *testing.T) {
	e := sectionsEnv(t)
	e.snapshot(t, "qnap", e.container("qbittorrent", "qbittorrent", "linuxserver/qbittorrent", "exited", 8181))
	page, err := e.store.InfraPage(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hosts) != 2 || page.Total != 5 {
		t.Fatalf("%+v", page)
	}
	for _, h := range page.Hosts {
		want := map[string]int{"mini": 4, "qnap": 1}[h.ID]
		if h.Services != want {
			t.Errorf("%s: %d services", h.ID, h.Services)
		}
	}
	if len(page.Watch) != 1 || page.Watch[0].Slug != "qbittorrent" || page.Watch[0].Status != domain.StatusDown {
		t.Errorf("watch: %+v", page.Watch)
	}
	if len(page.Groups) != 2 {
		t.Errorf("groups: %+v", page.Groups)
	}
}
