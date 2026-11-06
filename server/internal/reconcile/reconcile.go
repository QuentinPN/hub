// Package reconcile turns the containers an agent reports into logical services: one per
// Compose project on a host, or per container outside Compose. It only reads what it is
// given, so the same containers always give the same result.
package reconcile

import (
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"hub/server/internal/domain"
)

const labelDependsOn = "com.docker.compose.depends_on"

type Container struct {
	Name           string
	Image          string
	State          string
	ComposeProject string
	ComposeService string
	Labels         map[string]string
	// PublicPorts are the published host ports, duplicates included.
	PublicPorts []int
}

type Kind string

const (
	KindService  Kind = "service"
	KindAgent    Kind = "agent"
	KindDatabase Kind = "database"
	KindSelf     Kind = "self"
)

type Role string

const (
	RoleMain    Role = "main"
	RoleSupport Role = "support"
	// RoleOneShot runs once for the others to start, so being stopped says nothing.
	RoleOneShot Role = "oneshot"
)

type Member struct {
	Container
	Role Role
}

// Group is one Compose project, or one container outside Compose.
type Group struct {
	// Key identifies the group on its host and survives container recreations.
	Key      string
	Kind     Kind
	Slug     string
	Name     string
	Monogram string
	Icon     *string
	Color    string
	Category string
	// RecognizedBy says what the name comes from, for the service page.
	RecognizedBy string
	Members      []Member
	Ports        []int
}

// Groups classifies containers, sorted by key.
func Groups(containers []Container) []Group {
	byKey := map[string][]Container{}
	for _, c := range containers {
		key := c.ComposeProject
		if key == "" {
			key = c.Name
		}
		byKey[key] = append(byKey[key], c)
	}
	groups := make([]Group, 0, len(byKey))
	for key, list := range byKey {
		groups = append(groups, classify(key, list))
	}
	slices.SortFunc(groups, func(a, b Group) int { return strings.Compare(a.Key, b.Key) })
	return groups
}

func classify(key string, list []Container) Group {
	slices.SortFunc(list, func(a, b Container) int { return strings.Compare(a.Name, b.Name) })
	g := Group{Key: key, Kind: kindOf(list), Ports: publicPorts(list)}

	oneShots := oneShotServices(list)
	main := mainContainer(key, list, oneShots)
	for i, c := range list {
		role := RoleSupport
		switch {
		case i == main:
			role = RoleMain
		case oneShots[c.ComposeService]:
			role = RoleOneShot
		}
		g.Members = append(g.Members, Member{Container: c, Role: role})
	}

	sig, recognized := signatureOf(key, list, main)
	g.RecognizedBy = recognized
	g.Slug, g.Name, g.Color, g.Category = sig.slug, sig.name, sig.color, sig.category
	if sig.icon != "" {
		icon := sig.icon
		g.Icon = &icon
	}
	g.Monogram = Monogram(g.Name)
	return g
}

func kindOf(list []Container) Kind {
	agents, databases := 0, 0
	for _, c := range list {
		base := imageBase(c.Image)
		if selfImages[base] {
			return KindSelf
		}
		if agentImages[base] {
			agents++
		}
		if databaseImages[base] {
			databases++
		}
	}
	switch len(list) {
	case agents:
		return KindAgent
	case databases:
		return KindDatabase
	}
	return KindService
}

// mainContainer picks whose state is the service's: a recognized application first, then
// the Compose service named like its project, then a published port, then the first left.
func mainContainer(key string, list []Container, oneShots map[string]bool) int {
	candidate := func(c Container) bool {
		base := imageBase(c.Image)
		return !oneShots[c.ComposeService] && !agentImages[base] && !databaseImages[base]
	}
	for i, c := range list {
		if _, ok := applications[imageBase(c.Image)]; ok && candidate(c) {
			return i
		}
	}
	for i, c := range list {
		if candidate(c) && c.ComposeService != "" && (c.ComposeService == key || strings.HasPrefix(key, c.ComposeService+"-")) {
			return i
		}
	}
	for i, c := range list {
		if candidate(c) && len(c.PublicPorts) > 0 {
			return i
		}
	}
	for i, c := range list {
		if candidate(c) {
			return i
		}
	}
	return 0
}

