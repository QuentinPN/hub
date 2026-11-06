package widgets

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"hub/server/internal/secrets"
)

// WidgetTransfer is the widget of a qBittorrent connection.
const WidgetTransfer = "transfer"

// The routes come from its WebUI API page, the key from "API Key Authentication (≥v5.2.0)".
// No live connection: that documentation describes requests only.
type qbittorrent struct{}

// DataTorrents holds the torrents in progress. Names: for the Téléchargements page only.
const DataTorrents = "torrents"

// maxTorrents bounds the list: the busiest first, the total stays in Transfer.
const maxTorrents = 20

// noETA is what qBittorrent's own example shows when nothing comes in: no real estimate.
const noETA = 8640000

// Torrent is a download in progress.
type Torrent struct {
	Name      string  `json:"name"`
	Progress  float64 `json:"progress"`
	DownSpeed int64   `json:"downSpeed"`
	// ETA is in seconds; 0 when there is no estimate.
	ETA   int64  `json:"eta"`
	State string `json:"state"`
}

// Transfer is what the status bar says. No torrent name: only how many.
type Transfer struct {
	DownSpeed   int64  `json:"downSpeed"`
	UpSpeed     int64  `json:"upSpeed"`
	Downloading int    `json:"downloading"`
	Connection  string `json:"connection"`
}

func (qbittorrent) Manifest() Manifest {
	return Manifest{
		Kind:    "qbittorrent",
		Name:    "qBittorrent",
		KeyHelp: "Dans qBittorrent 5.2 ou plus : Options, Interface web, Clé d’API, Générer. Il n’y a qu’une clé : en générer une autre invalide la précédente.",
		Reads:   []string{"GET /api/v2/transfer/info", "GET /api/v2/torrents/info?filter=downloading"},
		Widgets: []Widget{{ID: WidgetTransfer, Label: "Transferts", Where: "accueil et fiche"}},
	}
}

func (qbittorrent) Authorize(req *http.Request, key secrets.Value) {
	req.Header.Set("Authorization", "Bearer "+key.Reveal())
}

func (qbittorrent) read(ctx context.Context, c *Conn) (Transfer, []Torrent, error) {
	var info struct {
		DlInfoSpeed      int64  `json:"dl_info_speed"`
		UpInfoSpeed      int64  `json:"up_info_speed"`
		ConnectionStatus string `json:"connection_status"`
	}
	if err := c.GetJSON(ctx, "/api/v2/transfer/info", nil, &info); err != nil {
		return Transfer{}, nil, err
	}
	var found []struct {
		Name     string
		Progress float64
		Dlspeed  int64
		Eta      int64
		State    string
	}
	if err := c.GetJSON(ctx, "/api/v2/torrents/info", url.Values{"filter": {"downloading"}}, &found); err != nil {
		return Transfer{}, nil, err
	}
	torrents := make([]Torrent, 0, len(found))
	for _, f := range found {
		t := Torrent{Name: f.Name, Progress: f.Progress, DownSpeed: f.Dlspeed, State: f.State}
		if f.Dlspeed > 0 && f.Eta > 0 && f.Eta < noETA {
			t.ETA = f.Eta
		}
		torrents = append(torrents, t)
	}
	slices.SortStableFunc(torrents, func(a, b Torrent) int { return cmp.Compare(b.DownSpeed, a.DownSpeed) })
	return Transfer{DownSpeed: info.DlInfoSpeed, UpSpeed: info.UpInfoSpeed, Downloading: len(found), Connection: info.ConnectionStatus},
		torrents[:min(len(torrents), maxTorrents)], nil
}

func (q qbittorrent) Probe(ctx context.Context, c *Conn) (Probe, error) {
	t, _, err := q.read(ctx, c)
	if err != nil {
		return Probe{}, err
	}
	note := "Aucun téléchargement en cours"
	switch {
	case t.Downloading == 1:
		note = "1 téléchargement en cours"
	case t.Downloading > 1:
		note = strconv.Itoa(t.Downloading) + " téléchargements en cours"
	}
	return Probe{Note: note, Options: []Option{}}, nil
}

func (q qbittorrent) Fetch(ctx context.Context, c *Conn, enabled []string) (map[string]any, error) {
	data := map[string]any{}
	if slices.Contains(enabled, WidgetTransfer) {
		t, torrents, err := q.read(ctx, c)
		if err != nil {
			return nil, err
		}
		data[WidgetTransfer], data[DataTorrents] = t, torrents
	}
	return data, nil
}

// TransferText words the transfers for the home page.
func TransferText(t Transfer) (headline, note string) {
	headline = Speed(t.DownSpeed)
	note = strconv.Itoa(t.Downloading) + " en cours · envoi " + Speed(t.UpSpeed)
	if t.Connection == "disconnected" {
		note += " · déconnecté"
	}
	return headline, note
}

// Speed writes bytes per second the French way: « 12,4 Mo/s ».
func Speed(bytes int64) string {
	switch {
	case bytes >= 1_000_000:
		return strings.Replace(fmt.Sprintf("%.1f", float64(bytes)/1_000_000), ".", ",", 1) + " Mo/s"
	case bytes >= 1_000:
		return strconv.FormatInt(bytes/1_000, 10) + " Ko/s"
	}
	return strconv.FormatInt(bytes, 10) + " o/s"
}
