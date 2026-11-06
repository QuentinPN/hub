package widgets

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// Emby's WebSocket only ever wakes a read: what it says is not shown. Its page documents the
// address and the key as a query parameter (dev.emby.media/doc/restapi/Web-Socket.html). It
// does not cover SessionsStart, Sessions or LibraryChanged, nor /embywebsocket: Listen tries
// the routes in turn and logs the message types seen, so that production tells which hold.
var embyRoutes = []struct {
	path    string
	inQuery bool
}{
	{"/", false},
	{"/", true},
	{"/embywebsocket", false},
	{"/embywebsocket", true},
}

const (
	embyDeviceID       = "hub"
	embyKeepAlive      = 30 * time.Second
	embySessionsEvery  = "0,2000" // first answer at once, then every 2 s at most
	embyRetryMin       = 5 * time.Second
	embyRetryMax       = 5 * time.Minute
	embyWriteTimeout   = 10 * time.Second
	embyMaxMessage     = maxBody
	embyMaxLoggedTypes = 32
)

// EmbyFirstMessage is how long a route may stay silent before the next one is tried. The
// last route stays connected: it has nowhere to go.
var EmbyFirstMessage = 15 * time.Second

var embyTypeName = regexp.MustCompile(`^[A-Za-z]{1,40}$`)

// Listen stays connected until ctx ends, and calls wake when something changed.
func (e emby) Listen(ctx context.Context, c *Conn, wake func()) {
	route, delay := 0, embyRetryMin
	seen := map[string]bool{}
	for ctx.Err() == nil {
		heard := e.listenOnce(ctx, c, route, seen, wake)
		switch {
		case ctx.Err() != nil:
			return
		case heard:
			delay = embyRetryMin
		case route < len(embyRoutes)-1:
			route++
			continue // no wait: the next route may simply be the right one
		default:
			route = 0
			delay = min(delay*2, embyRetryMax)
		}
		select {
		case <-ctx.Done():
		case <-time.After(delay):
		}
	}
}

// listenOnce holds one connection and tells whether it heard anything. Errors are not
// logged: a dial error quotes its URL, which may hold the key.
func (e emby) listenOnce(ctx context.Context, c *Conn, route int, seen map[string]bool, wake func()) bool {
	r := embyRoutes[route]
	query := url.Values{"deviceId": {embyDeviceID}}
	header := http.Header{}
	auth := "header"
	if r.inQuery {
		query.Set("api_key", c.key.Reveal())
		auth = "query"
	} else {
		header.Set("X-Emby-Token", c.key.Reveal())
	}
	conn, err := c.dialWS(ctx, r.path, query, header)
	if err != nil {
		return false
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(embyMaxMessage)
	slog.Info("widgets: live connection open", "service", c.name, "path", r.path, "auth", auth)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if !embySend(ctx, conn, `{"MessageType":"SessionsStart","Data":"`+embySessionsEvery+`"}`) {
		return false
	}
	go func() {
		ticker := time.NewTicker(embyKeepAlive)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !embySend(ctx, conn, `{"MessageType":"KeepAlive"}`) {
					cancel()
					return
				}
			}
		}
	}()

	heard := false
	last := ""
	for {
		readCtx := ctx
		stop := func() {}
		if !heard && route < len(embyRoutes)-1 {
			readCtx, stop = context.WithTimeout(ctx, EmbyFirstMessage)
		}
		_, raw, err := conn.Read(readCtx)
		stop()
		if err != nil {
			if heard && ctx.Err() == nil {
				slog.Info("widgets: live connection lost", "service", c.name)
			}
			return heard
		}
		heard = true
		var msg struct {
			MessageType string
			Data        json.RawMessage
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if !seen[msg.MessageType] && len(seen) < embyMaxLoggedTypes && embyTypeName.MatchString(msg.MessageType) {
			seen[msg.MessageType] = true
			slog.Info("widgets: live message type", "service", c.name, "type", msg.MessageType)
		}
		switch msg.MessageType {
		case "Sessions":
			if sig := embyPlaybacks(msg.Data); sig != last {
				last = sig
				wake()
			}
		case "LibraryChanged", "UserDataChanged", "PlaybackStart", "PlaybackStopped":
			wake()
		}
	}
}

func embySend(ctx context.Context, conn *websocket.Conn, text string) bool {
	ctx, cancel := context.WithTimeout(ctx, embyWriteTimeout)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, []byte(text)) == nil
}

// embyPlaybacks sums up what plays: who, what and whether it is paused. The position moves
// all the time and is left to the regular reads.
func embyPlaybacks(data json.RawMessage) string {
	var sessions []struct {
		ID             string
		NowPlayingItem *struct{ ID string }
		PlayState      struct{ IsPaused bool }
	}
	if err := json.Unmarshal(data, &sessions); err != nil {
		return ""
	}
	parts := []string{}
	for _, s := range sessions {
		if s.NowPlayingItem == nil {
			continue
		}
		state := "playing"
		if s.PlayState.IsPaused {
			state = "paused"
		}
		parts = append(parts, s.ID+"/"+s.NowPlayingItem.ID+"/"+state)
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}
