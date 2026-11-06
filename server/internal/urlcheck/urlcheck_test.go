package urlcheck_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hub/server/internal/urlcheck"
)

func TestValidate(t *testing.T) {
	for _, raw := range []string{"http://192.0.2.14:8096", "https://emby.example.fr/web", " https://x.fr "} {
		if _, err := urlcheck.Validate(raw); err != nil {
			t.Errorf("%q refused: %v", raw, err)
		}
	}
	for _, raw := range []string{"", "ftp://x.fr", "file:///etc/passwd", "javascript:alert(1)", "https://user:pw@x.fr", "http://", "x.fr", "https://" + strings.Repeat("a", 3000)} {
		if _, err := urlcheck.Validate(raw); err == nil {
			t.Errorf("%q accepted", raw)
		}
	}
	if got, _ := urlcheck.Validate("http://x.fr/a#b"); got != "http://x.fr/a" {
		t.Errorf("fragment kept: %q", got)
	}
}

func TestCheckVerdicts(t *testing.T) {
	cases := []struct {
		status int
		want   urlcheck.Kind
	}{
		{200, urlcheck.KindOK},
		{302, urlcheck.KindOK},
		{404, urlcheck.KindOK},
		{401, urlcheck.KindAuth},
		{403, urlcheck.KindAuth},
		{500, urlcheck.KindError},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", "http://127.0.0.1:1/elsewhere")
			w.WriteHeader(c.status)
		}))
		r := urlcheck.Check(context.Background(), srv.URL, "")
		srv.Close()
		if r.Kind != c.want || r.Status != c.status {
			t.Errorf("status %d: %+v, want %s", c.status, r, c.want)
		}
	}
}

func TestCheckUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if r := urlcheck.Check(context.Background(), url, ""); r.Kind != urlcheck.KindUnreachable {
		t.Fatalf("got %+v", r)
	}
}

func TestCheckSendsNoCookieAndFollowsNothing(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credentials sent")
		}
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer srv.Close()
	urlcheck.Check(context.Background(), srv.URL, "")
	if calls.Load() != 1 {
		t.Fatalf("%d requests, want 1", calls.Load())
	}
}

func TestCertificateMustBeAccepted(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	sum := sha256.Sum256(srv.Certificate().Raw)
	fingerprint := hex.EncodeToString(sum[:])

	r := urlcheck.Check(context.Background(), srv.URL, "")
	if r.Kind != urlcheck.KindCert || r.CertSHA256 != fingerprint || r.CertName == "" {
		t.Fatalf("unknown certificate: %+v", r)
	}
	if r := urlcheck.Check(context.Background(), srv.URL, strings.Repeat("0", 64)); r.Kind != urlcheck.KindCert {
		t.Fatalf("another pin passed: %+v", r)
	}
	if r := urlcheck.Check(context.Background(), srv.URL, fingerprint); r.Kind != urlcheck.KindOK {
		t.Fatalf("accepted certificate: %+v", r)
	}
}

type fakeSource struct {
	mu      sync.Mutex
	targets []urlcheck.Target
	got     map[string]urlcheck.Kind
	done    chan struct{}
}

func (f *fakeSource) URLTargets(context.Context) ([]urlcheck.Target, error) { return f.targets, nil }

func (f *fakeSource) RecordURLCheck(_ context.Context, t urlcheck.Target, r urlcheck.Result, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got[t.Slug] = r.Kind
	if len(f.got) == len(f.targets) {
		select {
		case f.done <- struct{}{}:
		default:
		}
	}
	return nil
}

func TestWatchRecordsEveryTarget(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	src := &fakeSource{
		targets: []urlcheck.Target{{Slug: "a", URL: srv.URL}, {Slug: "b", URL: "http://127.0.0.1:1"}},
		got:     map[string]urlcheck.Kind{},
		done:    make(chan struct{}, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go urlcheck.Watch(ctx, src, time.Hour)
	select {
	case <-src.done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.got["a"] != urlcheck.KindOK || src.got["b"] != urlcheck.KindUnreachable {
		t.Fatalf("got %v", src.got)
	}
}
