package widgets_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hub/server/internal/secrets"
	"hub/server/internal/widgets"
)

func fakeQbittorrent(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/api/v2/transfer/info":
			_, _ = w.Write([]byte(`{"dl_info_speed":12400000,"up_info_speed":1100000,"dl_info_data":5,"connection_status":"connected"}`))
		case "/api/v2/torrents/info":
			if r.URL.Query().Get("filter") != "downloading" {
				t.Errorf("filter: %s", r.URL)
			}
			_, _ = w.Write([]byte(`[{"name":"nom-de-torrent-prive"},{"name":"autre"},{"name":"encore"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestQbittorrentReadsCountsAndSpeedsOnly(t *testing.T) {
	where := widgets.Target{Kind: "qbittorrent", URL: fakeQbittorrent(t).URL}
	p, state := widgets.TestKey(context.Background(), where, secrets.NewValue(testKey))
	if state != widgets.StateOK || p.Note != "3 téléchargements en cours" {
		t.Errorf("%s %+v", state, p)
	}
	if _, state := widgets.TestKey(context.Background(), where, secrets.NewValue("mauvaise")); state != widgets.StateAuth {
		t.Errorf("wrong key: %s", state)
	}
	data, state := widgets.Read(context.Background(), where, secrets.NewValue(testKey), nil, []string{widgets.WidgetTransfer})
	if state != widgets.StateOK {
		t.Fatal(state)
	}
	if strings.Contains(string(data[widgets.WidgetTransfer]), "nom-de-torrent-prive") {
		t.Error("a torrent name reached the stored data")
	}
	var got widgets.Transfer
	if err := json.Unmarshal(data[widgets.WidgetTransfer], &got); err != nil {
		t.Fatal(err)
	}
	if got != (widgets.Transfer{DownSpeed: 12400000, UpSpeed: 1100000, Downloading: 3, Connection: "connected"}) {
		t.Errorf("%+v", got)
	}
}

func TestTransferText(t *testing.T) {
	h, n := widgets.TransferText(widgets.Transfer{DownSpeed: 12400000, UpSpeed: 1100000, Downloading: 3, Connection: "connected"})
	if h != "12,4 Mo/s" || n != "3 en cours · envoi 1,1 Mo/s" {
		t.Errorf("%q %q", h, n)
	}
	h, n = widgets.TransferText(widgets.Transfer{UpSpeed: 850000, Connection: "disconnected"})
	if h != "0 o/s" || n != "0 en cours · envoi 850 Ko/s · déconnecté" {
		t.Errorf("%q %q", h, n)
	}
}
