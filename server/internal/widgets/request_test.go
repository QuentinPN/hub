package widgets_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"hub/server/internal/secrets"
	"hub/server/internal/widgets"
)

// Answers shaped like Request's own schemas (src/shared), with what must not travel.
const (
	hubCountsJSON      = `{"pendingApprovals":3,"mineActive":2}`
	hubIssueCountsJSON = `{"open":1}`
	hubPendingJSON     = `{"page":1,"totalPages":2,"totals":{"pending":23,"approved":4,"declined":0,"cancelled":0,"sent":1,"failed":1,"available":9},"results":[
		{"id":12,"type":"movie","tmdbId":603,"status":"pending","seasons":[],"requester":{"id":"u1","name":"Alice","avatarUrl":"/secret-avatar"},
		 "createdBy":null,"is4k":false,"createdAt":"2026-11-18T10:00:00.000Z","updatedAt":"2026-11-18T10:00:00.000Z",
		 "media":{"tmdbId":603,"type":"movie","title":"Matrix","year":1999,"posterPath":"/abc.jpg","backdropPath":null,"overview":"ne-doit-pas-sortir"},
		 "failureReason":null,"declineReason":null},
		{"id":13,"type":"tv","tmdbId":99,"status":"pending","seasons":[1,2],"requester":{"id":"u2","name":"Bob","avatarUrl":null},
		 "is4k":true,"createdAt":"2026-11-18T11:00:00.000Z","updatedAt":"2026-11-18T11:00:00.000Z","media":null,"failureReason":null,"declineReason":null}]}`
	hubRecentJSON = `{"page":1,"totalPages":1,"totals":{},"results":[
		{"id":14,"type":"movie","tmdbId":604,"status":"declined","seasons":[],"requester":{"id":"u1","name":"Alice","avatarUrl":null},
		 "is4k":false,"createdAt":"2026-11-18T12:00:00.000Z","updatedAt":"2026-11-18T13:00:00.000Z",
		 "media":{"tmdbId":604,"type":"movie","title":"Reloaded","year":null,"posterPath":"/../../etc/passwd.jpg"},"failureReason":null,"declineReason":"Déjà là"},
		{"id":15,"type":"tv","tmdbId":605,"status":"failed","seasons":[1],"requester":{"id":"u1","name":"Alice","avatarUrl":null},
		 "is4k":false,"createdAt":"2026-11-18T11:00:00.000Z","updatedAt":"2026-11-18T14:00:00.000Z","media":null,"failureReason":"Sonarr injoignable","declineReason":null},
		{"id":12,"type":"movie","tmdbId":603,"status":"pending","seasons":[],"requester":{"id":"u1","name":"Alice","avatarUrl":null},
		 "is4k":false,"createdAt":"2026-11-18T10:00:00.000Z","updatedAt":"2026-11-18T10:00:00.000Z","media":null,"failureReason":null,"declineReason":null}]}`
	hubIssuesJSON = `{"page":1,"totalPages":1,"totals":{"open":7,"resolved":2},"results":[
		{"id":4,"type":"video","status":"open","media":{"type":"tv","tmdbId":99},"seasonNumber":1,"episodeNumber":3,"message":"ne-doit-pas-sortir",
		 "author":{"id":"u2","name":"Bob","avatarUrl":null},"resolvedBy":null,"createdAt":"2026-11-17T10:00:00.000Z","updatedAt":"2026-11-17T10:00:00.000Z",
		 "resolvedAt":null,"title":"Une série","posterPath":"/def.png"},
		{"id":5,"type":"audio","status":"open","media":{"type":"movie","tmdbId":603},"seasonNumber":null,"episodeNumber":null,"message":"x",
		 "author":null,"resolvedBy":null,"createdAt":"2026-11-16T10:00:00.000Z","updatedAt":"2026-11-16T10:00:00.000Z","resolvedAt":null,
		 "title":null,"posterPath":null}]}`
	hubStatsJSON = `{"requests":{"total":120,"byStatus":{"pending":3},"byMonth":[{"month":"2026-10","count":8},{"month":"2026-11","count":5}],
		"byType":{"movie":80,"tv":40},"byQuality":{"standard":110,"fourK":10}},
		"topMembers":[{"id":"a","name":"A","avatarUrl":null,"count":9},{"id":"b","name":"B","avatarUrl":null,"count":8},{"id":"c","name":"C","avatarUrl":null,"count":7},
			{"id":"d","name":"D","avatarUrl":null,"count":6},{"id":"e","name":"E","avatarUrl":null,"count":5},{"id":"f","name":"F","avatarUrl":null,"count":4}],
		"topTitles":[],"delays":{"approval":{"samples":3,"medianSeconds":3600.4,"meanSeconds":4000},"availability":{"samples":0,"medianSeconds":null,"meanSeconds":null}},
		"issues":{"open":1,"resolved":0},"members":4,"watchlist":{"entries":0,"titles":0}}`
	hubSearchJSON = `{"page":1,"totalPages":3,"people":[{"id":1,"name":"Une personne","profilePath":null,"knownFor":[]}],"results":[
		{"tmdbId":603,"type":"movie","title":"Matrix","year":1999,"posterPath":"/abc.jpg","backdropPath":null,"overview":"","status":"available"},
		{"tmdbId":99,"type":"tv","title":"Une série","year":null,"posterPath":null,"backdropPath":null,"overview":""}` +
		`,{"tmdbId":1,"type":"movie","title":"a","year":1,"posterPath":null,"backdropPath":null,"overview":""}` +
		`,{"tmdbId":2,"type":"movie","title":"b","year":1,"posterPath":null,"backdropPath":null,"overview":""}` +
		`,{"tmdbId":3,"type":"movie","title":"c","year":1,"posterPath":null,"backdropPath":null,"overview":""}` +
		`,{"tmdbId":4,"type":"movie","title":"d","year":1,"posterPath":null,"backdropPath":null,"overview":""}` +
		`,{"tmdbId":5,"type":"movie","title":"e","year":1,"posterPath":null,"backdropPath":null,"overview":""}` +
		`,{"tmdbId":6,"type":"movie","title":"f","year":1,"posterPath":null,"backdropPath":null,"overview":""}` +
		`,{"tmdbId":7,"type":"movie","title":"g","year":1,"posterPath":null,"backdropPath":null,"overview":""}` +
		`,{"tmdbId":8,"type":"movie","title":"h","year":1,"posterPath":null,"backdropPath":null,"overview":""}` +
		`,{"tmdbId":9,"type":"movie","title":"i","year":1,"posterPath":null,"backdropPath":null,"overview":""}]}`
)

