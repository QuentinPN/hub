package widgets_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"hub/server/internal/widgets"
)

// fakeHub speaks SignalR's JSON protocol over a WebSocket, as Sonarr and Radarr's hub does.
type fakeHub struct {
	mu        sync.Mutex
	urls      []string
	send      chan string
	pings     chan struct{}
	needQuery bool
}

func (h *fakeHub) accepted(r *http.Request) bool {
	if h.needQuery {
		return r.URL.Query().Get("access_token") == testKey
	}
	return r.Header.Get("X-Api-Key") == testKey
}

func (h *fakeHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.urls = append(h.urls, r.Method+" "+r.URL.String())
	h.mu.Unlock()
	switch r.URL.Path {
	case "/signalr/messages/negotiate":
		if !h.accepted(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"connectionId":"abc","connectionToken":"tok","negotiateVersion":1}`))
	case "/signalr/messages":
		if !h.accepted(r) || r.URL.Query().Get("id") != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx := r.Context()
		_, hello, err := conn.Read(ctx)
		if err != nil || !strings.Contains(string(hello), `"protocol":"json"`) || !strings.HasSuffix(string(hello), "\x1e") {
			return
		}
		if conn.Write(ctx, websocket.MessageText, []byte("{}\x1e")) != nil {
			return
		}
		go func() {
			for {
				_, data, err := conn.Read(ctx)
				if err != nil {
					return
				}
				if strings.Contains(string(data), `"type":6`) {
					select {
					case h.pings <- struct{}{}:
					default:
					}
				}
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case text := <-h.send:
				if conn.Write(ctx, websocket.MessageText, []byte(text)) != nil {
					return
				}
			}
		}
	case "/api/v3/calendar":
		_, _ = w.Write([]byte(`[]`))
	case "/api/v3/queue":
		_, _ = w.Write([]byte(`{"totalRecords":0,"records":[]}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func hubMessage(names ...string) string {
	var out strings.Builder
	for _, name := range names {
		raw, _ := json.Marshal(map[string]any{"type": 1, "target": "receiveMessage",
			"arguments": []any{map[string]any{"name": name, "body": map[string]any{"title": "ne-doit-pas-sortir"}}}})
		out.Write(raw)
		out.WriteByte(0x1e)
	}
	return out.String()
}

func runHub(t *testing.T, h *fakeHub, kind string) *memSource {
	t.Helper()
	h.send, h.pings = make(chan string, 16), make(chan struct{}, 4)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	d := widgets.Due{Slug: kind, Target: widgets.Target{Kind: kind, URL: srv.URL}, Enabled: []string{widgets.WidgetCalendar, widgets.WidgetQueue}, Generation: 1}
	src := &memSource{reads: map[string]widgets.State{}, done: make(chan string, 16), live: []widgets.Due{d}}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go widgets.Run(ctx, src, memKeys{}, widgets.NewBus(), time.Hour)
	return src
}

func TestHubMessagesWakeAReadOnlyWhenTheyMatter(t *testing.T) {
	for _, kind := range []string{"sonarr", "radarr"} {
		t.Run(kind, func(t *testing.T) {
			h := &fakeHub{}
			src := runHub(t, h, kind)

			h.send <- hubMessage("version", "health", "command")
			noRead(t, src, 300*time.Millisecond)

			h.send <- hubMessage("queue/status")
			waitRead(t, src)

			// Several records in one frame, and an unknown shape in passing.
			h.send <- "{\"type\":6}\x1e{oops}\x1e" + hubMessage("calendar")
			waitRead(t, src)
			src.mu.Lock()
			got := src.reads[kind]
			src.mu.Unlock()
			if got != widgets.StateOK {
				t.Errorf("read: %s", got)
			}

			h.mu.Lock()
			defer h.mu.Unlock()
			for _, u := range h.urls {
				if strings.Contains(u, testKey) {
					t.Errorf("key in the address while the header works: %s", u)
				}
			}
		})
	}
}

func TestHubFallsBackToTheAddressLikeTheirOwnPage(t *testing.T) {
	h := &fakeHub{needQuery: true}
	src := runHub(t, h, "radarr")
	h.send <- hubMessage("movie")
	waitRead(t, src)
	h.mu.Lock()
	defer h.mu.Unlock()
	found := false
	for _, u := range h.urls {
		found = found || strings.Contains(u, "access_token=")
	}
	if !found {
		t.Errorf("never tried the address: %v", h.urls)
	}
}

func TestHubAbsentLeavesTheRegularReads(t *testing.T) {
	srv := fakeArr(t, "[]")
	d := widgets.Due{Slug: "sonarr", Target: widgets.Target{Kind: "sonarr", URL: srv.URL}, Enabled: []string{widgets.WidgetQueue}, Generation: 1}
	src := &memSource{reads: map[string]widgets.State{}, done: make(chan string, 16), live: []widgets.Due{d}, due: []widgets.Due{d}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go widgets.Run(ctx, src, memKeys{}, widgets.NewBus(), time.Hour)
	waitRead(t, src)
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.reads["sonarr"] != widgets.StateOK {
		t.Errorf("%v", src.reads)
	}
}
