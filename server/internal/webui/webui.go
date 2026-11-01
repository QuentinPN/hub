// Package webui serves the SvelteKit build embedded in the binary. The build is copied
// into dist/ by `make web` (or the CI) before `go build`.
package webui

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

//go:embed all:dist
var embedded embed.FS

// scriptTag captures a <script> element's attributes and body.
var scriptTag = regexp.MustCompile(`(?s)<script([^>]*)>(.*?)</script>`)

// UI is the SPA handler plus the CSP hashes of index.html's inline scripts.
type UI struct {
	Handler      http.Handler
	ScriptHashes []string
}

// New builds the UI from the embedded build. If the build is missing, every page answers
// 503 with a hint, so the API stays usable during development.
func New() UI {
	dist, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err) // dist is embedded at compile time; this cannot fail at runtime
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return UI{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Interface non construite : lancer `npm run build` dans web/ puis copier build/ dans server/internal/webui/dist.", http.StatusServiceUnavailable)
		})}
	}
	return UI{Handler: spa(dist, index), ScriptHashes: hashInlineScripts(index)}
}

func hashInlineScripts(html []byte) []string {
	var hashes []string
	for _, m := range scriptTag.FindAllSubmatch(html, -1) {
		if strings.Contains(string(m[1]), "src=") {
			continue
		}
		sum := sha256.Sum256(m[2])
		hashes = append(hashes, "sha256-"+base64.StdEncoding.EncodeToString(sum[:]))
	}
	return hashes
}

// spa serves real files as-is and index.html for client-side routes.
func spa(dist fs.FS, index []byte) http.Handler {
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if st, err := fs.Stat(dist, name); err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "_app/immutable/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}
