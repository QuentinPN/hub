package widgets

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"
)

// The SignalR hub of Sonarr, Radarr and Prowlarr feeds their own web pages: /signalr/messages,
// message receiveMessage (SignalRConnector.js of Radarr and Prowlarr, Sonarr's MessageHub.cs). It is no public API
// and may change: it only ever wakes a read through the documented routes.
const (
	arrHubPath   = "/signalr/messages"
	arrSeparator = 0x1e // ends every SignalR record
	arrPing      = 15 * time.Second
)

// ArrWakeSpacing keeps a running download, which reports often, from causing constant reads.
var ArrWakeSpacing = 10 * time.Second

// What changes a widget. Radarr's page handles these names; on real servers both sent the
// queue ones, while the others have not been seen yet.
var arrWakingNames = map[string]bool{
	"calendar": true, "queue": true, "queue/status": true, "queue/details": true,
	"episode": true, "episodefile": true, "movie": true, "moviefile": true,
}

func (arr) WakeSpacing() time.Duration { return ArrWakeSpacing }

func (arr) Listen(ctx context.Context, c *Conn, wake func()) { listenHub(ctx, c, arrWakingNames, wake) }

// What changes Prowlarr's widget: handlers of its own SignalRConnector.js.
var prowlarrWakingNames = map[string]bool{"indexer": true, "indexerstatus": true, "health": true}

func (prowlarr) WakeSpacing() time.Duration { return ArrWakeSpacing }

func (prowlarr) Listen(ctx context.Context, c *Conn, wake func()) {
	listenHub(ctx, c, prowlarrWakingNames, wake)
}

// listenHub tries the key in a header, then in the address as their own page does.
func listenHub(ctx context.Context, c *Conn, names map[string]bool, wake func()) {
	seen := map[string]bool{}
	listenLoop(ctx, 2, func(route int) bool {
		return listenHubOnce(ctx, c, route == 1, seen, names, wake)
	})
}

// listenOnce holds one connection and tells whether the hub answered. Errors are not logged:
// they may quote an address holding the key.
func listenHubOnce(ctx context.Context, c *Conn, inQuery bool, seen, names map[string]bool, wake func()) bool {
	query := url.Values{"negotiateVersion": {"1"}}
	auth := "header"
	if inQuery {
		query.Set("access_token", c.key.Reveal())
		auth = "query"
	}
	var negotiated struct{ ConnectionToken, ConnectionID string }
	if err := c.postJSON(ctx, arrHubPath+"/negotiate", query, &negotiated); err != nil {
		return false
	}
	query.Del("negotiateVersion")
	query.Set("id", negotiated.ConnectionToken)
	if negotiated.ConnectionToken == "" {
		query.Set("id", negotiated.ConnectionID)
	}
	header := http.Header{"X-Api-Key": {c.key.Reveal()}}
	conn, err := c.dialWS(ctx, arrHubPath, query, header)
	if err != nil {
		return false
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(maxBody)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if !sendText(ctx, conn, "{\"protocol\":\"json\",\"version\":1}\x1e") {
		return false
	}
	first, stop := context.WithTimeout(ctx, LiveFirstMessage)
	_, reply, err := conn.Read(first)
	stop()
	if err != nil || bytes.Contains(reply, []byte(`"error"`)) {
		return false
	}
	slog.Info("widgets: live connection open", "service", c.name, "path", arrHubPath, "auth", auth)

	go func() {
		ticker := time.NewTicker(arrPing)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !sendText(ctx, conn, "{\"type\":6}\x1e") {
					cancel()
					return
				}
			}
		}
	}()

	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() == nil {
				slog.Info("widgets: live connection lost", "service", c.name)
			}
			return true
		}
		for _, record := range bytes.Split(raw, []byte{arrSeparator}) {
			if len(record) == 0 {
				continue
			}
			var msg struct {
				Type      int
				Target    string
				Arguments []struct{ Name string }
			}
			if json.Unmarshal(record, &msg) != nil {
				continue
			}
			if msg.Type == 7 { // the hub closes
				return true
			}
			if msg.Type != 1 || msg.Target != "receiveMessage" {
				continue
			}
			for _, a := range msg.Arguments {
				logLiveType(c, seen, a.Name)
				if names[a.Name] {
					wake()
				}
			}
		}
	}
}

func sendText(ctx context.Context, conn *websocket.Conn, text string) bool {
	ctx, cancel := context.WithTimeout(ctx, liveWriteTimeout)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, []byte(text)) == nil
}
