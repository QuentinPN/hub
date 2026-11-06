package widgets

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"hub/server/internal/secrets"
)

// maxPoster bounds an image: beyond it the poster is simply not shown.
const maxPoster = 5 << 20

// PosterSource is a connector whose service holds images. The reference comes from the
// browser: it is checked here, so that it can only ever name an image.
type PosterSource interface {
	PosterRequest(ref string) (path string, query url.Values, ok bool)
}

var (
	embyPosterRef = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
	arrPosterRef  = regexp.MustCompile(`^[0-9]{1,12}$`)
)

// Route of dev.emby.media/reference/RestAPI/ImageService.
func (emby) PosterRequest(ref string) (string, url.Values, bool) {
	if !embyPosterRef.MatchString(ref) {
		return "", nil, false
	}
	return "/emby/Items/" + ref + "/Images/Primary", url.Values{"MaxHeight": {"500"}}, true
}

// Route of their openapi.json; poster-500 is the size their own pages ask for.
func (arr) PosterRequest(ref string) (string, url.Values, bool) {
	if !arrPosterRef.MatchString(ref) {
		return "", nil, false
	}
	return "/api/v3/mediacover/" + ref + "/poster-500.jpg", nil, true
}

// Poster is an image ready to send to the browser.
type Poster struct {
	Body        []byte
	ContentType string
}

// FetchPoster asks the service for an image. Only raster formats pass, recognized from the
// bytes rather than from what the service says: an SVG can carry a script.
func FetchPoster(ctx context.Context, t Target, key secrets.Value, ref string) (Poster, bool) {
	connector, ok := connectors[t.Kind]
	if !ok {
		return Poster{}, false
	}
	source, ok := connector.(PosterSource)
	if !ok {
		return Poster{}, false
	}
	path, query, ok := source.PosterRequest(ref)
	if !ok {
		return Poster{}, false
	}
	c, err := open(t, key, nil)
	if err != nil {
		return Poster{}, false
	}
	target := strings.TrimRight(c.base, "/") + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Poster{}, false
	}
	req.Header.Set("User-Agent", "Hub")
	connector.Authorize(req, key)
	resp, err := c.client.Do(req)
	if err != nil {
		return Poster{}, false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Poster{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPoster+1))
	if err != nil || len(body) > maxPoster {
		return Poster{}, false
	}
	switch kind := http.DetectContentType(body); kind {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return Poster{Body: body, ContentType: kind}, true
	}
	return Poster{}, false
}
