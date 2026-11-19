package widgets

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"time"

	"hub/server/internal/secrets"
)

// Widgets of Request and Prowlarr.
const (
	WidgetRequests = "requests"
	WidgetIndexers = "indexers"
)

// Routes from Prowlarr's openapi.json. Its names stay out: how many, and how many fail.
type prowlarr struct{}

// Indexers counts the enabled indexers and those Prowlarr has set aside after failures.
type Indexers struct {
	Enabled int `json:"enabled"`
	Failing int `json:"failing"`
}

func (prowlarr) Manifest() Manifest {
	return Manifest{
		Kind:    "prowlarr",
		Name:    "Prowlarr",
		KeyHelp: "Dans Prowlarr : Settings, General, API Key.",
		Reads:   []string{"GET /api/v1/indexer", "GET /api/v1/indexerstatus", "POST puis WebSocket /signalr/messages : signaux de changement"},
		Widgets: []Widget{{ID: WidgetIndexers, Label: "Indexeurs", Where: "accueil et fiche"}},
	}
}

func (prowlarr) Authorize(req *http.Request, key secrets.Value) {
	req.Header.Set("X-Api-Key", key.Reveal())
}

func (prowlarr) read(ctx context.Context, c *Conn) (Indexers, error) {
	var indexers []struct {
		ID     int
		Enable bool
	}
	if err := c.GetJSON(ctx, "/api/v1/indexer", nil, &indexers); err != nil {
		return Indexers{}, err
	}
	var statuses []struct {
		IndexerID    int
		DisabledTill string
	}
	if err := c.GetJSON(ctx, "/api/v1/indexerstatus", nil, &statuses); err != nil {
		return Indexers{}, err
	}
	now := time.Now()
	out := Indexers{}
	for _, i := range indexers {
		if !i.Enable {
			continue
		}
		out.Enabled++
		if slices.ContainsFunc(statuses, func(s struct {
			IndexerID    int
			DisabledTill string
		}) bool {
			till, err := time.Parse(time.RFC3339, s.DisabledTill)
			return s.IndexerID == i.ID && err == nil && till.After(now)
		}) {
			out.Failing++
		}
	}
	return out, nil
}

func (p prowlarr) Probe(ctx context.Context, c *Conn) (Probe, error) {
	i, err := p.read(ctx, c)
	if err != nil {
		return Probe{}, err
	}
	return Probe{Note: indexersHeadline(i), Options: []Option{}}, nil
}

func (p prowlarr) Fetch(ctx context.Context, c *Conn, enabled []string) (map[string]any, error) {
	data := map[string]any{}
	if slices.Contains(enabled, WidgetIndexers) {
		i, err := p.read(ctx, c)
		if err != nil {
			return nil, err
		}
		data[WidgetIndexers] = i
	}
	return data, nil
}

func indexersHeadline(i Indexers) string {
	if i.Enabled == 1 {
		return "1 indexeur"
	}
	return strconv.Itoa(i.Enabled) + " indexeurs"
}

// Card is what a connection shows on the home page.
type Card struct {
	// Suffix follows the service's name; empty for the host's name.
	Suffix         string
	Headline, Note string
}

// CardsOf words the widgets that have a card on the home page. A value it cannot read is an
// error: the stored data is the connector's own.
func CardsOf(kind string, data map[string]json.RawMessage, enabled []string) ([]Card, error) {
	cards := []Card{}
	read := func(id string, v any) (bool, error) {
		raw, ok := data[id]
		if !ok || !slices.Contains(enabled, id) {
			return false, nil
		}
		return true, json.Unmarshal(raw, v)
	}
	var q Queue
	if ok, err := read(WidgetQueue, &q); err != nil {
		return nil, err
	} else if ok {
		h, n := QueueText(kind, q)
		cards = append(cards, Card{"file d’attente", h, n})
	}
	var t Transfer
	if ok, err := read(WidgetTransfer, &t); err != nil {
		return nil, err
	} else if ok {
		h, n := TransferText(t)
		cards = append(cards, Card{"", h, n})
	}
	var r Requests
	if ok, err := read(WidgetRequests, &r); err != nil {
		return nil, err
	} else if ok {
		cards = append(cards, Card{"demandes", requestsHeadline(r), issuesNote(r.OpenIssues)})
	}
	var i Indexers
	if ok, err := read(WidgetIndexers, &i); err != nil {
		return nil, err
	} else if ok {
		note := "Tous répondent"
		if i.Failing > 0 {
			note = strconv.Itoa(i.Failing) + " en panne"
		}
		cards = append(cards, Card{"indexeurs", indexersHeadline(i), note})
	}
	return cards, nil
}
