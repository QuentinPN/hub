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

// Set once: engines of earlier tests may still be reading them as they stop.
func init() {
	widgets.LiveFirstMessage = 500 * time.Millisecond
	widgets.ArrWakeSpacing = 20 * time.Millisecond
	widgets.HubWakeSpacing = 100 * time.Millisecond
}

const sessionsMsg = `{"MessageType":"Sessions","Data":[{"Id":"s1","NowPlayingItem":{"Id":"film"},"PlayState":{"IsPaused":%PAUSED%,"PositionTicks":%POS%}}]}`

func sessions(paused, pos string) string {
	return strings.NewReplacer("%PAUSED%", paused, "%POS%", pos).Replace(sessionsMsg)
}

// liveEmby is fakeEmby with a WebSocket on the path and in the way the test asks.
type liveEmby struct {
	mu        sync.Mutex
	paths     []string
	send      chan string
	received  chan string
	acceptAt  string
	needQuery bool
	silent    map[string]bool
}

func (l *liveEmby) handler(t *testing.T, rest http.Handler) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			rest.ServeHTTP(w, r)
			return
		}
		inQuery := r.URL.Query().Get("api_key") == testKey
		inHeader := r.Header.Get("X-Emby-Token") == testKey
		l.mu.Lock()
		l.paths = append(l.paths, r.URL.Path+"|query="+map[bool]string{true: "yes", false: "no"}[inQuery])
		silent := l.silent[r.URL.Path]
		l.mu.Unlock()
		if r.URL.Path != l.acceptAt && !silent {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if l.needQuery && !inQuery || !l.needQuery && !inHeader && !inQuery {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx := r.Context()
		go func() {
			for {
				_, data, err := conn.Read(ctx)
				if err != nil {
					return
				}
				select {
				case l.received <- string(data):
				default:
				}
			}
		}()
		if silent {
			<-ctx.Done()
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case text := <-l.send:
				if err := conn.Write(ctx, websocket.MessageText, []byte(text)); err != nil {
					return
				}
			}
		}
	})
}

func newLive(t *testing.T, l *liveEmby) *httptest.Server {
	t.Helper()
	l.send, l.received = make(chan string, 16), make(chan string, 16)
	srv := httptest.NewServer(l.handler(t, fakeEmby(t).Config.Handler))
	t.Cleanup(srv.Close)
	return srv
}

func runLive(t *testing.T, srv *httptest.Server) (*memSource, *widgets.Bus) {
	t.Helper()
	d := widgets.Due{Slug: "emby", Target: target(srv), Enabled: []string{widgets.EmbySessions}, Generation: 1}
	src := &memSource{reads: map[string]widgets.State{}, done: make(chan string, 16), live: []widgets.Due{d}}
	bus := widgets.NewBus()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go widgets.Run(ctx, src, memKeys{}, bus, time.Hour)
	return src, bus
}

func waitRead(t *testing.T, src *memSource) {
	t.Helper()
	select {
	case <-src.done:
	case <-time.After(8 * time.Second):
		t.Fatal("no read after a signal")
	}
}

func noRead(t *testing.T, src *memSource, during time.Duration) {
	t.Helper()
	select {
	case <-src.done:
		t.Fatal("read without a change")
	case <-time.After(during):
	}
}

func TestSignalsWakeAReadAndAnnounceIt(t *testing.T) {
	l := &liveEmby{acceptAt: "/"}
	srv := newLive(t, l)
	src, bus := runLive(t, srv)
	signals, stop := bus.Subscribe()
	defer stop()

	select {
	case msg := <-l.received:
		if !strings.Contains(msg, "SessionsStart") {
			t.Errorf("first message: %s", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("never subscribed to the sessions")
	}

	l.send <- sessions("false", "100")
	waitRead(t, src)
	select {
	case slug := <-signals:
		if slug != "emby" {
			t.Errorf("announced %q", slug)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read not announced")
	}

	// Only the position moves: nothing to tell.
	l.send <- sessions("false", "200")
	noRead(t, src, 500*time.Millisecond)

	// Pausing is a change, read after the spacing.
	l.send <- sessions("true", "200")
	waitRead(t, src)

	l.send <- `{"MessageType":"LibraryChanged","Data":{"ItemsAdded":["1"]}}`
	waitRead(t, src)

	// The key stayed out of the URL: the header was enough here.
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.paths) != 1 || l.paths[0] != "/|query=no" {
		t.Errorf("attempts: %v", l.paths)
	}
}

func TestListenTriesTheOtherRoutes(t *testing.T) {
	l := &liveEmby{acceptAt: "/embywebsocket", needQuery: true}
	srv := newLive(t, l)
	src, _ := runLive(t, srv)
	l.send <- `{"MessageType":"LibraryChanged"}`
	waitRead(t, src)
	l.mu.Lock()
	defer l.mu.Unlock()
	want := []string{"/|query=no", "/|query=yes", "/embywebsocket|query=no", "/embywebsocket|query=yes"}
	if strings.Join(l.paths, " ") != strings.Join(want, " ") {
		t.Errorf("attempts: %v", l.paths)
	}
}

func TestSilentRouteIsLeftForTheNext(t *testing.T) {
	l := &liveEmby{acceptAt: "/embywebsocket", silent: map[string]bool{"/": true}}
	srv := newLive(t, l)
	src, _ := runLive(t, srv)
	l.send <- `{"MessageType":"LibraryChanged"}`
	waitRead(t, src)
}

func TestListenerStopsWithItsConnection(t *testing.T) {
	var mu sync.Mutex
	open := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		mu.Lock()
		open++
		mu.Unlock()
		defer func() {
			mu.Lock()
			open--
			mu.Unlock()
		}()
		for {
			if _, _, err := conn.Read(r.Context()); err != nil {
				break
			}
		}
		_ = conn.CloseNow()
	}))
	defer srv.Close()
	d := widgets.Due{Slug: "emby", Target: target(srv), Generation: 1}
	src := &stoppable{memSource: memSource{reads: map[string]widgets.State{}, done: make(chan string, 16), live: []widgets.Due{d}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go widgets.Run(ctx, src, memKeys{}, widgets.NewBus(), 20*time.Millisecond)
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return open == 1 })
	src.setLive(nil)
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return open == 0 })
}

type stoppable struct{ memSource }

func (s *stoppable) setLive(list []widgets.Due) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.live = list
}

func (s *stoppable) LiveConnections(context.Context) ([]widgets.Due, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.live, nil
}

func (s *stoppable) RecordRead(context.Context, widgets.Due, map[string]json.RawMessage, widgets.State, time.Time) error {
	return nil
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("condition never met")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestBusEndsItsSubscribersOnClose(t *testing.T) {
	bus := widgets.NewBus()
	signals, stop := bus.Subscribe()
	defer stop()
	bus.Publish("emby")
	if got := <-signals; got != "emby" {
		t.Error(got)
	}
	bus.Close()
	if _, ok := <-signals; ok {
		t.Error("still open after Close")
	}
	late, lateStop := bus.Subscribe()
	defer lateStop()
	if _, ok := <-late; ok {
		t.Error("subscribed to a closed bus")
	}
}