// sent is a write the fake received.
type sent struct {
	method, path, contentType, body string
}

// fakeRequest answers like Request: a key only, never with the cookie session's header, and
// a JSON body for every write (Fastify refuses an empty one).
type fakeRequest struct {
	mu     sync.Mutex
	writes []sent
	reads  []string
	// refuse answers a write with this status and message, when set.
	refuse  int
	message string
	// scope is the key's scope /api/auth/me names; empty: an older Request, which names none.
	scope string
}

func (f *fakeRequest) refuseWith(status int, message string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refuse, f.message = status, message
}

// seen returns the writes and reads so far.
func (f *fakeRequest) seen() ([]sent, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.writes), slices.Clone(f.reads)
}

func (f *fakeRequest) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Header.Get("X-Api-Key") != testKey:
			w.WriteHeader(http.StatusUnauthorized)
			return
		case r.Header.Get("X-Request") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "":
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			f.writes = append(f.writes, sent{r.Method, r.URL.Path, r.Header.Get("Content-Type"), string(body)})
			switch {
			case len(body) == 0:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"Body cannot be empty when content-type is set to 'application/json'"}`))
			case f.refuse != 0:
				w.WriteHeader(f.refuse)
				_, _ = w.Write([]byte(`{"error":"` + f.message + `"}`))
			case r.URL.Path == "/api/requests":
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":20}`))
			default:
				_, _ = w.Write([]byte(`{"id":12}`))
			}
			return
		}
		f.reads = append(f.reads, r.URL.Path+"?"+r.URL.RawQuery)
		me := `{"viewer":{"id":"u1","email":"ne-doit-pas-sortir"}`
		if f.scope != "" {
			me += `,"apiKeyScope":"` + f.scope + `"`
		}
		routes := map[string]string{
			"/api/auth/me":           me + `}`,
			"/api/requests/counts":   hubCountsJSON,
			"/api/issues/counts":     hubIssueCountsJSON,
			"/api/requests":          hubRecentJSON,
			"/api/issues":            hubIssuesJSON,
			"/api/stats":             hubStatsJSON,
			"/api/images/w342/a.jpg": string(pngBytes),
		}
		if r.URL.Path == "/api/requests" && r.URL.Query().Get("status") == "pending" {
			routes["/api/requests"] = hubPendingJSON
		}
		if r.URL.Path == "/api/search" {
			if r.URL.Query().Get("q") != "matrix" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			routes["/api/search"] = hubSearchJSON
		}
		body, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

