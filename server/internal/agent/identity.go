package agent

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"hub/server/internal/agentproto"
	"hub/server/internal/pki"
)

// Files in the state directory. agent.pem holds the certificate and its key together, so
// a renewal replaces both in one rename.
const (
	identityFile = "agent.pem"
	caFile       = "ca.pem"
)

var ErrExpired = errors.New("agent certificate expired: enroll this host again")

// Identity is the agent's certificate, its key and the CA it trusts.
type Identity struct {
	Cert tls.Certificate
	Leaf *x509.Certificate
	CA   *x509.Certificate
}

// HasIdentity reports whether the agent has already enrolled.
func HasIdentity(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, identityFile))
	return err == nil
}

func LoadIdentity(dir string) (*Identity, error) {
	data, err := os.ReadFile(filepath.Join(dir, identityFile))
	if err != nil {
		return nil, err
	}
	cert, err := tls.X509KeyPair(data, data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", identityFile, err)
	}
	caPEM, err := os.ReadFile(filepath.Join(dir, caFile))
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(caPEM)
	if block == nil {
		return nil, fmt.Errorf("%s: no certificate", caFile)
	}
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	return &Identity{Cert: cert, Leaf: cert.Leaf, CA: ca}, nil
}

// DueForRenewal is true past two thirds of the certificate's lifetime.
func (id *Identity) DueForRenewal(now time.Time) bool {
	life := id.Leaf.NotAfter.Sub(id.Leaf.NotBefore)
	return now.After(id.Leaf.NotBefore.Add(life * 2 / 3))
}

// Client is an mTLS client of the server that trusts only the agents' CA.
func (id *Identity) Client() *http.Client {
	pool := x509.NewCertPool()
	pool.AddCert(id.CA)
	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{id.Cert}},
		ForceAttemptHTTP2: true,
		IdleConnTimeout:   90 * time.Second,
	}
	return &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

// Enroll trades the one-time code for a certificate. The key is made here and never
// leaves the host. Before sending the code, the server must show a chain whose CA matches
// pin: there is no trust on first use.
func Enroll(ctx context.Context, serverURL, pin, code, hostname, dir string) error {
	key, csr, err := newKey()
	if err != nil {
		return err
	}
	var ca *x509.Certificate
	u, err := url.Parse(serverURL)
	if err != nil {
		return err
	}
	tr := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS13,
		// Verified below against the pinned CA instead of the system roots.
		InsecureSkipVerify: true, //nolint:gosec
		VerifyConnection: func(cs tls.ConnectionState) error {
			var err error
			ca, err = verifyPinned(cs.PeerCertificates, pin, u.Hostname())
			return err
		},
	}}
	body, _ := json.Marshal(agentproto.EnrollRequest{Code: code, CSR: csr, Hostname: hostname})
	var resp agentproto.CertResponse
	if err := post(ctx, &http.Client{Transport: tr, Timeout: 30 * time.Second}, serverURL+agentproto.PathEnroll, body, &resp); err != nil {
		return fmt.Errorf("enroll: %w", err)
	}
	return save(dir, resp.Cert, key, ca)
}

// Renew replaces the certificate and the key over the current mTLS connection.
func Renew(ctx context.Context, id *Identity, serverURL, dir string) (*Identity, error) {
	key, csr, err := newKey()
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(agentproto.RenewRequest{CSR: csr})
	var resp agentproto.CertResponse
	if err := post(ctx, id.Client(), serverURL+agentproto.PathRenew, body, &resp); err != nil {
		return nil, fmt.Errorf("renew: %w", err)
	}
	if err := save(dir, resp.Cert, key, id.CA); err != nil {
		return nil, err
	}
	return LoadIdentity(dir)
}

// verifyPinned finds in the presented chain the CA whose key matches pin and checks the
// server certificate against it, for host.
func verifyPinned(chain []*x509.Certificate, pin, host string) (*x509.Certificate, error) {
	if len(chain) == 0 {
		return nil, errors.New("server sent no certificate")
	}
	for _, c := range chain[1:] {
		if pki.PinOf(c) != pin {
			continue
		}
		roots := x509.NewCertPool()
		roots.AddCert(c)
		_, err := chain[0].Verify(x509.VerifyOptions{Roots: roots, DNSName: host})
		if err != nil {
			return nil, fmt.Errorf("server certificate: %w", err)
		}
		return c, nil
	}
	return nil, errors.New("the server's CA does not match HUB_CA_PIN")
}

func newKey() (*ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	return key, csr, err
}

// save checks the certificate matches the key and the CA, then writes agent.pem (0600)
// and ca.pem through a rename, so a crash never leaves half a file.
func save(dir string, certDER []byte, key *ecdsa.PrivateKey, ca *x509.Certificate) error {
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return err
	}
	if !key.PublicKey.Equal(cert.PublicKey) {
		return errors.New("the server returned a certificate for another key")
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return fmt.Errorf("issued certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	_ = pem.Encode(&buf, &pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := writeFile(filepath.Join(dir, caFile), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})); err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, identityFile), buf.Bytes())
}

func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func post(ctx context.Context, c *http.Client, url string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return &StatusError{Code: resp.StatusCode, Body: string(bytes.TrimSpace(data))}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// StatusError is a non-2xx answer of the server.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string { return fmt.Sprintf("server answered %d: %s", e.Code, e.Body) }
