// Package widgets reads the services' own APIs, from the server. A connector only reads, a
// key never leaves the server, and what reaches the browser is what the connector kept.
// One failing connection never holds up another (WIDGETS.md).
package widgets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"hub/server/internal/secrets"
	"hub/server/internal/urlcheck"
)

// State is the outcome of the last read.
type State string

const (
	StateOK          State = "ok"
	StateAuth        State = "auth"        // the key is refused: no retry until it changes
	StateCert        State = "cert"        // the certificate is no longer the accepted one
	StateUnreachable State = "unreachable" // no answer, or a 5xx: retried, further apart
	StateUnexpected  State = "unexpected"  // an answer in a format the connector does not know
)

const (
	// maxBody bounds an answer; beyond it the JSON is cut and reads as unexpected.
	maxBody = 4 << 20
	// fetchTimeout bounds a whole read, several requests included.
	fetchTimeout = 20 * time.Second
	maxDelay     = 10 * time.Minute
	concurrency  = 4
)

// RefreshChoices are the rates the user can pick, in seconds.
var RefreshChoices = []int{30, 60, 300}

// Widget is what a connection can show.
type Widget struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Where says where it appears, for the connection panel.
	Where string `json:"where"`
}

// Manifest describes a connector to the connection panel.
type Manifest struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	KeyHelp string `json:"keyHelp"`
	// Reads lists the requests the connector makes: nothing else is ever sent.
	Reads   []string `json:"reads"`
	Widgets []Widget `json:"widgets"`
}

// Choice is one value of an Option.
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Option is a setting only the service itself can list, offered once the key works.
type Option struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Choices []Choice `json:"choices"`
}

// Probe is what a working key found.
type Probe struct {
	Note    string   `json:"note"`
	Options []Option `json:"options"`
}

// Connector reads one kind of service.
type Connector interface {
	Manifest() Manifest
	// Authorize puts the key on a request, in a header: never in the URL.
	Authorize(req *http.Request, key secrets.Value)
	Probe(ctx context.Context, c *Conn) (Probe, error)
	// Fetch reads the widgets asked for and keeps only what they show.
	Fetch(ctx context.Context, c *Conn, enabled []string) (map[string]any, error)
}

// Support says what Hub offers for an application the catalogue recognizes.
type Support string

const (
	SupportNone    Support = ""
	SupportReady   Support = "ready"
	SupportPlanned Support = "planned" // API found in its documentation, connector not written
	SupportLink    Support = "link"    // no confirmed API: a link and nothing more
)

var (
	connectors = map[string]Connector{"emby": emby{}, "sonarr": sonarr, "radarr": radarr, "qbittorrent": qbittorrent{}}
	planned    = map[string]bool{"prowlarr": true, "seerr": true, "jellyseerr": true, "beszel": true}
	linkOnly   = map[string]bool{"dockhand": true}
)

// SupportOf looks up an application by its catalogue slug.
func SupportOf(app string) Support {
	switch {
	case connectors[app] != nil:
		return SupportReady
	case planned[app]:
		return SupportPlanned
	case linkOnly[app]:
		return SupportLink
	}
	return SupportNone
}

// ManifestOf returns the connector's manifest, if it exists.
func ManifestOf(kind string) (Manifest, bool) {
	c, ok := connectors[kind]
	if !ok {
		return Manifest{}, false
	}
	return c.Manifest(), true
}

// Failure is an error that says which state it leads to.
type Failure struct{ State State }

func (f *Failure) Error() string { return "widget: " + string(f.State) }

// StateOf maps an error to a state. Anything unforeseen reads as unexpected.
func StateOf(err error) State {
	if err == nil {
		return StateOK
	}
	var f *Failure
	if errors.As(err, &f) {
		return f.State
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return StateUnreachable
	}
	return StateUnexpected
}

// Conn is a connection being read.
type Conn struct {
	// name is the service, for the logs.
	name        string
	base        string
	key         secrets.Value
	options     map[string]string
	connector   Connector
	client      *http.Client
	certRefused func() bool
}

// Option returns a setting of the connection, or "".
func (c *Conn) Option(id string) string { return c.options[id] }

// GetJSON requests a path under the service's address and decodes its answer.
func (c *Conn) GetJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.requestJSON(ctx, http.MethodGet, path, query, out)
}

// postJSON only serves to open a live connection: a connector's reads stay GET.
func (c *Conn) postJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.requestJSON(ctx, http.MethodPost, path, query, out)
}