var hubAll = []string{widgets.WidgetRequests, widgets.HubPending, widgets.HubRecent, widgets.HubIssues, widgets.HubStats}

func hubTarget(srv *httptest.Server) widgets.Target {
	return widgets.Target{Kind: "request", URL: srv.URL}
}

func TestRequestChecksTheKey(t *testing.T) {
	where := hubTarget((&fakeRequest{}).server(t))
	p, state := widgets.TestKey(context.Background(), where, secrets.NewValue(testKey))
	if state != widgets.StateOK || p.Note != "3 en attente" {
		t.Errorf("%s %+v", state, p)
	}
	if _, state := widgets.TestKey(context.Background(), where, secrets.NewValue("mauvaise")); state != widgets.StateAuth {
		t.Errorf("wrong key: %s", state)
	}
	m, ok := widgets.ManifestOf("request")
	if !ok || !m.Webhook || !m.Writes || m.DefaultRefresh != 60 || widgets.SupportOf("request") != widgets.SupportReady {
		t.Errorf("%+v", m)
	}
}

func decodeWidget[T any](t *testing.T, data map[string]json.RawMessage, id string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(data[id], &v); err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	return v
}

func TestRequestReadKeepsOnlyWhatTheWidgetsShow(t *testing.T) {
	data, state := widgets.Read(context.Background(), hubTarget((&fakeRequest{}).server(t)), secrets.NewValue(testKey), nil, hubAll)
	if state != widgets.StateOK {
		t.Fatal(state)
	}
	for id, raw := range data {
		for _, private := range []string{"ne-doit-pas-sortir", "secret-avatar", "passwd"} {
			if strings.Contains(string(raw), private) {
				t.Errorf("%s carries %q: %s", id, private, raw)
			}
		}
	}

	r := decodeWidget[widgets.Requests](t, data, widgets.WidgetRequests)
	if r.PendingApprovals == nil || *r.PendingApprovals != 3 || r.MineActive != 2 || r.OpenIssues != 1 || !r.CanAct {
		t.Errorf("requests: %+v", r)
	}

	// The total covers every page; a title TMDB could not give still has a name.
	if raw := string(data[widgets.HubPending]); !strings.Contains(raw, `"seasons":[]`) || strings.Contains(raw, `"year":null`) {
		t.Errorf("pending JSON: %s", raw)
	}
	p := decodeWidget[widgets.PendingList](t, data, widgets.HubPending)
	if p.Total != 23 || len(p.Items) != 2 {
		t.Fatalf("pending: %+v", p)
	}
	matrix, unknown := p.Items[0], p.Items[1]
	if matrix.ID != 12 || matrix.MediaType != "movie" || matrix.TmdbID != 603 || matrix.Title != "Matrix" || matrix.Year == nil || *matrix.Year != 1999 ||
		matrix.Poster != "abc.jpg" || matrix.RequestedBy != "Alice" || matrix.Is4K || len(matrix.Seasons) != 0 || matrix.CreatedAt != "2026-11-18T10:00:00.000Z" {
		t.Errorf("pending: %+v", matrix)
	}
	if unknown.Title != "TMDB 99" || unknown.Year != nil || unknown.Poster != "" || !unknown.Is4K || len(unknown.Seasons) != 2 || unknown.RequestedBy != "Bob" {
		t.Errorf("pending without media: %+v", unknown)
	}

	// Request's own order is kept: createdAt, then id, newest first.
	rec := decodeWidget[widgets.RecentList](t, data, widgets.HubRecent)
	if len(rec.Items) != 3 || rec.Items[0].ID != 14 || rec.Items[1].ID != 15 || rec.Items[2].ID != 12 {
		t.Fatalf("recent: %+v", rec)
	}
	if d := rec.Items[0]; d.Status != "declined" || d.Reason != "Déjà là" || d.Poster != "" || d.Year != nil || d.UpdatedAt != "2026-11-18T13:00:00.000Z" {
		t.Errorf("declined: %+v", d)
	}
	if f := rec.Items[1]; f.Status != "failed" || f.Reason != "Sonarr injoignable" || f.Title != "TMDB 605" {
		t.Errorf("failed: %+v", f)
	}
	if !strings.Contains(string(data[widgets.HubRecent]), `"id":12,"mediaType":"movie","tmdbId":603,"title":"TMDB 603","requestedBy":"Alice","status":"pending","updatedAt"`) {
		t.Errorf("no reason must leave the field out: %s", data[widgets.HubRecent])
	}

	is := decodeWidget[widgets.IssueList](t, data, widgets.HubIssues)
	if is.Total != 7 || len(is.Items) != 2 {
		t.Fatalf("issues: %+v", is)
	}
	if i := is.Items[0]; i.ID != 4 || i.Type != "video" || i.Title != "Une série" || i.MediaType != "tv" || i.Poster != "def.png" ||
		i.CreatedBy == nil || *i.CreatedBy != "Bob" || i.Season == nil || *i.Season != 1 || i.Episode == nil || *i.Episode != 3 {
		t.Errorf("issue: %+v", i)
	}
	if i := is.Items[1]; i.Title != "TMDB 603" || i.CreatedBy != nil || i.Season != nil || i.Poster != "" {
		t.Errorf("issue without author: %+v", i)
	}
	if raw := string(data[widgets.HubIssues]); !strings.Contains(raw, `"createdBy":null`) || strings.Contains(raw, `"season":null`) {
		t.Errorf("issues JSON: %s", raw)
	}

	s := decodeWidget[widgets.RequestStats](t, data, widgets.HubStats)
	if s.Total != 120 || len(s.ByMonth) != 2 || s.ByMonth[1] != (widgets.MonthCount{Month: "2026-11", Count: 5}) || s.ByType.Movie != 80 || s.ByType.TV != 40 ||
		s.ByQuality.Standard != 110 || s.ByQuality.FourK != 10 || s.MedianApprovalSeconds == nil || *s.MedianApprovalSeconds != 3600 ||
		s.MedianAvailableSeconds != nil || len(s.TopMembers) != 5 || s.TopMembers[0] != (widgets.MemberCount{Name: "A", Count: 9}) {
		t.Errorf("stats: %+v", s)
	}
}

