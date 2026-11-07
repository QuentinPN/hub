package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"hub/server/internal/secrets"
)

// newHTTPClient never follows a redirect: the topic goes to the server typed, nowhere else.
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// sendNtfy publishes as JSON to the server's root, as docs.ntfy.sh/publish says: the topic
// travels in the body, never in the address, so no log along the way sees it.
func sendNtfy(ctx context.Context, client *http.Client, base string, topic secrets.Value, m Message) error {
	priority, tags := 4, []string{"warning"}
	switch {
	case m.Test:
		priority, tags = 3, []string{"loudspeaker"}
	case m.Recovery:
		priority, tags = 3, []string{"heavy_check_mark"}
	}
	body, err := json.Marshal(map[string]any{
		"topic": topic.Reveal(), "title": m.Title, "message": m.Body, "priority": priority, "tags": tags,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/", bytes.NewReader(body))
	if err != nil {
		return errors.New("adresse du serveur illisible")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		// The error quotes the address: say what happened, not where.
		var cert *tls.CertificateVerificationError
		if errors.As(err, &cert) {
			return errors.New("certificat du serveur refusé")
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return errors.New("serveur trop lent à répondre")
		}
		return errors.New("serveur injoignable")
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return fmt.Errorf("redirection refusée (%d) : vérifier l'adresse", resp.StatusCode)
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("accès refusé par le serveur (%d) : sujet protégé ?", resp.StatusCode)
	case resp.StatusCode == http.StatusTooManyRequests:
		return errors.New("trop d'envois, le serveur limite (429)")
	default:
		return fmt.Errorf("réponse %d du serveur", resp.StatusCode)
	}
}

// sendSMTP sends one plain-text e-mail. Port 465 is TLS from the start; any other port must
// offer STARTTLS: neither the password nor the message ever crosses in clear.
func sendSMTP(ctx context.Context, base *tls.Config, set Settings, password secrets.Value, m Message, now time.Time) error {
	from, err := mail.ParseAddress(set.SMTPFrom)
	if err != nil {
		return errors.New("expéditeur illisible")
	}
	to, err := mail.ParseAddressList(set.SMTPTo)
	if err != nil {
		return errors.New("destinataire illisible")
	}
	addr := net.JoinHostPort(set.SMTPHost, strconv.Itoa(set.SMTPPort))
	cfg := base.Clone()
	cfg.ServerName = set.SMTPHost

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(20 * time.Second)
	}
	dialer := &net.Dialer{Deadline: deadline}
	var conn net.Conn
	if set.SMTPPort == 465 {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: cfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return smtpError("connexion", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(deadline)

	c, err := smtp.NewClient(conn, set.SMTPHost)
	if err != nil {
		return smtpError("accueil du serveur", err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Hello("hub"); err != nil {
		return smtpError("présentation", err)
	}
	if set.SMTPPort != 465 {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("le serveur ne propose pas STARTTLS : envoi en clair refusé")
		}
		if err := c.StartTLS(cfg); err != nil {
			return smtpError("STARTTLS", err)
		}
	}
	if set.SMTPUsername != "" {
		if err := c.Auth(smtp.PlainAuth("", set.SMTPUsername, password.Reveal(), set.SMTPHost)); err != nil {
			return smtpError("identifiants", err)
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return smtpError("expéditeur", err)
	}
	for _, a := range to {
		if err := c.Rcpt(a.Address); err != nil {
			return smtpError("destinataire", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return smtpError("envoi", err)
	}
	if _, err := w.Write(mailBody(from, to, m, now)); err != nil {
		return smtpError("envoi", err)
	}
	if err := w.Close(); err != nil {
		return smtpError("envoi", err)
	}
	_ = c.Quit()
	return nil
}

// smtpError says at which step it failed, with the server's code but not its text, which can
// quote an address.
func smtpError(step string, err error) error {
	var proto *textproto.Error
	if errors.As(err, &proto) {
		if step == "identifiants" {
			return fmt.Errorf("identifiants refusés (%d)", proto.Code)
		}
		return fmt.Errorf("%s refusé par le serveur (%d)", step, proto.Code)
	}
	var unknown x509.UnknownAuthorityError
	var host x509.HostnameError
	var verify *tls.CertificateVerificationError
	if errors.As(err, &unknown) || errors.As(err, &host) || errors.As(err, &verify) {
		return errors.New("certificat du serveur refusé")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%s : serveur trop lent à répondre", step)
	}
	if step == "connexion" {
		return errors.New("serveur injoignable")
	}
	return fmt.Errorf("%s : échec", step)
}

func mailBody(from *mail.Address, to []*mail.Address, m Message, now time.Time) []byte {
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	list := make([]string, len(to))
	for i, a := range to {
		list[i] = a.String()
	}
	var b bytes.Buffer
	h := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	h("From", from.String())
	h("To", strings.Join(list, ", "))
	h("Subject", mime.QEncoding.Encode("utf-8", "Hub · "+m.Title))
	h("Date", now.Format(time.RFC1123Z))
	h("Message-ID", "<"+hex.EncodeToString(id)+"@hub>")
	h("MIME-Version", "1.0")
	h("Content-Type", "text/plain; charset=utf-8")
	h("Content-Transfer-Encoding", "quoted-printable")
	h("Auto-Submitted", "auto-generated")
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	text := m.Body
	if text == "" {
		text = m.Title
	}
	_, _ = qp.Write([]byte(strings.ReplaceAll(text, "\n", "\r\n") + "\r\n\r\n— Hub\r\n"))
	_ = qp.Close()
	return b.Bytes()
}
