package notify

import (
	"context"
	"errors"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"hub/server/internal/secrets"
)

// Status is what the « Notifications » section shows. No secret is in it: a hint of the
// topic, and whether a password is kept.
type Status struct {
	Ntfy       NtfyStatus    `json:"ntfy"`
	SMTP       SMTPStatus    `json:"smtp"`
	Alerts     []AlertStatus `json:"alerts"`
	QuietHours QuietHours    `json:"quietHours"`
	// Pending counts the messages kept for the morning.
	Pending int `json:"pending"`
}

type NtfyStatus struct {
	Configured bool   `json:"configured"`
	URL        string `json:"url"`
	TopicHint  string `json:"topicHint"`
	Last       *Send  `json:"last"`
}

type SMTPStatus struct {
	Configured  bool   `json:"configured"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	PasswordSet bool   `json:"passwordSet"`
	From        string `json:"from"`
	To          string `json:"to"`
	Last        *Send  `json:"last"`
}

type AlertStatus struct {
	ID           string `json:"id"`
	Enabled      bool   `json:"enabled"`
	DelaySeconds int    `json:"delaySeconds"`
}

type QuietHours struct {
	Enabled bool   `json:"enabled"`
	From    string `json:"from"`
	To      string `json:"to"`
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	set, err := s.store.NotifySettings(ctx)
	if err != nil {
		return Status{}, err
	}
	states, err := s.store.ChannelStates(ctx)
	if err != nil {
		return Status{}, err
	}
	digest, err := s.store.Digest(ctx)
	if err != nil {
		return Status{}, err
	}
	out := Status{
		Ntfy: NtfyStatus{Configured: set.NtfyURL != "", URL: set.NtfyURL, TopicHint: set.NtfyHint},
		SMTP: SMTPStatus{Configured: set.SMTPHost != "", Host: set.SMTPHost, Port: set.SMTPPort,
			Username: set.SMTPUsername, From: set.SMTPFrom, To: set.SMTPTo},
		QuietHours: QuietHours{Enabled: set.QuietHours, From: "23:00", To: "08:00"},
		Pending:    len(digest),
	}
	if st, ok := states[Ntfy]; ok && out.Ntfy.Configured {
		out.Ntfy.Last = &st
	}
	if st, ok := states[SMTP]; ok && out.SMTP.Configured {
		out.SMTP.Last = &st
	}
	if set.SMTPUsername != "" {
		_, err := s.secrets.Get(ctx, secretPassword)
		out.SMTP.PasswordSet = err == nil
	}
	for _, a := range Alerts {
		out.Alerts = append(out.Alerts, AlertStatus{ID: a.ID, Enabled: set.enabled(a.ID), DelaySeconds: int(a.Delay.Seconds())})
	}
	return out, nil
}

// Topic names, as docs.ntfy.sh/publish allows them.
var topicRe = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)

// topicHint shows the start of a long topic only: enough to recognise, not to guess.
func topicHint(topic string) string {
	if len(topic) < 12 {
		return ""
	}
	return topic[:4]
}

// SaveNtfy sets the server and the topic. An empty topic keeps the one kept, on the same
// server only: a topic never goes to a server it was not typed for.
func (s *Service) SaveNtfy(ctx context.Context, rawURL, topic string) error {
	u, err := validServerURL(rawURL)
	if err != nil {
		return err
	}
	topic = strings.TrimSpace(topic)
	if topic != "" && !topicRe.MatchString(topic) {
		return invalid("sujet invalide : lettres, chiffres, - et _ seulement, 64 au plus")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	set, err := s.store.NotifySettings(ctx)
	if err != nil {
		return err
	}
	if topic == "" && (set.NtfyURL == "" || set.NtfyURL != u) {
		return invalid("saisir le sujet : il n'est envoyé qu'au serveur pour lequel il a été saisi")
	}
	if topic != "" {
		if err := s.secrets.Put(ctx, secretTopic, secrets.NewValue(topic)); err != nil {
			return err
		}
		set.NtfyHint = topicHint(topic)
	}
	if set.NtfyURL != u || topic != "" {
		// A new destination has not been tried yet.
		if err := s.store.DeleteChannelState(ctx, Ntfy); err != nil {
			return err
		}
	}
	set.NtfyURL = u
	return s.store.SaveNotifySettings(ctx, set)
}

func validServerURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return "", invalid("adresse du serveur vide ou trop longue")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", invalid("adresse du serveur illisible")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", invalid("seules http:// et https:// sont acceptées")
	}
	if u.User != nil {
		return "", invalid("pas d'identifiant dans l'adresse")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", invalid("l'adresse du serveur seule, sans sujet ni paramètre")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// SMTPInput is the form of « E-mail ». An empty password keeps the one kept, for the same
// server and account.
type SMTPInput struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
}

var hostRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*$`)

