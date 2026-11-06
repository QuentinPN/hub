package widgets

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"

	"hub/server/internal/secrets"
)

// WidgetSystems is the widget of a Beszel connection.
const WidgetSystems = "systems"

// HostOptionPrefix starts the option that ties a Hub host to a Beszel system by its name.
const HostOptionPrefix = "host:"

// Beszel is read through PocketBase, as beszel.dev/guide/rest-api says. It warns that the
// data may change in minor releases: a change ends as StateUnexpected. It has no API key: a
// read-only account signs in, and its token is kept in memory only.
type beszel struct{}

// System is what the widget shows of a Beszel system, percentages rounded.
type System struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	CPU    int    `json:"cpu"`
	Memory int    `json:"memory"`
	Disk   int    `json:"disk"`
}

// JoinCredentials makes the one sealed value of an account. The identity holds no line break:
// API.validKey refuses control characters.
func JoinCredentials(identity, password string) string { return identity + "\n" + password }

func (beszel) Manifest() Manifest {
	return Manifest{
		Kind:     "beszel",
		Name:     "Beszel",
		Auth:     AuthPassword,
		KeyHelp:  "Crée dans Beszel un compte au rôle readonly, réservé à Hub, puis partage-lui tes systèmes.",
		MapHosts: true,
		Reads:    []string{"POST /api/collections/users/auth-with-password", "GET /api/collections/systems/records"},
		Widgets:  []Widget{{ID: WidgetSystems, Label: "CPU, mémoire et disque", Where: "accueil, Infra et fiche"}},
	}
}

// Authorize adds nothing: the token comes from Conn.token, once signed in.
func (beszel) Authorize(*http.Request, secrets.Value) {}

// Tokens by address and sealed value: a changed password never reuses an old one.
var beszelTokens sync.Map

func beszelCacheKey(c *Conn) string { return c.base + "\x00" + c.key.Reveal() }

func (beszel) login(ctx context.Context, c *Conn) error {
	identity, password, ok := strings.Cut(c.key.Reveal(), "\n")
	if !ok {
		return &Failure{StateAuth}
	}
	body, err := json.Marshal(map[string]string{"identity": identity, "password": password})
	if err != nil {
		return &Failure{StateUnexpected}
	}
	target := strings.TrimRight(c.base, "/") + "/api/collections/users/auth-with-password"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return &Failure{StateUnexpected}
	}
	req.Header.Set("User-Agent", "Hub")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if c.certRefused() {
			return &Failure{StateCert}
		}
		return &Failure{StateUnreachable}
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	// PocketBase answers a wrong password with a 400.
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return &Failure{StateAuth}
	case resp.StatusCode >= 500:
		return &Failure{StateUnreachable}
	case resp.StatusCode != http.StatusOK:
		return &Failure{StateUnexpected}
	}
	var out struct{ Token string }
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&out); err != nil || out.Token == "" {
		return &Failure{StateUnexpected}
	}
	c.token = out.Token
	beszelTokens.Store(beszelCacheKey(c), out.Token)
	return nil
}

func (b beszel) systems(ctx context.Context, c *Conn) ([]System, error) {
	if t, ok := beszelTokens.Load(beszelCacheKey(c)); ok {
		c.token = t.(string)
	} else if err := b.login(ctx, c); err != nil {
		return nil, err
	}
	var page struct {
		Items *[]struct {
			Name   string
			Status string
			Info   struct {
				CPU float64 `json:"cpu"`
				MP  float64 `json:"mp"`
				DP  float64 `json:"dp"`
			}
		}
	}
	query := url.Values{"perPage": {"200"}, "fields": {"name,status,info"}}
	err := c.GetJSON(ctx, "/api/collections/systems/records", query, &page)
	if StateOf(err) == StateAuth {
		// The token expired: one new sign-in tells it from a refused account.
		beszelTokens.Delete(beszelCacheKey(c))
		if err := b.login(ctx, c); err != nil {
			return nil, err
		}
		err = c.GetJSON(ctx, "/api/collections/systems/records", query, &page)
	}
	if err != nil {
		return nil, err
	}
	if page.Items == nil {
		return nil, &Failure{StateUnexpected}
	}
	list := make([]System, 0, len(*page.Items))
	for _, s := range *page.Items {
		list = append(list, System{Name: s.Name, Status: s.Status, CPU: percent(s.Info.CPU), Memory: percent(s.Info.MP), Disk: percent(s.Info.DP)})
	}
	return list, nil
}

func percent(v float64) int { return int(min(100, max(0, v)) + 0.5) }

func (b beszel) Probe(ctx context.Context, c *Conn) (Probe, error) {
	list, err := b.systems(ctx, c)
	if err != nil {
		return Probe{}, err
	}
	p := Probe{Options: []Option{}, Systems: make([]string, 0, len(list))}
	for _, s := range list {
		p.Systems = append(p.Systems, s.Name)
	}
	switch len(list) {
	case 0:
		p.Note = "Compte accepté, mais aucun système visible : partage-les avec ce compte"
	case 1:
		p.Note = "1 système visible"
	default:
		p.Note = strconv.Itoa(len(list)) + " systèmes visibles"
	}
	return p, nil
}

func (b beszel) Fetch(ctx context.Context, c *Conn, enabled []string) (map[string]any, error) {
	data := map[string]any{}
	if slices.Contains(enabled, WidgetSystems) {
		list, err := b.systems(ctx, c)
		if err != nil {
			return nil, err
		}
		data[WidgetSystems] = list
	}
	return data, nil
}

// MatchSystem finds the system named like a host: "mini-PC" and "minipc" are the same.
func MatchSystem(systems []string, names ...string) string {
	norm := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, strings.ToLower(s))
	}
	for _, sys := range systems {
		for _, name := range names {
			if n := norm(name); n != "" && norm(sys) == n {
				return sys
			}
		}
	}
	return ""
}
