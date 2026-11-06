// Package urlcheck asks whether a service's address answers, from the server. It reports a
// verdict and never what the page contains, follows no redirect and sends no credential.
package urlcheck

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	Timeout = 5 * time.Second
	maxLen  = 2048
)

type Kind string

const (
	KindOK          Kind = "ok"
	KindAuth        Kind = "auth" // answers and asks to log in
	KindCert        Kind = "cert" // certificate neither trusted nor accepted
	KindUnreachable Kind = "unreachable"
	KindError       Kind = "error" // the service answers with a 5xx
)

type Result struct {
	Kind      Kind
	Status    int
	LatencyMs int
	// CertSHA256 and CertName describe the certificate seen, for KindCert.
	CertSHA256 string
	CertName   string
}

// Answers tells whether the address can be used as it is.
func (r Result) Answers() bool { return r.Kind == KindOK || r.Kind == KindAuth }

// Validate returns the address to store, or the reason it is refused. Private addresses
// are allowed: they are what a homelab's services use.
func Validate(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxLen {
		return "", errors.New("adresse vide ou trop longue")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", errors.New("adresse illisible")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("seules http:// et https:// sont acceptées")
	}
	if u.User != nil {
		return "", errors.New("pas d'identifiant dans l'adresse")
	}
	u.Fragment = ""
	return u.String(), nil
}

// Check requests raw once. pinned is the hex SHA-256 of a certificate the user accepted:
// that exact certificate passes, anything else goes through the usual verification.
func Check(ctx context.Context, raw, pinned string) Result {
	u, err := url.Parse(raw)
	if err != nil {
		return Result{Kind: KindUnreachable}
	}
	var mu sync.Mutex
	var seen Result
	transport := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig: &tls.Config{
			// Verified below: the standard check has no room for an accepted certificate.
			InsecureSkipVerify: true, //nolint:gosec
			VerifyConnection: func(cs tls.ConnectionState) error {
				leaf := cs.PeerCertificates[0]
				sum := sha256.Sum256(leaf.Raw)
				fingerprint := hex.EncodeToString(sum[:])
				if pinned != "" && fingerprint == pinned {
					return nil
				}
				intermediates := x509.NewCertPool()
				for _, c := range cs.PeerCertificates[1:] {
					intermediates.AddCert(c)
				}
				_, err := leaf.Verify(x509.VerifyOptions{DNSName: u.Hostname(), Intermediates: intermediates})
				if err != nil {
					mu.Lock()
					seen = Result{Kind: KindCert, CertSHA256: fingerprint, CertName: certName(leaf)}
					mu.Unlock()
				}
				return err
			},
		},
	}
	defer transport.CloseIdleConnections()
	client := http.Client{
		Transport: transport,
		Timeout:   Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return Result{Kind: KindUnreachable}
	}
	req.Header.Set("User-Agent", "Hub")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		mu.Lock()
		defer mu.Unlock()
		if seen.Kind == KindCert {
			return seen
		}
		return Result{Kind: KindUnreachable}
	}
	_ = resp.Body.Close()
	r := Result{Kind: KindOK, Status: resp.StatusCode, LatencyMs: int(time.Since(start).Milliseconds())}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		r.Kind = KindAuth
	case resp.StatusCode >= 500:
		r.Kind = KindError
	}
	return r
}

func certName(c *x509.Certificate) string {
	if len(c.DNSNames) > 0 {
		return c.DNSNames[0]
	}
	return c.Subject.CommonName
}

// Target is an address to watch.
type Target struct {
	Slug   string
	URL    string
	Pinned string
}

// Source is where Watch finds addresses and leaves verdicts.
type Source interface {
	URLTargets(ctx context.Context) ([]Target, error)
	RecordURLCheck(ctx context.Context, t Target, r Result, at time.Time) error
}

// Watch checks every address now, then at each interval, until ctx ends.
func Watch(ctx context.Context, src Source, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		checkAll(ctx, src)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func checkAll(ctx context.Context, src Source) {
	targets, err := src.URLTargets(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("url check", "err", err)
		}
		return
	}
	limit := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Add(1)
		limit <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-limit }()
			r := Check(ctx, t.URL, t.Pinned)
			if err := src.RecordURLCheck(ctx, t, r, time.Now()); err != nil && ctx.Err() == nil {
				slog.Error("url check", "service", t.Slug, "err", err)
			}
		}()
	}
	wg.Wait()
}
