package icons

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// RemoteBase is where icons are downloaded from: the selfh.st repository through jsDelivr,
// on its main branch so that icons published after this build are found too. Every body is
// checked before it is cached.
const RemoteBase = "https://cdn.jsdelivr.net/gh/selfhst/icons@main"

// HTTPFetcher downloads from a fixed base URL. The reference is validated before it is
// appended, so no caller can steer the request elsewhere (SSRF).
type HTTPFetcher struct {
	Base   string
	Client *http.Client
}

// NewHTTPFetcher returns a fetcher with a short timeout that refuses redirects.
func NewHTTPFetcher(base string) *HTTPFetcher {
	return &HTTPFetcher{
		Base: base,
		Client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("redirect refused")
			},
		},
	}
}

// Fetch tries the SVG, then the WebP: some icons only exist as bitmaps.
func (f *HTTPFetcher) Fetch(ctx context.Context, ref string) (Icon, error) {
	if !ValidRef(ref) {
		return Icon{}, ErrNotFound
	}
	for _, v := range []struct{ dir, ext, contentType string }{
		{"svg", "svg", "image/svg+xml"},
		{"webp", "webp", "image/webp"},
	} {
		body, err := f.get(ctx, fmt.Sprintf("%s/%s/%s.%s", f.Base, v.dir, ref, v.ext))
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return Icon{}, err
		}
		if err := Check(v.contentType, body); err != nil {
			slog.Warn("icon rejected", "ref", ref, "err", err)
			return Icon{}, ErrNotFound
		}
		return Icon{ContentType: v.contentType, Body: body}, nil
	}
	return Icon{}, ErrNotFound
}

func (f *HTTPFetcher) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "hub-server (https://selfh.st/icons)")
	res, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%s: HTTP %d", url, res.StatusCode)
	}
	// One byte over the bound is enough for Check to reject the body.
	return io.ReadAll(io.LimitReader(res.Body, MaxBytes+1))
}
