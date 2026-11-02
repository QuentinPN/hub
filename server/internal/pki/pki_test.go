package pki

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

func csrFor(t *testing.T, cn string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// The agent asks for another host's identity: it must be ignored.
	tmpl := &x509.CertificateRequest{Subject: pkix.Name{CommonName: cn}, URIs: []*url.URL{AgentURI("other-host")}}
	der, err := x509.CreateCertificateRequest(rand.Reader, tmpl, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestCreateLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-ca.pem")
	ca, err := CreateFile(path, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateFile(path, now); err == nil {
		t.Fatal("CreateFile overwrote an existing file")
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Pin() != ca.Pin() {
		t.Fatalf("pin changed: %s != %s", loaded.Pin(), ca.Pin())
	}
	if _, err := LoadFile(filepath.Join(t.TempDir(), "missing")); !IsNotExist(err) {
		t.Fatalf("missing file: %v", err)
	}
	if info, err := os.Stat(path); err == nil && os.PathSeparator == '/' && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
}

func TestLoadFileRejectsMismatchedKey(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if _, err := CreateFile(filepath.Join(dir, name), now); err != nil {
			t.Fatal(err)
		}
	}
	da, _ := os.ReadFile(filepath.Join(dir, "a"))
	db, _ := os.ReadFile(filepath.Join(dir, "b"))
	// Certificate of a, key of b.
	mixed := append(da[:indexKey(da)], db[indexKey(db):]...)
	path := filepath.Join(dir, "mixed")
	if err := os.WriteFile(path, mixed, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil {
		t.Fatal("accepted a key that does not match the certificate")
	}
}

func indexKey(b []byte) int { return bytes.Index(b, []byte("-----BEGIN PRIVATE KEY-----")) }

func TestSignAgent(t *testing.T) {
	ca, err := NewCA(now)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := ca.SignAgent(csrFor(t, "rpi5"), "rpi5", now)
	if err != nil {
		t.Fatal(err)
	}
	id, err := AgentHostID(cert)
	if err != nil || id != "rpi5" {
		t.Fatalf("host id %q, %v", id, err)
	}
	if got := cert.NotAfter.Sub(now); got != AgentLifetime {
		t.Fatalf("lifetime %v", got)
	}
	if !cert.NotBefore.Equal(now.Add(-Backdate)) {
		t.Fatalf("not before %v", cert.NotBefore)
	}
	_, err = cert.Verify(x509.VerifyOptions{Roots: ca.Pool(), CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: ca.Pool(), CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err == nil {
		t.Fatal("an agent certificate must not work as a server certificate")
	}
}

func TestSignAgentRejects(t *testing.T) {
	ca, _ := NewCA(now)
	if _, err := ca.SignAgent([]byte("junk"), "rpi5", now); err == nil {
		t.Fatal("accepted junk")
	}
	if _, err := ca.SignAgent(csrFor(t, "x"), "Bad_Host", now); err == nil {
		t.Fatal("accepted an invalid host id")
	}
	// Altered request: the signature no longer matches.
	der := csrFor(t, "x")
	der[len(der)-5] ^= 0xff
	if _, err := ca.SignAgent(der, "rpi5", now); err == nil {
		t.Fatal("accepted a request with a bad signature")
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaCSR, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, rsaKey)
	if _, err := ca.SignAgent(rsaCSR, "rpi5", now); err == nil {
		t.Fatal("accepted an RSA key")
	}
}

func TestServerCertificate(t *testing.T) {
	ca, _ := NewCA(now)
	tc, err := ca.ServerCertificate([]string{"192.0.2.14", "localhost"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(tc.Certificate) != 2 {
		t.Fatal("the chain must carry the CA")
	}
	for _, name := range []string{"192.0.2.14", "localhost"} {
		_, err := tc.Leaf.Verify(x509.VerifyOptions{Roots: ca.Pool(), DNSName: name, CurrentTime: now})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := AgentHostID(tc.Leaf); err == nil {
		t.Fatal("the server certificate must not read as an agent")
	}
}

func TestAgentHostIDRejectsForeignURIs(t *testing.T) {
	for _, raw := range []string{"https://agent/rpi5", "hub://server/rpi5", "hub://agent/Bad", "hub://agent/a/b"} {
		u, _ := url.Parse(raw)
		if _, err := AgentHostID(&x509.Certificate{URIs: []*url.URL{u}}); err == nil {
			t.Errorf("%s accepted", raw)
		}
	}
}
