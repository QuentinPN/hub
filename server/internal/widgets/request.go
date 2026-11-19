package widgets

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"hub/server/internal/secrets"
)

// Widgets of Request, besides WidgetRequests.
const (
	HubPending = "pending"
	HubRecent  = "recent"
	HubIssues  = "issues"
	HubStats   = "stats"

	// hubTopMembers is how many of its most active members the stats keep.
	hubTopMembers = 5
	// HubSearchMax is how many titles a search keeps.
	HubSearchMax = 10
)

// HubWakeSpacing coalesces a write and the webhook that follows it into two reads at most.
var HubWakeSpacing = 5 * time.Second

// Routes and formats from Request's own source (docs/API.md, src/shared). The key has the
// scope its owner gave it: Hub reads every demand, and writes only what the user asks for here,
// which a "read" key cannot.
// No X-Request header: that one is for cookie sessions, and mixing them is refused.
type request struct{}

// hubPosterRef is a TMDB file name, as Request's image proxy accepts it: no separator, so
// neither a path nor another address can be named.
var hubPosterRef = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}\.(jpg|jpeg|png|webp)$`)

func (request) Manifest() Manifest {
	return Manifest{
		Kind:    "request",
		Name:    "Request",
		KeyHelp: "Dans Request : Réglages › API, Créer une clé avec la portée « Demandes » (lire, approuver, refuser, demander), réservée à Hub. « Lecture » suffit si tu ne veux qu'afficher.",
		Reads: []string{
			"GET /api/auth/me", "GET /api/requests/counts", "GET /api/issues/counts", "GET /api/requests", "GET /api/issues",
			"GET /api/stats", "GET /api/images/w342/… : affiches", "GET /api/search : quand tu cherches un titre depuis Hub",
			"POST /api/requests/…/approve et /decline : quand tu approuves ou refuses depuis Hub",
			"POST /api/requests : quand tu demandes un titre depuis Hub",
		},
		Widgets: []Widget{
			{ID: WidgetRequests, Label: "Demandes", Where: "accueil et fiche"},
			{ID: HubPending, Label: "À approuver", Where: "page Médias et fiche"},
			{ID: HubRecent, Label: "Demandes récentes", Where: "page Médias et fiche"},
			{ID: HubIssues, Label: "Signalements", Where: "fiche"},
			{ID: HubStats, Label: "Statistiques", Where: "fiche"},
		},
		DefaultRefresh: 60,
		Webhook:        true,
		Writes:         true,
	}
}

func (request) Authorize(req *http.Request, key secrets.Value) {
	req.Header.Set("X-Api-Key", key.Reveal())
}

// The webhook is its only signal: the listener holds the connection's wake for it.
func (request) WakeSpacing() time.Duration { return HubWakeSpacing }

func (request) Listen(ctx context.Context, c *Conn, wake func()) { holdWake(ctx, c.name, wake) }

// Route of src/server/routes/images.ts; w342 is the size of its own posters.
func (request) PosterRequest(ref string) (string, url.Values, bool) {
	if !hubPosterRef.MatchString(ref) {
		return "", nil, false
	}
	return "/api/images/w342/" + ref, nil, true
}

// Requests counts what waits for the owner. PendingApprovals is null for a key that cannot
// approve. CanAct is false only for a key limited to reading.
type Requests struct {
	PendingApprovals *int `json:"pendingApprovals"`
	MineActive       int  `json:"mineActive"`
	OpenIssues       int  `json:"openIssues"`
	CanAct           bool `json:"canAct"`
}

// RequestItem is what a demand shows in any list.
type RequestItem struct {
	ID          int    `json:"id"`
	MediaType   string `json:"mediaType"`
	TmdbID      int    `json:"tmdbId"`
	Title       string `json:"title"`
	Year        *int   `json:"year,omitempty"`
	Poster      string `json:"poster,omitempty"`
	RequestedBy string `json:"requestedBy"`
}

// PendingRequest waits for an approval.
type PendingRequest struct {
	RequestItem
	Is4K      bool   `json:"is4k"`
	Seasons   []int  `json:"seasons"`
	CreatedAt string `json:"createdAt"`
}

// PendingList is the first page of the demands to approve.
type PendingList struct {
	Total int              `json:"total"`
	Items []PendingRequest `json:"items"`
}

// RecentRequest is a demand in any state. Reason is why it failed or was declined.
type RecentRequest struct {
	RequestItem
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	UpdatedAt string `json:"updatedAt"`
}

// RecentList is the first page of the demands, newest first.
type RecentList struct {
	Items []RecentRequest `json:"items"`
}

// Issue is a problem reported on a title. CreatedBy is null once its author is gone.
type Issue struct {
	ID        int     `json:"id"`
	Type      string  `json:"type"`
	Title     string  `json:"title"`
	MediaType string  `json:"mediaType"`
	Poster    string  `json:"poster,omitempty"`
	CreatedBy *string `json:"createdBy"`
	Season    *int    `json:"season,omitempty"`
	Episode   *int    `json:"episode,omitempty"`
	CreatedAt string  `json:"createdAt"`
}

// IssueList is the first page of the open issues.
type IssueList struct {
	Total int     `json:"total"`
	Items []Issue `json:"items"`
}

// MonthCount is a month (AAAA-MM, UTC) of demands.
type MonthCount struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

// MemberCount is a member and how many demands they made.
type MemberCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// RequestStats are Request's own statistics, what the fiche shows of them.
type RequestStats struct {
	Total   int          `json:"total"`
	ByMonth []MonthCount `json:"byMonth"`
	ByType  struct {
		Movie int `json:"movie"`
		TV    int `json:"tv"`
	} `json:"byType"`
	ByQuality struct {
		Standard int `json:"standard"`
		FourK    int `json:"fourK"`
	} `json:"byQuality"`
	MedianAvailableSeconds *int          `json:"medianAvailableSeconds"`
	MedianApprovalSeconds  *int          `json:"medianApprovalSeconds"`
	TopMembers             []MemberCount `json:"topMembers"`
}

// Found is a title a search found. Status is null when it is neither in the library nor asked.
type Found struct {
	MediaType string  `json:"mediaType"`
	TmdbID    int     `json:"tmdbId"`
	Title     string  `json:"title"`
	Year      *int    `json:"year,omitempty"`
	Poster    string  `json:"poster,omitempty"`
	Status    *string `json:"status"`
}

// What is read of Request's answers (requestViewSchema, issueViewSchema, statsSchema…).
type (
	hubMedia struct {
		TmdbID     int
		Type       string
		Title      string
		Year       *int
		PosterPath *string
		Status     *string
	}
	hubRequest struct {
		ID            int
		Type          string
		TmdbID        int
		Status        string
		Seasons       []int
		Requester     struct{ Name string }
		Is4k          bool
		CreatedAt     string
		UpdatedAt     string
		Media         *hubMedia
		FailureReason *string
		DeclineReason *string
	}
	hubRequestPage struct {
		Results []hubRequest
		Totals  map[string]int
	}
	hubIssue struct {
		ID         int
		Type       string
		Title      *string
		PosterPath *string
		Media      struct {
			Type   string
			TmdbID int
		}
		Author        *struct{ Name string }
		SeasonNumber  *int
		EpisodeNumber *int
		CreatedAt     string
	}
	hubCounts struct {
		PendingApprovals *int
		MineActive       int
	}
)

// hubReadScope is the API key scope that opens GETs only (src/shared/api-keys.ts).
const hubReadScope = "read"

// canAct is whether the key may approve, decline or ask: /api/auth/me names its scope. An older
// Request sends none, and its keys acted as their owner.
func (request) canAct(ctx context.Context, c *Conn) (bool, error) {
	var me struct{ APIKeyScope string }
	if err := c.GetJSON(ctx, "/api/auth/me", nil, &me); err != nil {
		return false, err
	}
	return me.APIKeyScope != hubReadScope, nil
}

func (u request) counts(ctx context.Context, c *Conn) (Requests, error) {
	canAct, err := u.canAct(ctx, c)
	if err != nil {
		return Requests{}, err
	}
	var counts hubCounts
	if err := c.GetJSON(ctx, "/api/requests/counts", nil, &counts); err != nil {
		return Requests{}, err
	}
	var issues struct{ Open int }
	if err := c.GetJSON(ctx, "/api/issues/counts", nil, &issues); err != nil {
		return Requests{}, err
	}
	return Requests{PendingApprovals: counts.PendingApprovals, MineActive: counts.MineActive, OpenIssues: issues.Open, CanAct: canAct}, nil
}

func (u request) Probe(ctx context.Context, c *Conn) (Probe, error) {
	r, err := u.counts(ctx, c)
	if err != nil {
		return Probe{}, err
	}
	return Probe{Note: requestsHeadline(r), Options: []Option{}}, nil
}

func (u request) Fetch(ctx context.Context, c *Conn, enabled []string) (map[string]any, error) {
	data := map[string]any{}
	reads := []struct {
		id   string
		read func() (any, error)
	}{
		{WidgetRequests, func() (any, error) { return u.counts(ctx, c) }},
		{HubPending, func() (any, error) { return u.pending(ctx, c) }},
		{HubRecent, func() (any, error) { return u.recent(ctx, c) }},
		{HubIssues, func() (any, error) { return u.issues(ctx, c) }},
		{HubStats, func() (any, error) { return u.stats(ctx, c) }},
	}
	for _, r := range reads {
		if !slices.Contains(enabled, r.id) {
			continue
		}
		v, err := r.read()
		if err != nil {
			return nil, err
		}
		data[r.id] = v
	}
	return data, nil
}

// The upstream page size is fixed (20) and takes no sort: page 1 is what fits a widget.
func (request) requestPage(ctx context.Context, c *Conn, query url.Values) (hubRequestPage, error) {
	var page hubRequestPage
	return page, c.GetJSON(ctx, "/api/requests", query, &page)
}

// total is the demands to approve across all pages, from the page's own totals.
func (u request) pending(ctx context.Context, c *Conn) (PendingList, error) {
	page, err := u.requestPage(ctx, c, url.Values{"status": {"pending"}})
	if err != nil {
		return PendingList{}, err
	}
	out := PendingList{Total: page.Totals["pending"], Items: []PendingRequest{}}
	for _, r := range page.Results {
		seasons := r.Seasons
		if seasons == nil {
			seasons = []int{}
		}
		out.Items = append(out.Items, PendingRequest{RequestItem: r.item(), Is4K: r.Is4k, Seasons: seasons, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// Request lists the demands by createdAt, then id, newest first.
func (u request) recent(ctx context.Context, c *Conn) (RecentList, error) {
	page, err := u.requestPage(ctx, c, nil)
	if err != nil {
		return RecentList{}, err
	}
	out := RecentList{Items: []RecentRequest{}}
	for _, r := range page.Results {
		reason := ""
		for _, why := range []*string{r.FailureReason, r.DeclineReason} {
			if why != nil && reason == "" {
				reason = *why
			}
		}
		out.Items = append(out.Items, RecentRequest{RequestItem: r.item(), Status: r.Status, Reason: reason, UpdatedAt: r.UpdatedAt})
	}
	return out, nil
}

func (r hubRequest) item() RequestItem {
	item := RequestItem{ID: r.ID, MediaType: r.Type, TmdbID: r.TmdbID, Title: tmdbTitle(nil, r.TmdbID), RequestedBy: r.Requester.Name}
	if m := r.Media; m != nil {
		item.Title, item.Year, item.Poster = tmdbTitle(&m.Title, r.TmdbID), m.Year, posterRef(m.PosterPath)
	}
	return item
}

// tmdbTitle stands in for a title TMDB could not give.
func tmdbTitle(title *string, tmdbID int) string {
	if title == nil || *title == "" {
		return "TMDB " + strconv.Itoa(tmdbID)
	}
	return *title
}

// posterRef turns a TMDB path ("/abc.jpg") into the reference of /api/v1/posters, or "".
func posterRef(path *string) string {
	if path == nil {
		return ""
	}
	ref := strings.TrimPrefix(*path, "/")
	if !hubPosterRef.MatchString(ref) {
		return ""
	}
	return ref
}

func (request) issues(ctx context.Context, c *Conn) (IssueList, error) {
	var page struct {
		Results []hubIssue
		Totals  struct{ Open int }
	}
	if err := c.GetJSON(ctx, "/api/issues", url.Values{"status": {"open"}}, &page); err != nil {
		return IssueList{}, err
	}
	out := IssueList{Total: page.Totals.Open, Items: []Issue{}}
	for _, i := range page.Results {
		issue := Issue{ID: i.ID, Type: i.Type, Title: tmdbTitle(i.Title, i.Media.TmdbID), MediaType: i.Media.Type, Poster: posterRef(i.PosterPath),
			Season: i.SeasonNumber, Episode: i.EpisodeNumber, CreatedAt: i.CreatedAt}
		if i.Author != nil {
			issue.CreatedBy = &i.Author.Name
		}
		out.Items = append(out.Items, issue)
	}
	return out, nil
}

func (request) stats(ctx context.Context, c *Conn) (RequestStats, error) {
	type delay struct{ MedianSeconds *float64 }
	var s struct {
		Requests struct {
			Total     int
			ByMonth   []MonthCount
			ByType    map[string]int
			ByQuality struct{ Standard, FourK int }
		}
		TopMembers []MemberCount
		Delays     struct{ Approval, Availability delay }
	}
	if err := c.GetJSON(ctx, "/api/stats", nil, &s); err != nil {
		return RequestStats{}, err
	}
	seconds := func(d delay) *int {
		if d.MedianSeconds == nil {
			return nil
		}
		v := int(math.Round(*d.MedianSeconds))
		return &v
	}
	out := RequestStats{Total: s.Requests.Total, ByMonth: s.Requests.ByMonth, TopMembers: s.TopMembers[:min(len(s.TopMembers), hubTopMembers)],
		MedianAvailableSeconds: seconds(s.Delays.Availability), MedianApprovalSeconds: seconds(s.Delays.Approval)}
	if out.ByMonth == nil {
		out.ByMonth = []MonthCount{}
	}
	if out.TopMembers == nil {
		out.TopMembers = []MemberCount{}
	}
	out.ByType.Movie, out.ByType.TV = s.Requests.ByType["movie"], s.Requests.ByType["tv"]
	out.ByQuality.Standard, out.ByQuality.FourK = s.Requests.ByQuality.Standard, s.Requests.ByQuality.FourK
	return out, nil
}

func requestsHeadline(r Requests) string {
	switch {
	case r.PendingApprovals == nil:
		return strconv.Itoa(r.MineActive) + " en cours"
	case *r.PendingApprovals == 0:
		return "Aucune demande en attente"
	}
	return strconv.Itoa(*r.PendingApprovals) + " en attente"
}

func issuesNote(open int) string {
	switch open {
	case 0:
		return "Aucun signalement"
	case 1:
		return "1 signalement ouvert"
	}
	return strconv.Itoa(open) + " signalements ouverts"
}

// --- What the user asks for from Hub ---

// ErrNoActions is a service Hub cannot act on.
var ErrNoActions = errors.New("widget: no actions for this service")

// DeclineReasonMax is Request's own limit for a decline's reason.
const DeclineReasonMax = 500

// Media types of a demand.
const (
	MediaMovie = "movie"
	MediaTV    = "tv"
)

// requester is a connector that acts on demands.
type requester interface {
	approve(ctx context.Context, c *Conn, id int) error
	decline(ctx context.Context, c *Conn, id int, reason string) error
	request(ctx context.Context, c *Conn, mediaType string, tmdbID int) error
	search(ctx context.Context, c *Conn, q string) ([]Found, error)
}

// act runs one action on the connection, within the time a read is given.
func act(ctx context.Context, t Target, key secrets.Value, do func(context.Context, requester, *Conn) error) error {
	r, ok := connectors[t.Kind].(requester)
	if !ok {
		return ErrNoActions
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	c, err := open(t, key, nil)
	if err != nil {
		return err
	}
	return do(ctx, r, c)
}

// Approve approves demand id.
func Approve(ctx context.Context, t Target, key secrets.Value, id int) error {
	return act(ctx, t, key, func(ctx context.Context, r requester, c *Conn) error { return r.approve(ctx, c, id) })
}

// Decline declines demand id, for reason.
func Decline(ctx context.Context, t Target, key secrets.Value, id int, reason string) error {
	return act(ctx, t, key, func(ctx context.Context, r requester, c *Conn) error { return r.decline(ctx, c, id, reason) })
}

// Request asks for a title: a film, or every season of a series.
func Request(ctx context.Context, t Target, key secrets.Value, mediaType string, tmdbID int) error {
	return act(ctx, t, key, func(ctx context.Context, r requester, c *Conn) error { return r.request(ctx, c, mediaType, tmdbID) })
}

// Search looks a title up, films and series only.
func Search(ctx context.Context, t Target, key secrets.Value, q string) ([]Found, error) {
	var found []Found
	err := act(ctx, t, key, func(ctx context.Context, r requester, c *Conn) error {
		var err error
		found, err = r.search(ctx, c, q)
		return err
	})
	return found, err
}

func hubRequestPath(id int, action string) string {
	return "/api/requests/" + strconv.Itoa(id) + "/" + action
}

// The body is never empty: Request refuses an empty JSON body.
func (request) approve(ctx context.Context, c *Conn, id int) error {
	return c.actJSON(ctx, http.MethodPost, hubRequestPath(id, "approve"), nil, struct{}{}, nil)
}

func (request) decline(ctx context.Context, c *Conn, id int, reason string) error {
	return c.actJSON(ctx, http.MethodPost, hubRequestPath(id, "decline"), nil, map[string]string{"reason": reason}, nil)
}

// A series asks for "all": every aired season still missing. is4k stays at its default, false.
func (request) request(ctx context.Context, c *Conn, mediaType string, tmdbID int) error {
	body := map[string]any{"type": mediaType, "tmdbId": tmdbID}
	if mediaType == MediaTV {
		body["seasons"] = "all"
	}
	return c.actJSON(ctx, http.MethodPost, "/api/requests", nil, body, nil)
}

func (request) search(ctx context.Context, c *Conn, q string) ([]Found, error) {
	var page struct{ Results []hubMedia }
	if err := c.actJSON(ctx, http.MethodGet, "/api/search", url.Values{"q": {q}, "page": {"1"}}, nil, &page); err != nil {
		return nil, err
	}
	found := []Found{}
	for _, m := range page.Results[:min(len(page.Results), HubSearchMax)] {
		found = append(found, Found{MediaType: m.Type, TmdbID: m.TmdbID, Title: tmdbTitle(&m.Title, m.TmdbID), Year: m.Year,
			Poster: posterRef(m.PosterPath), Status: m.Status})
	}
	return found, nil
}
