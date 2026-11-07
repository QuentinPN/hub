package pgstore_test

import (
	"slices"
	"testing"

	"hub/server/internal/api"
)

func rowIDs(rows []api.LayoutRow) []string {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids
}

func TestLayoutStartsInTheDefaultOrderAndSurvivesASave(t *testing.T) {
	e := sectionsEnv(t)
	l, err := e.store.HomeLayout(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rowIDs(l.Rows), api.DefaultRows) {
		t.Errorf("%v", rowIDs(l.Rows))
	}
	if err := e.store.SaveHomeLayout(e.ctx, api.NormalizeLayout([]api.LayoutRow{
		{ID: api.RowHosts, Visible: true},
		{ID: api.RowPlaying, Visible: false},
		{ID: api.RowDownloads, Visible: true, Excluded: []string{"prowlarr"}},
	})); err != nil {
		t.Fatal(err)
	}
	l, err = e.store.HomeLayout(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The band stays first, the rows left unsaid come after.
	want := []string{api.RowPlaying, api.RowHosts, api.RowDownloads, api.RowFavorites, api.RowLatest, api.RowUpcoming}
	if !slices.Equal(rowIDs(l.Rows), want) {
		t.Errorf("%v", rowIDs(l.Rows))
	}
	if l.Rows[0].Visible || !slices.Equal(l.Rows[2].Excluded, []string{"prowlarr"}) {
		t.Errorf("%+v", l.Rows)
	}
	if err := e.store.ResetHomeLayout(e.ctx); err != nil {
		t.Fatal(err)
	}
	l, err = e.store.HomeLayout(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rowIDs(l.Rows), api.DefaultRows) || !l.Rows[0].Visible {
		t.Errorf("%+v", l.Rows)
	}
}

func TestChoicesNameWhatEachRowCouldShow(t *testing.T) {
	e := sectionsEnv(t)
	e.connect(t, "sonarr", "sonarr", []string{"calendar", "queue"}, map[string]string{
		"calendar": `[]`, "queue": `{"total":0,"downloading":0,"problems":0}`,
	})
	e.connect(t, "emby", "emby", []string{"sessions"}, map[string]string{"sessions": `[]`})
	l, err := e.store.HomeLayout(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	has := func(row, id string) bool {
		return slices.ContainsFunc(l.Choices[row], func(c api.LayoutChoice) bool { return c.ID == id })
	}
	if !has(api.RowPlaying, "emby") || !has(api.RowUpcoming, "sonarr") || !has(api.RowDownloads, "sonarr") {
		t.Errorf("%+v", l.Choices)
	}
	if has(api.RowDownloads, "emby") || has(api.RowUpcoming, "emby") {
		t.Errorf("emby has neither: %+v", l.Choices)
	}
	if !has(api.RowHosts, "mini") || !has(api.RowHosts, "qnap") {
		t.Errorf("%+v", l.Choices[api.RowHosts])
	}
}

func TestHomeShowsEmbyWithNothingPlaying(t *testing.T) {
	e := sectionsEnv(t)
	e.connect(t, "emby", "emby", []string{"sessions", "latest", "library"}, map[string]string{
		"sessions": `[]`,
		"latest":   `[{"title":"Un film","detail":"Film","kind":"movie","addedAt":"2030-01-08T00:00:00Z"},{"title":"Une série","detail":"S02E06","kind":"episode","addedAt":"2030-01-07T00:00:00Z"}]`,
		"library":  `{"movies":120,"series":30,"episodes":900}`,
	})
	home, err := e.store.Home(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if home.SessionsSlug != "emby" || len(home.Sessions) != 0 {
		t.Errorf("band: %q %+v", home.SessionsSlug, home.Sessions)
	}
	if home.LastAdded == nil || home.LastAdded.Title != "Un film" || home.Library == nil || home.Library.Movies != 120 {
		t.Errorf("idle band: %+v %+v", home.LastAdded, home.Library)
	}
	if home.LatestSlug != "emby" || len(home.Latest) != 2 || !slices.Contains(home.Rows, api.RowLatest) {
		t.Errorf("latest: %q %+v %v", home.LatestSlug, home.Latest, home.Rows)
	}
	l, err := e.store.HomeLayout(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Choices[api.RowLatest]) != 1 || l.Choices[api.RowLatest][0].ID != "emby" {
		t.Errorf("choices: %+v", l.Choices[api.RowLatest])
	}

	// Left out of its row, the shelf goes; the band keeps its own source.
	if err := e.store.SaveHomeLayout(e.ctx, api.NormalizeLayout([]api.LayoutRow{
		{ID: api.RowLatest, Visible: true, Excluded: []string{"emby"}},
	})); err != nil {
		t.Fatal(err)
	}
	home, err = e.store.Home(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(home.Latest) != 0 || home.LatestSlug != "" || home.LastAdded == nil {
		t.Errorf("excluded: %+v %q %+v", home.Latest, home.LatestSlug, home.LastAdded)
	}
}

func TestHomeFollowsTheLayout(t *testing.T) {
	e := sectionsEnv(t)
	e.connect(t, "sonarr", "sonarr", []string{"calendar", "queue"}, map[string]string{
		"calendar": `[{"title":"Une série","kind":"episode","at":"2030-01-08T00:00:00Z"}]`,
		"queue":    `{"total":1,"downloading":1,"problems":0}`,
	})
	e.connect(t, "radarr", "radarr", []string{"calendar", "queue"}, map[string]string{
		"calendar": `[{"title":"Un film","kind":"movie","at":"2030-01-09T00:00:00Z"}]`,
		"queue":    `{"total":1,"downloading":1,"problems":0}`,
	})
	e.connect(t, "emby", "emby", []string{"sessions"}, map[string]string{"sessions": `[{"title":"Le film"}]`})

	home, err := e.store.Home(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(home.Rows, api.DefaultRows) || len(home.Upcoming) != 2 || len(home.Downloads) != 2 || len(home.Sessions) != 1 || len(home.Hosts) != 2 {
		t.Fatalf("%+v", home)
	}

	if err := e.store.SaveHomeLayout(e.ctx, api.NormalizeLayout([]api.LayoutRow{
		{ID: api.RowDownloads, Visible: true, Excluded: []string{"radarr"}},
		{ID: api.RowUpcoming, Visible: true, Excluded: []string{"sonarr"}},
		{ID: api.RowHosts, Visible: true, Excluded: []string{"qnap"}},
		{ID: api.RowFavorites, Visible: false},
		{ID: api.RowPlaying, Visible: true, Excluded: []string{"emby"}},
	})); err != nil {
		t.Fatal(err)
	}
	home, err = e.store.Home(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	// A row the stored layout does not name yet comes at the end.
	want := []string{api.RowPlaying, api.RowDownloads, api.RowUpcoming, api.RowHosts, api.RowLatest}
	if !slices.Equal(home.Rows, want) {
		t.Errorf("rows: %v", home.Rows)
	}
	if len(home.Downloads) != 1 || home.Downloads[0].Slug != "sonarr" {
		t.Errorf("downloads: %+v", home.Downloads)
	}
	if len(home.Upcoming) != 1 || home.Upcoming[0].Title != "Un film" {
		t.Errorf("upcoming: %+v", home.Upcoming)
	}
	if len(home.Hosts) != 1 || home.Hosts[0].ID != "mini" {
		t.Errorf("hosts: %+v", home.Hosts)
	}
	if len(home.Sessions) != 0 {
		t.Errorf("sessions: %+v", home.Sessions)
	}

	// The other pages show everything: the layout is the home page's alone.
	page, err := e.store.DownloadsPage(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Cards) != 2 {
		t.Errorf("downloads page: %+v", page.Cards)
	}
}
