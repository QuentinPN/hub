package authtest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/go-webauthn/webauthn/protocol"
)

// Authenticator flags (WebAuthn §6.1).
const (
	flagUP = 0x01
	flagUV = 0x04
	flagBE = 0x08
	flagBS = 0x10
	flagAT = 0x40
)

var b64 = base64.RawURLEncoding

// SoftKey is a software passkey for tests: one P-256 key, "none" attestation, synced like
// a platform passkey (backup eligible, counter left at 0).
type SoftKey struct {
	Origin  string
	ID      []byte
	key     *ecdsa.PrivateKey
	rpID    string
	handle  []byte
	Counter uint32
	SkipUV  bool
}

func NewSoftKey(t *testing.T, origin string) *SoftKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	_, _ = rand.Read(id)
	return &SoftKey{Origin: origin, ID: id, key: key}
}

// Create answers registration options with a new credential, as navigator.credentials.create.
func (k *SoftKey) Create(t *testing.T, options []byte) []byte {
	t.Helper()
	var creation protocol.CredentialCreation
	if err := json.Unmarshal(options, &creation); err != nil {
		t.Fatal(err)
	}
	opts := creation.Response
	id, _ := opts.User.ID.(string) // base64url in the JSON options
	handle, err := b64.DecodeString(id)
	if err != nil || len(handle) == 0 {
		t.Fatalf("user id %v: %v", opts.User.ID, err)
	}
	k.rpID, k.handle = opts.RelyingParty.ID, handle

	pub := k.key.PublicKey
	x, y := make([]byte, 32), make([]byte, 32)
	pub.X.FillBytes(x) //nolint:staticcheck // the COSE key needs the raw coordinates
	pub.Y.FillBytes(y) //nolint:staticcheck
	enc, err := cbor.CTAP2EncOptions().EncMode()
	if err != nil {
		t.Fatal(err)
	}
	coseKey, err := enc.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: x, -3: y})
	if err != nil {
		t.Fatal(err)
	}
	authData := k.authData(flagAT)
	authData = append(authData, make([]byte, 16)...) // AAGUID: none
	authData = binary.BigEndian.AppendUint16(authData, uint16(len(k.ID)))
	authData = append(authData, k.ID...)
	authData = append(authData, coseKey...)
	attObj, err := enc.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": authData})
	if err != nil {
		t.Fatal(err)
	}
	return k.marshal(t, map[string]any{
		"clientDataJSON":    b64.EncodeToString(k.clientData(t, "webauthn.create", opts.Challenge)),
		"attestationObject": b64.EncodeToString(attObj),
		"transports":        []string{"internal"},
	})
}

// Get answers login options with a signed assertion, as navigator.credentials.get.
func (k *SoftKey) Get(t *testing.T, options []byte) []byte {
	t.Helper()
	var assertion protocol.CredentialAssertion
	if err := json.Unmarshal(options, &assertion); err != nil {
		t.Fatal(err)
	}
	clientData := k.clientData(t, "webauthn.get", assertion.Response.Challenge)
	authData := k.authData(0)
	sum := sha256.Sum256(clientData)
	digest := sha256.Sum256(append(append([]byte{}, authData...), sum[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, k.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return k.marshal(t, map[string]any{
		"clientDataJSON":    b64.EncodeToString(clientData),
		"authenticatorData": b64.EncodeToString(authData),
		"signature":         b64.EncodeToString(sig),
		"userHandle":        b64.EncodeToString(k.handle),
	})
}

func (k *SoftKey) authData(extra byte) []byte {
	rpHash := sha256.Sum256([]byte(k.rpID))
	flags := byte(flagUP|flagUV|flagBE|flagBS) | extra
	if k.SkipUV {
		flags &^= flagUV
	}
	out := append(rpHash[:], flags)
	return binary.BigEndian.AppendUint32(out, k.Counter)
}

func (k *SoftKey) clientData(t *testing.T, typ string, challenge protocol.URLEncodedBase64) []byte {
	t.Helper()
	out, err := json.Marshal(map[string]any{
		"type":        typ,
		"challenge":   b64.EncodeToString(challenge),
		"origin":      k.Origin,
		"crossOrigin": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (k *SoftKey) marshal(t *testing.T, response map[string]any) []byte {
	t.Helper()
	out, err := json.Marshal(map[string]any{
		"id":                      b64.EncodeToString(k.ID),
		"rawId":                   b64.EncodeToString(k.ID),
		"type":                    "public-key",
		"authenticatorAttachment": "platform",
		"response":                response,
		"clientExtensionResults":  map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