func TestRequestReadsOnlyTheWidgetsAsked(t *testing.T) {
	f := &fakeRequest{}
	data, state := widgets.Read(context.Background(), hubTarget(f.server(t)), secrets.NewValue(testKey), nil, []string{widgets.HubIssues})
	if state != widgets.StateOK || len(data) != 1 {
		t.Fatalf("%s %v", state, data)
	}
	if _, reads := f.seen(); len(reads) != 1 || reads[0] != "/api/issues?status=open" {
		t.Errorf("%v", reads)
	}
}

// Only a key limited to reading cannot act; an older Request names no scope, and its keys acted.
func TestRequestKeyScopeSaysWhetherHubCanAct(t *testing.T) {
	for scope, want := range map[string]bool{"read": false, "requests": true, "full": true, "": true} {
		f := &fakeRequest{scope: scope}
		data, state := widgets.Read(context.Background(), hubTarget(f.server(t)), secrets.NewValue(testKey), nil, []string{widgets.WidgetRequests})
		if state != widgets.StateOK {
			t.Fatalf("%q: %s", scope, state)
		}
		if r := decodeWidget[widgets.Requests](t, data, widgets.WidgetRequests); r.CanAct != want {
			t.Errorf("scope %q: canAct %v, want %v", scope, r.CanAct, want)
		}
	}
}

