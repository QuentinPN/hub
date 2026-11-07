package notify

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP is a local SMTP server, enough for net/smtp: STARTTLS, AUTH PLAIN, one message.
// Nothing leaves the machine.
type fakeSMTP struct {
	ln       net.Listener
	cfg      *tls.Config
	pool     *x509.CertPool
	starttls bool

	mu    sync.Mutex
	auth  string
	from  string
	rcpt  []string
	data  string
	clear bool // a command after DATA came in clear
}

func newFakeSMTP(t *testing.T, starttls bool) *fakeSMTP {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln, pool: pool, starttls: starttls,
		cfg: &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}}
	t.Cleanup(func() { _ = ln.Close() })
	go f.serve()
	return f
}

func (f *fakeSMTP) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeSMTP) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeSMTP) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r, w := bufio.NewReader(conn), conn
	say := func(s string) { _, _ = w.Write([]byte(s + "\r\n")) }
	say("220 localhost ESMTP")
	secure := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimSpace(line)
		verb := strings.ToUpper(strings.SplitN(cmd, " ", 2)[0])
		switch verb {
		case "EHLO", "HELO":
			if f.starttls && !secure {
				say("250-localhost")
				say("250 STARTTLS")
			} else {
				say("250-localhost")
				say("250 AUTH PLAIN")
			}
		case "STARTTLS":
			say("220 go ahead")
			tc := tls.Server(conn, f.cfg)
			if tc.Handshake() != nil {
				return
			}
			conn, r, w, secure = tc, bufio.NewReader(tc), tc, true
		case "AUTH":
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(cmd, "AUTH PLAIN "))
			f.mu.Lock()
			f.auth = string(raw)
			f.mu.Unlock()
			if strings.HasSuffix(string(raw), "\x00bon-mot-de-passe") {
				say("235 ok")
			} else {
				say("535 5.7.8 refusé pour bob@exemple.fr")
			}
		case "MAIL":
			f.mu.Lock()
			f.from, f.clear = cmd, !secure
			f.mu.Unlock()
			say("250 ok")
		case "RCPT":
			f.mu.Lock()
			f.rcpt = append(f.rcpt, cmd)
			f.mu.Unlock()
			say("250 ok")
		case "DATA":
			say("354 go")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			f.mu.Lock()
			f.data = b.String()
			f.mu.Unlock()
			say("250 queued")
		case "QUIT":
			say("221 bye")
			return
		default:
			say("502 ?")
		}
	}
}

func smtpService(t *testing.T, f *fakeSMTP) (*Service, *memStore) {
	t.Helper()
	store := newMem()
	svc := NewService(store, memSecrets{}, nil)
	svc.smtpTLS = &tls.Config{RootCAs: f.pool, MinVersion: tls.VersionTLS12}
	if err := svc.SaveSMTP(context.Background(), SMTPInput{Host: "localhost", Port: f.port(), Username: "hub@exemple.fr",
		Password: "bon-mot-de-passe", From: "Hub <hub@exemple.fr>", To: "moi@exemple.fr, autre@exemple.fr"}); err != nil {
		t.Fatal(err)
	}
	return svc, store
}

func TestSMTPSendsOverSTARTTLS(t *testing.T) {
	f := newFakeSMTP(t, true)
	svc, store := smtpService(t, f)
	if err := svc.Test(context.Background(), SMTP); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.clear || f.auth != "\x00hub@exemple.fr\x00bon-mot-de-passe" || f.from != "MAIL FROM:<hub@exemple.fr>" || len(f.rcpt) != 2 {
		t.Fatalf("clear %v auth %q from %q rcpt %v", f.clear, f.auth, f.from, f.rcpt)
	}
	for _, want := range []string{"Subject: =?utf-8?q?Hub_=C2=B7_Test_de_Hub?=", "Content-Type: text/plain; charset=utf-8", "Si tu lis ceci"} {
		if !strings.Contains(f.data, want) {
			t.Errorf("message lacks %q:\n%s", want, f.data)
		}
	}
	if st := store.states[SMTP]; !st.OK || !st.Test {
		t.Fatalf("state %+v", st)
	}
	status, _ := svc.Status(context.Background())
	if !status.SMTP.PasswordSet || status.SMTP.To != "moi@exemple.fr, autre@exemple.fr" {
		t.Fatalf("status %+v", status.SMTP)
	}
}

func TestSMTPRefusesClearText(t *testing.T) {
	f := newFakeSMTP(t, false)
	svc, _ := smtpService(t, f)
	err := svc.Test(context.Background(), SMTP)
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("err %v", err)
	}
	if f.from != "" {
		t.Fatal("a message went out in clear")
	}
}

func TestSMTPErrorsQuoteNoAddress(t *testing.T) {
	f := newFakeSMTP(t, true)
	svc, store := smtpService(t, f)
	ctx := context.Background()
	if err := svc.SaveSMTP(ctx, SMTPInput{Host: "localhost", Port: f.port(), Username: "hub@exemple.fr", Password: "mauvais",
		From: "hub@exemple.fr", To: "moi@exemple.fr"}); err != nil {
		t.Fatal(err)
	}
	err := svc.Test(ctx, SMTP)
	if err == nil || err.Error() != "identifiants refusés (535)" {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(store.states[SMTP].Error, "@") {
		t.Fatalf("state quotes an address: %q", store.states[SMTP].Error)
	}
	// An unknown authority is refused too.
	svc.smtpTLS = &tls.Config{MinVersion: tls.VersionTLS12}
	if err := svc.Test(ctx, SMTP); err == nil || err.Error() != "certificat du serveur refusé" {
		t.Fatalf("err %v", err)
	}
}

func TestSaveSMTPKeepsThePasswordOnItsServer(t *testing.T) {
	f := newFakeSMTP(t, true)
	svc, _ := smtpService(t, f)
	ctx := context.Background()
	in := SMTPInput{Host: "localhost", Port: f.port(), Username: "hub@exemple.fr", From: "hub@exemple.fr", To: "moi@exemple.fr"}
	if err := svc.SaveSMTP(ctx, in); err != nil {
		t.Fatalf("same server, password kept: %v", err)
	}
	other := in
	other.Host = "smtp.ailleurs.fr"
	if err := svc.SaveSMTP(ctx, other); err == nil {
		t.Fatal("password sent to another server")
	}
	for _, bad := range []SMTPInput{
		{Host: "smtp://x", Port: 587, From: "a@b.fr", To: "c@d.fr"},
		{Host: "x", Port: 0, From: "a@b.fr", To: "c@d.fr"},
		{Host: "x", Port: 587, From: "pas une adresse", To: "c@d.fr"},
		{Host: "x", Port: 587, From: "a@b.fr", To: ""},
		{Host: "x", Port: 587, Username: "u", Password: "a\r\nb", From: "a@b.fr", To: "c@d.fr"},
	} {
		if err := svc.SaveSMTP(ctx, bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
