package icons

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type memRepo struct {
	mu   sync.Mutex
	rows map[string]CachedIcon
}

func (m *memRepo) CachedIcon(_ context.Context, ref string) (CachedIcon, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.rows[ref]
	if !ok {
		return CachedIcon{}, ErrNotFound
	}
	return c, nil
}

func (m *memRepo) PutCachedIcon(_ context.Context, c CachedIcon) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[c.Ref] = c
	return nil
}

type fakeFetcher struct {
	calls int
	icon  Icon
	err   error
}

func (f *fakeFetcher) Fetch(context.Context, string) (Icon, error) {
	f.calls++
	return f.icon, f.err
}

const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><path fill="url(#g)" d="M0 0"/></svg>`

func newTestService(f Fetcher) (*Service, *memRepo, *time.Time) {
	repo := &memRepo{rows: map[string]CachedIcon{}}
	s := NewService(repo, f)
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	return s, repo, &now
}

func TestBundledIconNeedsNoNetwork(t *testing.T) {
	f := &fakeFetcher{err: errors.New("offline")}
	s, _, _ := newTestService(f)
	icon, err := s.Get(context.Background(), "emby")
	if err != nil || icon.ContentType != "image/svg+xml" || f.calls != 0 {
		t.Fatalf("icon %q, err %v, calls %d", icon.ContentType, err, f.calls)
	}
	if icon, err := s.Get(context.Background(), "tdarr"); err != nil || icon.ContentType != "image/webp" {
		t.Fatalf("tdarr: %q, %v", icon.ContentType, err)
	}
}

func TestDownloadedOnceThenCached(t *testing.T) {
	f := &fakeFetcher{icon: Icon{ContentType: "image/svg+xml", Body: []byte(svg)}}
	s, repo, _ := newTestService(f)
	for range 3 {
		if _, err := s.Get(context.Background(), "audiobookshelf"); err != nil {
			t.Fatal(err)
		}
	}
	if f.calls != 1 || repo.rows["audiobookshelf"].Icon == nil {
		t.Fatalf("calls %d, cached %+v", f.calls, repo.rows["audiobookshelf"])
	}
}

func TestMissingIsRememberedForAWeek(t *testing.T) {
	f := &fakeFetcher{err: ErrNotFound}
	s, _, now := newTestService(f)
	ctx := context.Background()
	for range 2 {
		if _, err := s.Get(ctx, "nope"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err %v", err)
		}
	}
	if f.calls != 1 {
		t.Fatalf("calls %d, want 1", f.calls)
	}
	*now = now.Add(missingTTL + time.Minute)
	_, _ = s.Get(ctx, "nope")
	if f.calls != 2 {
		t.Fatalf("calls %d after a week, want 2", f.calls)
	}
}

func TestNetworkFailureBacksOffAndIsNotCached(t *testing.T) {
	f := &fakeFetcher{err: errors.New("dial tcp: timeout")}
	s, repo, now := newTestService(f)
	ctx := context.Background()
	for range 3 {
		if _, err := s.Get(ctx, "audiobookshelf"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err %v", err)
		}
	}
	if f.calls != 1 || len(repo.rows) != 0 {
		t.Fatalf("calls %d, rows %d", f.calls, len(repo.rows))
	}
	*now = now.Add(failureBackoff + time.Second)
	f.err, f.icon = nil, Icon{ContentType: "image/svg+xml", Body: []byte(svg)}
	if _, err := s.Get(ctx, "audiobookshelf"); err != nil || f.calls != 2 {
		t.Fatalf("err %v, calls %d", err, f.calls)
	}
}

func TestNoFetcherStaysOffline(t *testing.T) {
	s, _, _ := newTestService(nil)
	if _, err := s.Get(context.Background(), "audiobookshelf"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err %v", err)
	}
}

func TestInvalidRefsNeverReachTheFetcher(t *testing.T) {
	f := &fakeFetcher{}
	s, _, _ := newTestService(f)
	for _, ref := range []string{"", "../secrets", "Emby", "emby.svg", "a/b", "-x", strings.Repeat("a", 64)} {
		if _, err := s.Get(context.Background(), ref); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q: err %v", ref, err)
		}
	}
	if f.calls != 0 {
		t.Fatalf("fetcher called %d times", f.calls)
	}
}

func TestCheckRejectsActiveOrExternalSVG(t *testing.T) {
	for name, body := range map[string]string{
		"script":       `<svg><script>alert(1)</script></svg>`,
		"handler":      `<svg onload="alert(1)"></svg>`,
		"foreign":      `<svg><foreignObject></foreignObject></svg>`,
		"js link":      `<svg><a href="javascript:alert(1)"></a></svg>`,
		"external":     `<svg><image href="https://evil.example/x.png"/></svg>`,
		"xlink":        `<svg><use xlink:href="//evil.example/s.svg#a"/></svg>`,
		"css url":      `<svg><style>rect{fill:url(https://evil.example/x)}</style></svg>`,
		"entity":       `<!DOCTYPE svg [<!ENTITY x "y">]><svg></svg>`,
		"not svg":      `<html></html>`,
		"too large":    `<svg>` + strings.Repeat(" ", MaxBytes) + `</svg>`,
		"empty":        ``,
		"svg as webp?": `RIFF0000WEBX`,
	} {
		ct := "image/svg+xml"
		if strings.HasPrefix(body, "RIFF") {
			ct = "image/webp"
		}
		if err := Check(ct, []byte(body)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	ok := `<svg><defs><linearGradient id="g"/></defs><use href="#g"/><image href="data:image/png;base64,AA"/><rect fill="url(#g)"/></svg>`
	if err := Check("image/svg+xml", []byte(ok)); err != nil {
		t.Fatalf("safe SVG rejected: %v", err)
	}
}

func TestHTTPFetcherFallsBackToWebPAndRefusesRedirects(t *testing.T) {
	webp := "RIFF\x00\x00\x00\x00WEBPVP8 "
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/svg/emby.svg":
			_, _ = w.Write([]byte(svg))
		case "/webp/tdarr.webp":
			_, _ = w.Write([]byte(webp))
		case "/svg/moved.svg":
			http.Redirect(w, r, "http://169.254.169.254/", http.StatusFound)
		case "/svg/evil.svg":
			_, _ = w.Write([]byte(`<svg onload="x()"/>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	f := NewHTTPFetcher(srv.URL)
	ctx := context.Background()

	if icon, err := f.Fetch(ctx, "emby"); err != nil || icon.ContentType != "image/svg+xml" {
		t.Fatalf("emby: %q %v", icon.ContentType, err)
	}
	if icon, err := f.Fetch(ctx, "tdarr"); err != nil || icon.ContentType != "image/webp" {
		t.Fatalf("tdarr: %q %v", icon.ContentType, err)
	}
	if _, err := f.Fetch(ctx, "none"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("none: %v", err)
	}
	if _, err := f.Fetch(ctx, "evil"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("evil: %v", err)
	}
	if _, err := f.Fetch(ctx, "moved"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("redirect followed or hidden: %v", err)
	}
}