func TestRequestCard(t *testing.T) {
	for _, c := range []struct{ data, headline, note string }{
		{`{"pendingApprovals":3,"mineActive":2,"openIssues":1}`, "3 en attente", "1 signalement ouvert"},
		{`{"pendingApprovals":0,"mineActive":2,"openIssues":4}`, "Aucune demande en attente", "4 signalements ouverts"},
		{`{"pendingApprovals":null,"mineActive":2,"openIssues":0}`, "2 en cours", "Aucun signalement"},
	} {
		cards, err := widgets.CardsOf("request", map[string]json.RawMessage{widgets.WidgetRequests: json.RawMessage(c.data)}, []string{widgets.WidgetRequests})
		if err != nil || len(cards) != 1 {
			t.Fatalf("%+v %v", cards, err)
		}
		if got := cards[0]; got.Suffix != "demandes" || got.Headline != c.headline || got.Note != c.note {
			t.Errorf("%s: %+v", c.data, got)
		}
	}
}

func TestRequestActionsSendWhatRequestExpects(t *testing.T) {
	f := &fakeRequest{}
	where, key, ctx := hubTarget(f.server(t)), secrets.NewValue(testKey), context.Background()
	if err := widgets.Approve(ctx, where, key, 12); err != nil {
		t.Fatal(err)
	}
	if err := widgets.Decline(ctx, where, key, 13, "Déjà là"); err != nil {
		t.Fatal(err)
	}
	if err := widgets.Request(ctx, where, key, widgets.MediaMovie, 603); err != nil {
		t.Fatal(err)
	}
	if err := widgets.Request(ctx, where, key, widgets.MediaTV, 99); err != nil {
		t.Fatal(err)
	}
	want := []sent{
		{"POST", "/api/requests/12/approve", "application/json", `{}`},
		{"POST", "/api/requests/13/decline", "application/json", `{"reason":"Déjà là"}`},
		{"POST", "/api/requests", "application/json", `{"tmdbId":603,"type":"movie"}`},
		{"POST", "/api/requests", "application/json", `{"seasons":"all","tmdbId":99,"type":"tv"}`},
	}
	writes, _ := f.seen()
	if len(writes) != len(want) {
		t.Fatalf("%+v", writes)
	}
	for i, w := range want {
		if writes[i] != w {
			t.Errorf("write %d: %+v, want %+v", i, writes[i], w)
		}
	}

	found, err := widgets.Search(ctx, where, key, "matrix")
	if err != nil || len(found) != widgets.HubSearchMax {
		t.Fatalf("%v %+v", err, found)
	}
	m, s := found[0], found[1]
	if m.MediaType != "movie" || m.TmdbID != 603 || m.Title != "Matrix" || m.Year == nil || m.Poster != "abc.jpg" || m.Status == nil || *m.Status != "available" {
		t.Errorf("%+v", m)
	}
	if s.MediaType != "tv" || s.Year != nil || s.Poster != "" || s.Status != nil {
		t.Errorf("%+v", s)
	}
	raw, _ := json.Marshal(found)
	if strings.Contains(string(raw), "Une personne") || !strings.Contains(string(raw), `"status":null`) {
		t.Errorf("%s", raw)
	}
}

func TestRequestRefusalsKeepTheirStatusAndMessage(t *testing.T) {
	f := &fakeRequest{}
	f.refuseWith(http.StatusForbidden, "Droit requis")
	where, key, ctx := hubTarget(f.server(t)), secrets.NewValue(testKey), context.Background()
	var refusal *widgets.Refusal
	if err := widgets.Approve(ctx, where, key, 12); !errors.As(err, &refusal) || refusal.Status != http.StatusForbidden || refusal.Message != "Droit requis" {
		t.Errorf("%v %+v", err, refusal)
	}
	f.refuseWith(http.StatusConflict, `Déjà demandé\u0007\nsuite`)
	if err := widgets.Request(ctx, where, key, widgets.MediaMovie, 603); !errors.As(err, &refusal) || refusal.Status != http.StatusConflict || refusal.Message != "Déjà demandé  suite" {
		t.Errorf("%v %+v", err, refusal)
	}
	f.refuseWith(http.StatusBadGateway, "")
	if err := widgets.Approve(ctx, where, key, 12); widgets.StateOf(err) != widgets.StateUnreachable {
		t.Errorf("5xx: %v", err)
	}
	if err := widgets.Approve(ctx, where, secrets.NewValue("mauvaise"), 12); !errors.As(err, &refusal) || refusal.Status != http.StatusUnauthorized {
		t.Errorf("wrong key: %v", err)
	}
	if err := widgets.Approve(ctx, widgets.Target{Kind: "emby", URL: where.URL}, key, 12); !errors.Is(err, widgets.ErrNoActions) {
		t.Errorf("emby has no actions: %v", err)
	}
}

