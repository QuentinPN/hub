package api

import (
	"context"
	"net/http"
	"slices"
)

// Rows of the home page. RowPlaying is the band at the top: it can be hidden, not moved.
const (
	RowPlaying   = "playing"
	RowFavorites = "favorites"
	RowLatest    = "latest"
	RowUpcoming  = "upcoming"
	RowDownloads = "downloads"
	RowHosts     = "hosts"
)

// DefaultRows is the order of a home page nobody has arranged.
var DefaultRows = []string{RowPlaying, RowFavorites, RowLatest, RowUpcoming, RowDownloads, RowHosts}

const (
	maxExcluded = 64
	maxSourceID = 128
)

// LayoutRow is a row as the user arranged it. Excluded names the sources left out, by service
// slug or host id: those stay valid through a rescan.
type LayoutRow struct {
	ID       string   `json:"id"`
	Visible  bool     `json:"visible"`
	Excluded []string `json:"excluded"`
}

// LayoutChoice is a source a row can show or leave out.
type LayoutChoice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// HomeLayout is what the "Personnaliser" screen reads: the rows, and what each could show.
type HomeLayout struct {
	Rows    []LayoutRow               `json:"rows"`
	Choices map[string][]LayoutChoice `json:"choices"`
}

// LayoutEditor is implemented by stores that keep the home page's arrangement.
type LayoutEditor interface {
	HomeLayout(ctx context.Context) (HomeLayout, error)
	SaveHomeLayout(ctx context.Context, rows []LayoutRow) error
	ResetHomeLayout(ctx context.Context) error
}

// NormalizeLayout gives any stored or received arrangement the shape the pages expect: every
// known row once, the top band first, a row added by a later version in its default place at
// the end. Unknown rows drop out.
func NormalizeLayout(rows []LayoutRow) []LayoutRow {
	out := make([]LayoutRow, 0, len(DefaultRows))
	playing := LayoutRow{ID: RowPlaying, Visible: true, Excluded: []string{}}
	seen := map[string]bool{}
	for _, r := range rows {
		if !slices.Contains(DefaultRows, r.ID) || seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		if r.Excluded == nil {
			r.Excluded = []string{}
		}
		if r.ID == RowPlaying {
			playing = r
			continue
		}
		out = append(out, r)
	}
	for _, id := range DefaultRows {
		if !seen[id] && id != RowPlaying {
			out = append(out, LayoutRow{ID: id, Visible: true, Excluded: []string{}})
		}
	}
	return append([]LayoutRow{playing}, out...)
}

func validLayout(rows []LayoutRow) bool {
	seen := map[string]bool{}
	for _, r := range rows {
		if !slices.Contains(DefaultRows, r.ID) || seen[r.ID] || len(r.Excluded) > maxExcluded {
			return false
		}
		seen[r.ID] = true
		for _, id := range r.Excluded {
			if id == "" || len(id) > maxSourceID {
				return false
			}
		}
	}
	return true
}

func registerLayout(mux *http.ServeMux, editor LayoutEditor) {
	mux.HandleFunc("GET /api/v1/home-layout", func(w http.ResponseWriter, r *http.Request) {
		respond(w, r)(editor.HomeLayout(r.Context()))
	})
	mux.HandleFunc("PUT /api/v1/home-layout", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Rows []LayoutRow }
		if !decodeJSON(w, r, &body) {
			return
		}
		if !validLayout(body.Rows) {
			writeError(w, http.StatusBadRequest, "disposition invalide")
			return
		}
		if err := editor.SaveHomeLayout(r.Context(), NormalizeLayout(body.Rows)); err != nil {
			internalError(w, r, err)
			return
		}
		respond(w, r)(editor.HomeLayout(r.Context()))
	})
	mux.HandleFunc("DELETE /api/v1/home-layout", func(w http.ResponseWriter, r *http.Request) {
		if err := editor.ResetHomeLayout(r.Context()); err != nil {
			internalError(w, r, err)
			return
		}
		respond(w, r)(editor.HomeLayout(r.Context()))
	})
}