func (s *Service) SaveSMTP(ctx context.Context, in SMTPInput) error {
	host := strings.TrimSpace(in.Host)
	if host == "" || len(host) > 253 || !hostRe.MatchString(host) && net.ParseIP(host) == nil {
		return invalid("serveur SMTP : un nom d'hôte ou une IP, sans smtp:// ni port")
	}
	if in.Port < 1 || in.Port > 65535 {
		return invalid("port invalide")
	}
	user := strings.TrimSpace(in.Username)
	if len(user) > 256 || strings.IndexFunc(user, unicode.IsControl) >= 0 {
		return invalid("compte invalide")
	}
	if len(in.Password) > 1024 || strings.ContainsAny(in.Password, "\r\n\x00") {
		return invalid("mot de passe invalide")
	}
	from, err := mail.ParseAddress(strings.TrimSpace(in.From))
	if err != nil || len(in.From) > 320 {
		return invalid("expéditeur : une adresse e-mail")
	}
	to, err := mail.ParseAddressList(strings.TrimSpace(in.To))
	if err != nil || len(to) == 0 || len(to) > 5 || len(in.To) > 1024 {
		return invalid("destinataire : une à cinq adresses e-mail, séparées par des virgules")
	}
	list := make([]string, len(to))
	for i, a := range to {
		list[i] = a.Address
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	set, err := s.store.NotifySettings(ctx)
	if err != nil {
		return err
	}
	switch {
	case user == "":
		if err := s.secrets.Delete(ctx, secretPassword); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			return err
		}
	case in.Password != "":
		if err := s.secrets.Put(ctx, secretPassword, secrets.NewValue(in.Password)); err != nil {
			return err
		}
	case set.SMTPHost != host || set.SMTPUsername != user:
		return invalid("saisir le mot de passe : il n'est envoyé qu'au serveur et au compte pour lesquels il a été saisi")
	default:
		if _, err := s.secrets.Get(ctx, secretPassword); err != nil {
			return invalid("saisir le mot de passe")
		}
	}
	if set.SMTPHost != host || set.SMTPPort != in.Port || set.SMTPUsername != user || in.Password != "" {
		if err := s.store.DeleteChannelState(ctx, SMTP); err != nil {
			return err
		}
	}
	set.SMTPHost, set.SMTPPort, set.SMTPUsername = host, in.Port, user
	set.SMTPFrom, set.SMTPTo = from.String(), strings.Join(list, ", ")
	return s.store.SaveNotifySettings(ctx, set)
}

// Remove forgets a channel and its secret.
func (s *Service) Remove(ctx context.Context, channel string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	set, err := s.store.NotifySettings(ctx)
	if err != nil {
		return err
	}
	secret := secretTopic
	switch channel {
	case Ntfy:
		set.NtfyURL, set.NtfyHint = "", ""
	case SMTP:
		secret = secretPassword
		set.SMTPHost, set.SMTPUsername, set.SMTPFrom, set.SMTPTo = "", "", "", ""
	default:
		return invalid("canal inconnu")
	}
	if err := s.secrets.Delete(ctx, secret); err != nil && !errors.Is(err, secrets.ErrNotFound) {
		return err
	}
	if err := s.store.DeleteChannelState(ctx, channel); err != nil {
		return err
	}
	return s.store.SaveNotifySettings(ctx, set)
}

// SaveAlerts turns alerts on or off, and the quiet hours.
func (s *Service) SaveAlerts(ctx context.Context, alerts map[string]bool, quiet bool) error {
	for id := range alerts {
		if _, ok := alertOf(id); !ok {
			return invalid("alerte inconnue : %s", id)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	set, err := s.store.NotifySettings(ctx)
	if err != nil {
		return err
	}
	set.Alerts = map[string]bool{}
	for _, a := range Alerts {
		on, ok := alerts[a.ID]
		if !ok {
			on = set.enabled(a.ID)
		}
		if on != a.Default {
			set.Alerts[a.ID] = on
		}
	}
	set.QuietHours = quiet
	return s.store.SaveNotifySettings(ctx, set)
}

// ErrNotConfigured is a test on a channel not set up.
var ErrNotConfigured = errors.New("canal pas configuré")

// Test sends a test message now, quiet hours or not, and returns how it went.
func (s *Service) Test(ctx context.Context, channel string) error {
	if channel != Ntfy && channel != SMTP {
		return invalid("canal inconnu")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	set, err := s.store.NotifySettings(ctx)
	if err != nil {
		return err
	}
	if !configured(set, channel) {
		return ErrNotConfigured
	}
	m := Message{Title: "Test de Hub", Body: "Si tu lis ceci, les alertes de Hub arriveront ici.", Test: true}
	err = s.sendTo(ctx, set, channel, m)
	s.record(ctx, channel, s.now(), true, err)
	return err
}