// A connection whose connector is gone (Seerr) is read as unexpected, never as a crash.
func TestAConnectorNoLongerInHubIsToleratedEverywhere(t *testing.T) {
	srv := (&fakeRequest{}).server(t)
	gone := widgets.Target{Kind: "seerr", URL: srv.URL}
	if widgets.SupportOf("seerr") != widgets.SupportNone || widgets.SupportOf("jellyseerr") != widgets.SupportNone {
		t.Error("seerr is still offered")
	}
	if _, ok := widgets.ManifestOf("seerr"); ok {
		t.Error("seerr still has a manifest")
	}
	if _, state := widgets.TestKey(context.Background(), gone, secrets.NewValue(testKey)); state != widgets.StateUnexpected {
		t.Errorf("test: %s", state)
	}
	if _, state := widgets.Read(context.Background(), gone, secrets.NewValue(testKey), nil, []string{widgets.WidgetRequests}); state != widgets.StateUnexpected {
		t.Errorf("read: %s", state)
	}
	if err := widgets.Approve(context.Background(), gone, secrets.NewValue(testKey), 1); !errors.Is(err, widgets.ErrNoActions) {
		t.Errorf("approve: %v", err)
	}

	// The engine records it and carries on with the others.
	src := &memSource{reads: map[string]widgets.State{}, done: make(chan string, 4), due: []widgets.Due{
		{Slug: "seerr", Target: gone, Enabled: []string{widgets.WidgetRequests}},
		{Slug: "hub", Target: hubTarget(srv), Enabled: []string{widgets.WidgetRequests}},
	}, live: []widgets.Due{{Slug: "seerr", Target: gone, Generation: 1}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go widgets.Run(ctx, src, memKeys{}, widgets.NewBus(), time.Hour)
	for range 2 {
		select {
		case <-src.done:
		case <-time.After(3 * time.Second):
			t.Fatal("no read")
		}
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.reads["seerr"] != widgets.StateUnexpected || src.reads["hub"] != widgets.StateOK {
		t.Errorf("%v", src.reads)
	}
}

// A write through Hub and the webhook that follows it make two reads at most.
func TestWakeReadsAtOnceAndCoalesces(t *testing.T) {
	srv := (&fakeRequest{}).server(t)
	d := widgets.Due{Slug: "hub-wake", Target: hubTarget(srv), Enabled: []string{widgets.WidgetRequests}, Generation: 1}
	src := &memSource{reads: map[string]widgets.State{}, done: make(chan string, 64), live: []widgets.Due{d}}
	bus := widgets.NewBus()
	signals, stop := bus.Subscribe()
	defer stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go widgets.Run(ctx, src, memKeys{}, bus, time.Hour)

	// The listener opens in the background: wake until it answers.
	deadline := time.After(3 * time.Second)
	for woke := false; !woke; {
		widgets.Wake(d.Slug)
		select {
		case <-src.done:
			woke = true
		case <-time.After(20 * time.Millisecond):
		case <-deadline:
			t.Fatal("Wake never read the connection")
		}
	}
	if got := <-signals; got != d.Slug {
		t.Errorf("bus: %s", got)
	}

	for range 10 {
		widgets.Wake(d.Slug)
	}
	reads := 0
	quiet := time.After(4 * widgets.HubWakeSpacing)
	for done := false; !done; {
		select {
		case <-src.done:
			reads++
		case <-quiet:
			done = true
		}
	}
	if reads != 1 {
		t.Errorf("10 wakes within the spacing made %d reads, want 1", reads)
	}
	widgets.Wake("nobody-listens") // no listener: nothing happens, nothing blocks
}