func (c *Conn) requestJSON(ctx context.Context, method, path string, query url.Values, out any) error {
	target := strings.TrimRight(c.base, "/") + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return &Failure{StateUnexpected}
	}
	req.Header.Set("User-Agent", "Hub")
	req.Header.Set("Accept", "application/json")
	c.connector.Authorize(req, c.key)
	resp, err := c.client.Do(req)
	if err != nil {
		if c.certRefused() {
			return &Failure{StateCert}
		}
		return &Failure{StateUnreachable}
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return &Failure{StateAuth}
	case resp.StatusCode >= 500:
		return &Failure{StateUnreachable}
	case resp.StatusCode != http.StatusOK:
		return &Failure{StateUnexpected}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return &Failure{StateUnexpected}
	}
	return nil
}

// dialWS opens a WebSocket under the service's address, with the rules of its client.
func (c *Conn) dialWS(ctx context.Context, path string, query url.Values, header http.Header) (*websocket.Conn, error) {
	target := strings.TrimRight(c.base, "/") + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	conn, _, err := websocket.Dial(ctx, target, &websocket.DialOptions{HTTPClient: c.client, HTTPHeader: header})
	return conn, err
}

// Target is where a connection reads.
type Target struct {
	Kind   string
	URL    string
	Pinned string
}

func open(t Target, key secrets.Value, options map[string]string) (*Conn, error) {
	connector, ok := connectors[t.Kind]
	if !ok {
		return nil, fmt.Errorf("no connector for %q", t.Kind)
	}
	client, rejected, err := urlcheck.Client(t.URL, t.Pinned)
	if err != nil {
		return nil, &Failure{StateUnexpected}
	}
	return &Conn{base: t.URL, key: key, options: options, connector: connector, client: client, certRefused: rejected}, nil
}

// TestKey asks the service whether it accepts the key.
func TestKey(ctx context.Context, t Target, key secrets.Value) (Probe, State) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	c, err := open(t, key, nil)
	if err != nil {
		return Probe{}, StateOf(err)
	}
	p, err := c.connector.Probe(ctx, c)
	return p, StateOf(err)
}

// Read fetches the widgets asked for, as stored JSON.
func Read(ctx context.Context, t Target, key secrets.Value, options map[string]string, enabled []string) (map[string]json.RawMessage, State) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	c, err := open(t, key, options)
	if err != nil {
		return nil, StateOf(err)
	}
	found, err := c.connector.Fetch(ctx, c, enabled)
	if err != nil {
		return nil, StateOf(err)
	}
	data := make(map[string]json.RawMessage, len(found))
	for id, v := range found {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, StateUnexpected
		}
		data[id] = raw
	}
	return data, StateOK
}

// KeyName is the secret holding a service's key.
func KeyName(slug string) string { return "widget:" + slug + ":api_key" }

// KeyHint keeps enough of a key to recognize it. A short key shows nothing.
func KeyHint(key string) string {
	if len(key) < 12 {
		return ""
	}
	return key[len(key)-4:]
}

// NextDelay spaces out the reads of a connection that keeps failing.
func NextDelay(refresh time.Duration, failures int) time.Duration {
	delay := refresh
	for range failures {
		delay *= 2
		if delay >= maxDelay {
			return max(refresh, maxDelay)
		}
	}
	return delay
}

// Due is a connection to read now.
type Due struct {
	Target
	Slug       string
	Options    map[string]string
	Enabled    []string
	Generation int64
}

// Source is where Run finds connections and leaves what it read.
type Source interface {
	// LiveConnections lists those worth talking to now.
	LiveConnections(ctx context.Context) ([]Due, error)
	DueConnections(ctx context.Context, now time.Time) ([]Due, error)
	// RecordRead ignores a read of a connection saved again since it began.
	RecordRead(ctx context.Context, d Due, data map[string]json.RawMessage, state State, at time.Time) error
}

// Keys opens the stored keys.
type Keys interface {
	Get(ctx context.Context, name string) (secrets.Value, error)
}

// Listener is a connector the service can warn of a change, without waiting for the next read.
type Listener interface {
	// Listen stays connected until ctx ends, and calls wake when something changed.
	Listen(ctx context.Context, c *Conn, wake func())
	// WakeSpacing keeps a chatty service from turning its signals into constant reads.
	WakeSpacing() time.Duration
}

const (
	liveRetryMin     = 5 * time.Second
	liveRetryMax     = 5 * time.Minute
	liveWriteTimeout = 10 * time.Second
	liveMaxTypes     = 32
)

// LiveFirstMessage is how long a live connection may stay silent before its next way is tried.
var LiveFirstMessage = 15 * time.Second

var liveTypeName = regexp.MustCompile(`^[A-Za-z/]{1,40}$`)

// listenLoop tries each of the ways to connect until one answers, and holds it. attempt tells
// whether that way answered. Once none does, it waits longer and longer before starting over.
func listenLoop(ctx context.Context, ways int, attempt func(way int) bool) {
	way, delay := 0, liveRetryMin
	for ctx.Err() == nil {
		heard := attempt(way)
		switch {
		case ctx.Err() != nil:
			return
		case heard:
			delay = liveRetryMin
		case way < ways-1:
			way++
			continue // no wait: the next way may simply be the right one
		default:
			way = 0
			delay = min(delay*2, liveRetryMax)
		}
		select {
		case <-ctx.Done():
		case <-time.After(delay):
		}
	}
}

