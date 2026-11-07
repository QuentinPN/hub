package notify

import (
	"fmt"
	"strings"
	"time"
)

// Details the store gives, turned into words here.
var details = map[string]string{
	// Hosts.
	"offline":     "l'agent ne répond plus",
	"docker_down": "Docker ne répond plus à l'agent",
	"expired":     "le certificat de l'agent a expiré",
	// Widgets.
	"unreachable":     "service injoignable",
	"auth":            "clé refusée",
	"cert":            "certificat changé",
	"unexpected":      "réponse inattendue",
	"address_changed": "adresse changée, clé à saisir de nouveau",
	// Backups.
	"failed": "la dernière a échoué",
	"late":   "aucune réussie depuis 2 jours",
}

func detailText(d string) string {
	if t, ok := details[d]; ok {
		return t
	}
	return d
}

func alertMessage(inc Incident) Message {
	d := detailText(inc.Detail)
	switch inc.Alert {
	case AlertService:
		return Message{Title: inc.Subject + " est arrêté ou injoignable", Body: capitalize(d) + "."}
	case AlertHost:
		return Message{Title: inc.Subject + " ne répond plus", Body: capitalize(d) + ". Ses services sont inconnus jusqu'à son retour."}
	case AlertWidget:
		return Message{Title: "Connexion de " + inc.Subject + " en échec", Body: capitalize(d) + "."}
	case AlertBackup:
		return Message{Title: "Sauvegarde en échec", Body: capitalize(d) + ". Détail dans Réglages, Sauvegarde."}
	case AlertDisk:
		return Message{Title: "Disque à " + inc.Detail + " % sur " + inc.Subject, Body: "D'après Beszel, au-delà de 90 %."}
	}
	return Message{Title: inc.Subject, Body: d}
}

func recoveryMessage(inc Incident, now time.Time) Message {
	after := "Après " + duration(now.Sub(inc.Since)) + "."
	m := Message{Recovery: true, Body: after}
	switch inc.Alert {
	case AlertService:
		m.Title = inc.Subject + " est revenu"
	case AlertHost:
		m.Title = inc.Subject + " répond de nouveau"
	case AlertWidget:
		m.Title = "Connexion de " + inc.Subject + " rétablie"
	case AlertBackup:
		m.Title = "Sauvegarde réussie de nouveau"
	case AlertDisk:
		m.Title = "Disque sous 90 % sur " + inc.Subject
	default:
		m.Title = inc.Subject + " : revenu"
	}
	return m
}

func duration(d time.Duration) string {
	switch m := int(d.Round(time.Minute) / time.Minute); {
	case m < 1:
		return "moins d'une minute"
	case m < 60:
		return fmt.Sprintf("%d min", m)
	case m < 48*60:
		if m%60 == 0 {
			return fmt.Sprintf("%d h", m/60)
		}
		return fmt.Sprintf("%d h %02d", m/60, m%60)
	default:
		return fmt.Sprintf("%d jours", m/(24*60))
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// DeviceLabel names a device by its browser and system, from its User-Agent: « Firefox sur
// Windows ». A new version of the same browser stays the same device.
func DeviceLabel(ua string) string {
	browser := "Navigateur inconnu"
	for _, b := range []struct{ token, name string }{
		{"Edg/", "Edge"}, {"EdgiOS/", "Edge"}, {"OPR/", "Opera"}, {"SamsungBrowser/", "Samsung Internet"},
		{"Firefox/", "Firefox"}, {"FxiOS/", "Firefox"}, {"CriOS/", "Chrome"}, {"Chrome/", "Chrome"},
		{"Safari/", "Safari"},
	} {
		if strings.Contains(ua, b.token) {
			browser = b.name
			break
		}
	}
	system := "système inconnu"
	for _, o := range []struct{ token, name string }{
		{"iPhone", "iPhone"}, {"iPad", "iPad"}, {"Android", "Android"}, {"CrOS", "ChromeOS"},
		{"Windows", "Windows"}, {"Macintosh", "Mac"}, {"Linux", "Linux"},
	} {
		if strings.Contains(ua, o.token) {
			system = o.name
			break
		}
	}
	if strings.TrimSpace(ua) == "" {
		return "un appareil sans nom"
	}
	return browser + " sur " + system
}
