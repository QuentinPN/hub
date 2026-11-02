// Package icons serves the service icons from selfh.st (https://selfh.st/icons, CC BY 4.0).
// An icon is looked up in the set bundled with the binary, then in the PostgreSQL cache,
// then downloaded once from selfh.st through jsDelivr, checked and cached. The browser only
// ever talks to this server; the server only ever talks to one fixed host.
package icons

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Icon is an image ready to serve.
type Icon struct {
	ContentType string // image/svg+xml or image/webp
	Body        []byte
}

// ErrNotFound means there is no usable icon for the reference: unknown to selfh.st,
// rejected by the checks, or not reachable right now. The UI then shows the monogram.
var ErrNotFound = errors.New("icon not found")

// CachedIcon is a cache row. A nil Icon records that selfh.st has no such icon.
type CachedIcon struct {
	Ref       string
	Icon      *Icon
	FetchedAt time.Time
}

// Repo keeps downloaded icons. CachedIcon returns ErrNotFound for a reference never fetched.
type Repo interface {
	CachedIcon(ctx context.Context, ref string) (CachedIcon, error)
	PutCachedIcon(ctx context.Context, c CachedIcon) error
}

// Fetcher downloads an icon. It returns ErrNotFound when the source has none.
type Fetcher interface {
	Fetch(ctx context.Context, ref string) (Icon, error)
}

const (
	// A reference selfh.st did not know is asked again after this delay: new icons appear.
	missingTTL = 7 * 24 * time.Hour
	// After a network failure, no new attempt for this reference before this delay, so an
	// offline server does not wait on a timeout at every page load.
	failureBackoff = 10 * time.Minute
	// MaxBytes bounds a downloaded icon; selfh.st icons weigh a few kilobytes.
	MaxBytes = 200 << 10
)

// refPattern is a selfh.st reference ("emby", "pi-hole", "portainer-light"). It is also
// the only thing that reaches a file name or a URL path.
var refPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ValidRef reports whether ref has the shape of a selfh.st reference.
func ValidRef(ref string) bool { return refPattern.MatchString(ref) }

//go:embed bundled/*.svg bundled/*.webp
var bundledFS embed.FS

// Service resolves icons. With a nil Fetcher it never goes to the network.
type Service struct {
	repo  Repo
	fetch Fetcher
	now   func() time.Time

	mu       sync.Mutex
	inflight map[string]*sync.Mutex
	failures map[string]time.Time
}

func NewService(repo Repo, fetch Fetcher) *Service {
	return &Service{
		repo:     repo,
		fetch:    fetch,
		now:      time.Now,
		inflight: map[string]*sync.Mutex{},
		failures: map[string]time.Time{},
	}
}

// Get returns the icon for ref, or ErrNotFound. Other errors come from the cache.
func (s *Service) Get(ctx context.Context, ref string) (Icon, error) {
	if !ValidRef(ref) {
		return Icon{}, ErrNotFound
	}
	if icon, ok := bundled(ref); ok {
		return icon, nil
	}

	// One download at a time per reference; the others wait and then read the cache.
	lock := s.lockFor(ref)
	lock.Lock()
	defer lock.Unlock()

	cached, err := s.repo.CachedIcon(ctx, ref)
	switch {
	case err == nil && cached.Icon != nil:
		return *cached.Icon, nil
	case err == nil && s.now().Sub(cached.FetchedAt) < missingTTL:
		return Icon{}, ErrNotFound
	case err != nil && !errors.Is(err, ErrNotFound):
		return Icon{}, err
	}
	if s.fetch == nil || s.recentFailure(ref) {
		return Icon{}, ErrNotFound
	}

	icon, err := s.fetch.Fetch(ctx, ref)
	switch {
	case errors.Is(err, ErrNotFound):
		return Icon{}, errors.Join(ErrNotFound, s.repo.PutCachedIcon(ctx, CachedIcon{Ref: ref, FetchedAt: s.now()}))
	case err != nil:
		s.recordFailure(ref)
		slog.Warn("icon download failed, showing the monogram", "ref", ref, "err", err)
		return Icon{}, ErrNotFound
	}
	if err := s.repo.PutCachedIcon(ctx, CachedIcon{Ref: ref, Icon: &icon, FetchedAt: s.now()}); err != nil {
		return Icon{}, err
	}
	slog.Info("icon downloaded from selfh.st", "ref", ref, "bytes", len(icon.Body))
	return icon, nil
}

func (s *Service) lockFor(ref string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.inflight[ref]
	if !ok {
		m = &sync.Mutex{}
		s.inflight[ref] = m
	}
	return m
}

func (s *Service) recentFailure(ref string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	at, ok := s.failures[ref]
	return ok && s.now().Sub(at) < failureBackoff
}

func (s *Service) recordFailure(ref string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures[ref] = s.now()
}

func bundled(ref string) (Icon, bool) {
	for _, f := range []struct{ ext, contentType string }{{"svg", "image/svg+xml"}, {"webp", "image/webp"}} {
		body, err := fs.ReadFile(bundledFS, path.Join("bundled", ref+"."+f.ext))
		if err == nil {
			return Icon{ContentType: f.contentType, Body: body}, true
		}
	}
	return Icon{}, false
}

// unsafeSVG lists what an SVG served from our origin must not contain: opened directly
// rather than through <img>, it would run in the dashboard's origin.
var unsafeSVG = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<script`),
	regexp.MustCompile(`(?i)\son[a-z]+\s*=`),
	regexp.MustCompile(`(?i)<foreignObject`),
	regexp.MustCompile(`(?i)javascript:`),
	regexp.MustCompile(`(?i)<!ENTITY`),
}

// references captures href, src and url() targets; only in-document anchors and inline
// images are allowed, never a resource fetched from elsewhere.
var references = regexp.MustCompile(`(?i)(?:\b(?:href|src)\s*=\s*["']\s*|url\(\s*["']?\s*)([^"')\s]*)`)

func localReference(target string) bool {
	return strings.HasPrefix(target, "#") || strings.HasPrefix(strings.ToLower(target), "data:image/")
}

// Check accepts a downloaded body as an icon of the given content type.
func Check(contentType string, body []byte) error {
	if len(body) == 0 || len(body) > MaxBytes {
		return fmt.Errorf("size %d out of bounds", len(body))
	}
	switch contentType {
	case "image/svg+xml":
		if !bytes.Contains(body, []byte("<svg")) {
			return errors.New("not an SVG document")
		}
		for _, re := range unsafeSVG {
			if re.Match(body) {
				return fmt.Errorf("SVG rejected: matches %s", re)
			}
		}
		for _, m := range references.FindAllSubmatch(body, -1) {
			if !localReference(string(m[1])) {
				return fmt.Errorf("SVG rejected: external reference %q", m[1])
			}
		}
		return nil
	case "image/webp":
		if len(body) < 12 || string(body[0:4]) != "RIFF" || string(body[8:12]) != "WEBP" {
			return errors.New("not a WebP image")
		}
		return nil
	default:
		return fmt.Errorf("unexpected content type %q", contentType)
	}
}
