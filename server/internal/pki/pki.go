// Package pki is Hub's private certificate authority for the agents (AGENT-STUDY.md). It
// signs the agents' certificates and the certificate of the agent listener; it has nothing
// to do with the dashboard's public certificate. The CA lives in a file outside the
// database (HUB_AGENT_CA_FILE), created on the first start and never overwritten.
package pki

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	CALifetime     = 10 * 365 * 24 * time.Hour
	AgentLifetime  = 30 * 24 * time.Hour
	ServerLifetime = 365 * 24 * time.Hour
	// Backdate makes a certificate valid a little before it is issued: the Raspberry Pi has
	// no battery-backed clock and may run behind until NTP catches up.
	Backdate = 5 * time.Minute

	agentScheme = "hub"
	agentHost   = "agent"
)

var (
	ErrInvalidCSR = errors.New("invalid certificate request")
	ErrNotAgent   = errors.New("not an agent certificate")
	hostIDRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

// ValidHostID reports whether id can name a host (same rule as the hosts table).
func ValidHostID(id string) bool { return hostIDRe.MatchString(id) }

type CA struct {
	cert *x509.Certificate
	key  crypto.Signer
}

// Cert is the CA certificate, which agents trust.
func (ca *CA) Cert() *x509.Certificate { return ca.cert }

// Pin is what an agent checks before trusting the CA on its first contact:
// "sha256:" and the hex SHA-256 of the CA's public key (SubjectPublicKeyInfo).
func (ca *CA) Pin() string { return PinOf(ca.cert) }

func PinOf(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// IsNotExist reports whether LoadFile failed because the file does not exist yet.
func IsNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }

// LoadFile reads a CA written by CreateFile: the certificate then the private key, in PEM.
func LoadFile(path string) (*CA, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ca CA
	for block, rest := pem.Decode(data); block != nil; block, rest = pem.Decode(rest) {
		switch block.Type {
		case "CERTIFICATE":
			if ca.cert, err = x509.ParseCertificate(block.Bytes); err != nil {
				return nil, fmt.Errorf("CA certificate: %w", err)
			}
		case "PRIVATE KEY":
			k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("CA key: %w", err)
			}
			signer, ok := k.(crypto.Signer)
			if !ok {
				return nil, errors.New("CA key: not a signing key")
			}
			ca.key = signer
		}
	}
	if ca.cert == nil || ca.key == nil {
		return nil, errors.New("CA file must hold a certificate and a private key")
	}
	if !ca.cert.IsCA || !publicKeysEqual(ca.cert.PublicKey, ca.key.Public()) {
		return nil, errors.New("CA file: the key does not match a CA certificate")
	}
	return &ca, nil
}

// CreateFile makes a new CA and writes it to path (0600). It never overwrites a file.
func CreateFile(path string, now time.Time) (*CA, error) {
	ca, err := NewCA(now)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(ca.key)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw})
	_ = pem.Encode(&buf, &pem.Block{Type: "PRIVATE KEY", Bytes: der})
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return ca, f.Close()
}

// NewCA makes a CA in memory (ECDSA P-256).
func NewCA(now time.Time) (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		Subject:               pkix.Name{CommonName: "Hub agents CA"},
		NotBefore:             now.Add(-Backdate),
		NotAfter:              now.Add(CALifetime),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	if tmpl.SerialNumber, err = serial(); err != nil {
		return nil, err
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &CA{cert: cert, key: key}, nil
}

// SignAgent issues the certificate of hostID's agent for the key in csrDER. Only the key
// is taken from the request: the identity comes from the enrollment or the current
// certificate, never from what the agent asks for.
func (ca *CA) SignAgent(csrDER []byte, hostID string, now time.Time) (*x509.Certificate, error) {
	if !ValidHostID(hostID) {
		return nil, fmt.Errorf("invalid host id %q", hostID)
	}
	csr, err := parseCSR(csrDER)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		Subject:     pkix.Name{CommonName: "hub-agent " + hostID},
		URIs:        []*url.URL{AgentURI(hostID)},
		NotBefore:   now.Add(-Backdate),
		NotAfter:    now.Add(AgentLifetime),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	return ca.issue(tmpl, csr.PublicKey)
}

// CheckCSR reports whether SignAgent would accept csrDER, without signing anything.
func CheckCSR(csrDER []byte) error {
	_, err := parseCSR(csrDER)
	return err
}

func parseCSR(der []byte) (*x509.CertificateRequest, error) {
	csr, err := x509.ParseCertificateRequest(der)
	if err != nil || csr.CheckSignature() != nil {
		return nil, ErrInvalidCSR
	}
	if !acceptedKey(csr.PublicKey) {
		return nil, fmt.Errorf("%w: key must be ECDSA P-256 or Ed25519", ErrInvalidCSR)
	}
	return csr, nil
}

// ServerCertificate issues the listener's certificate for names (host names or IPs), with
// a key that stays in memory: a new one at each start, so it never touches the disk.
func (ca *CA) ServerCertificate(names []string, now time.Time) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		Subject:     pkix.Name{CommonName: "hub-server"},
		NotBefore:   now.Add(-Backdate),
		NotAfter:    now.Add(ServerLifetime),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, n := range names {
		if ip := net.ParseIP(n); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else if n != "" {
			tmpl.DNSNames = append(tmpl.DNSNames, n)
		}
	}
	cert, err := ca.issue(tmpl, key.Public())
	if err != nil {
		return tls.Certificate{}, err
	}
	// The chain carries the CA: on its first contact an agent has only the pin to check it.
	return tls.Certificate{Certificate: [][]byte{cert.Raw, ca.cert.Raw}, PrivateKey: key, Leaf: cert}, nil
}

// Pool is the CertPool that verifies agent certificates.
func (ca *CA) Pool() *x509.CertPool {
	p := x509.NewCertPool()
	p.AddCert(ca.cert)
	return p
}

func (ca *CA) issue(tmpl *x509.Certificate, pub crypto.PublicKey) (*x509.Certificate, error) {
	var err error
	if tmpl.SerialNumber, err = serial(); err != nil {
		return nil, err
	}
	if tmpl.NotAfter.After(ca.cert.NotAfter) {
		tmpl.NotAfter = ca.cert.NotAfter
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, pub, ca.key)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(der)
}

// AgentURI is the identity an agent certificate carries: hub://agent/<host id>.
func AgentURI(hostID string) *url.URL {
	return &url.URL{Scheme: agentScheme, Host: agentHost, Path: "/" + hostID}
}

// AgentHostID reads the host id from a verified agent certificate.
func AgentHostID(cert *x509.Certificate) (string, error) {
	if len(cert.URIs) != 1 {
		return "", ErrNotAgent
	}
	u := cert.URIs[0]
	id := strings.TrimPrefix(u.Path, "/")
	if u.Scheme != agentScheme || u.Host != agentHost || !ValidHostID(id) {
		return "", ErrNotAgent
	}
	return id, nil
}

// Serial is the text form of a certificate's serial number, as the database keeps it.
func Serial(cert *x509.Certificate) string { return cert.SerialNumber.Text(16) }

func serial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

func acceptedKey(pub crypto.PublicKey) bool {
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		return k.Curve == elliptic.P256()
	case ed25519.PublicKey:
		return true
	}
	return false
}

func publicKeysEqual(a, b crypto.PublicKey) bool {
	ka, ok := a.(interface{ Equal(crypto.PublicKey) bool })
	return ok && ka.Equal(b)
}