// oneShotServices reads the depends_on labels of Compose, such as
// "init-db:service_completed_successfully:false": a service another one waits to complete
// is a task, whatever its name.
func oneShotServices(list []Container) map[string]bool {
	found := map[string]bool{}
	for _, c := range list {
		for dep := range strings.SplitSeq(c.Labels[labelDependsOn], ",") {
			if name, condition, ok := strings.Cut(dep, ":"); ok && strings.HasPrefix(condition, "service_completed_successfully") {
				found[name] = true
			}
		}
	}
	return found
}

func signatureOf(key string, list []Container, main int) (signature, string) {
	if sig, ok := ownProjects[key]; ok {
		return sig, "projet Compose " + key
	}
	base := imageBase(list[main].Image)
	if sig, ok := applications[base]; ok {
		return sig, "image " + base
	}
	return signature{
		slug:     Slugify(key),
		name:     displayName(key),
		color:    fallbackColors[hash(key)%uint32(len(fallbackColors))],
		category: categoryOther,
	}, "nom " + key
}

// Link counts other's containers as support of g, its own tasks staying tasks.
func (g *Group) Link(other Group) {
	for _, m := range other.Members {
		if m.Role == RoleMain {
			m.Role = RoleSupport
		}
		g.Members = append(g.Members, m)
	}
}

// Status is what the containers say about the service. Nothing to open yet means
// "configuration required", but only once everything runs: an outage always shows.
func (g Group) Status() (domain.Status, string) {
	var stopped []string
	for _, m := range g.Members {
		switch {
		case m.Role == RoleOneShot:
		case m.Role == RoleMain && m.State != "running":
			if m.State == "paused" || m.State == "restarting" {
				return domain.StatusDegraded, m.Name + " : " + StateLabel(m.State)
			}
			return domain.StatusDown, m.Name + " : " + StateLabel(m.State)
		case m.State != "running":
			stopped = append(stopped, m.Name)
		}
	}
	if len(stopped) > 0 {
		return domain.StatusDegraded, strings.Join(stopped, ", ") + " : arrêté"
	}
	return domain.StatusConfigRequired, g.portsNote()
}

func (g Group) portsNote() string {
	switch len(g.Ports) {
	case 0:
		return "aucun port publié"
	case 1:
		return "port " + strconv.Itoa(g.Ports[0])
	}
	parts := make([]string, len(g.Ports))
	for i, p := range g.Ports {
		parts[i] = strconv.Itoa(p)
	}
	return "ports " + strings.Join(parts, ", ")
}

// StateLabel is a Docker state in the words of the UI.
func StateLabel(state string) string {
	switch state {
	case "running":
		return "en cours"
	case "exited", "dead":
		return "arrêté"
	case "paused":
		return "en pause"
	case "restarting":
		return "redémarre"
	case "created":
		return "créé, pas démarré"
	}
	return state
}

func publicPorts(list []Container) []int {
	var ports []int
	for _, c := range list {
		ports = append(ports, c.PublicPorts...)
	}
	slices.Sort(ports)
	return slices.Compact(ports)
}

// imageBase reduces "ghcr.io/org/app:1.2@sha256:abc" to "app".
func imageBase(image string) string {
	image, _, _ = strings.Cut(image, "@")
	if i := strings.LastIndex(image, "/"); i >= 0 {
		image = image[i+1:]
	}
	image, _, _ = strings.Cut(image, ":")
	return strings.ToLower(image)
}

// Slugify keeps what a service slug accepts; never empty.
func Slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	slug := strings.TrimRight(b.String(), "-")
	if slug == "" {
		return "service"
	}
	if len(slug) > 50 {
		return strings.TrimRight(slug[:50], "-")
	}
	return slug
}

func displayName(key string) string {
	words := strings.FieldsFunc(key, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	name := strings.Join(words, " ")
	if name == "" {
		name = "Service"
	}
	if r := []rune(name); len(r) > 80 {
		return string(r[:80])
	}
	return name
}

// Monogram is the letter a service without icon shows.
func Monogram(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(unicode.ToUpper(r))
		}
	}
	return "?"
}

func hash(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}