// logLiveType notes each kind of message once, its name only: production tells what a service
// really sends, and nothing of what it says.
func logLiveType(c *Conn, seen map[string]bool, name string) {
	if seen[name] || len(seen) >= liveMaxTypes || !liveTypeName.MatchString(name) {
		return
	}
	seen[name] = true
	slog.Info("widgets: live message type", "service", c.name, "type", name)
}

type engine struct {
	src       Source
	keys      Keys
	bus       *Bus
	limit     chan struct{}
	locks     sync.Map // slug -> *sync.Mutex: one read at a time per connection
	listening map[string]listening
}

type listening struct {
	generation int64
	stop       context.CancelFunc
}

// Run reads each connection when due or when it signals a change, until ctx ends. A slow one
// only holds its own slot. Every read is announced on bus.
func Run(ctx context.Context, src Source, keys Keys, bus *Bus, every time.Duration) {
	e := &engine{src: src, keys: keys, bus: bus, limit: make(chan struct{}, concurrency), listening: map[string]listening{}}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		live, err := src.LiveConnections(ctx)
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("widgets", "err", err)
			}
		} else {
			e.reconcile(ctx, live)
		}
		due, err := src.DueConnections(ctx, time.Now())
		if err != nil && ctx.Err() == nil {
			slog.Error("widgets", "err", err)
		}
		for _, d := range due {
			go e.read(ctx, d, false)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// reconcile keeps one listener per connection that can have one. A connection saved again
// gets a new one: its key or address may have changed.
func (e *engine) reconcile(ctx context.Context, live []Due) {
	wanted := map[string]bool{}
	for _, d := range live {
		if _, ok := connectors[d.Kind].(Listener); !ok {
			continue
		}
		wanted[d.Slug] = true
		if l, ok := e.listening[d.Slug]; ok {
			if l.generation == d.Generation {
				continue
			}
			l.stop()
			delete(e.listening, d.Slug)
		}
		key, err := e.keys.Get(ctx, KeyName(d.Slug))
		if err != nil {
			continue // the read says what is wrong with the key
		}
		c, err := open(d.Target, key, d.Options)
		if err != nil {
			continue
		}
		c.name = d.Slug
		lctx, stop := context.WithCancel(ctx)
		e.listening[d.Slug] = listening{generation: d.Generation, stop: stop}
		go e.listen(lctx, d, c)
	}
	for slug, l := range e.listening {
		if !wanted[slug] {
			l.stop()
			delete(e.listening, slug)
		}
	}
}

func (e *engine) listen(ctx context.Context, d Due, c *Conn) {
	woke := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-woke:
			}
			e.read(ctx, d, true)
			select {
			case <-ctx.Done():
				return
			case <-time.After(c.connector.(Listener).WakeSpacing()):
			}
		}
	}()
	c.connector.(Listener).Listen(ctx, c, func() {
		select {
		case woke <- struct{}{}:
		default:
		}
	})
}

// read skips a connection being read already, unless asked to wait: a signal says the read
// in progress may have missed the change.
func (e *engine) read(ctx context.Context, d Due, wait bool) {
	v, _ := e.locks.LoadOrStore(d.Slug, &sync.Mutex{})
	lock := v.(*sync.Mutex)
	if wait {
		lock.Lock()
	} else if !lock.TryLock() {
		return
	}
	defer lock.Unlock()
	select {
	case e.limit <- struct{}{}:
		defer func() { <-e.limit }()
	case <-ctx.Done():
		return
	}
	readOne(ctx, e.src, e.keys, d)
	e.bus.Publish(d.Slug)
}

func readOne(ctx context.Context, src Source, keys Keys, d Due) {
	key, err := keys.Get(ctx, KeyName(d.Slug))
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("widgets: key unavailable", "service", d.Slug, "err", err)
		}
		// A database hiccup is retried; a key that is gone (key file changed, secret deleted)
		// leaves the connection as good as refused.
		if !errors.Is(err, secrets.ErrNotFound) && !errors.Is(err, secrets.ErrUnknownKey) && !errors.Is(err, secrets.ErrDecrypt) {
			return
		}
		if err := src.RecordRead(ctx, d, nil, StateAuth, time.Now()); err != nil && ctx.Err() == nil {
			slog.Error("widgets", "service", d.Slug, "err", err)
		}
		return
	}
	data, state := Read(ctx, d.Target, key, d.Options, d.Enabled)
	if ctx.Err() != nil {
		return
	}
	if err := src.RecordRead(ctx, d, data, state, time.Now()); err != nil && ctx.Err() == nil {
		slog.Error("widgets", "service", d.Slug, "err", err)
	}
}
