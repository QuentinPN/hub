package agentproto

import (
	"reflect"
	"strings"
	"testing"
)

const id = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestLabelAllowed(t *testing.T) {
	cases := map[string]bool{
		"com.docker.compose.project":                          true,
		"com.docker.compose.service":                          true,
		"com.docker.compose.project.working_dir":              true,
		"org.opencontainers.image.version":                    true,
		"homepage.href":                                       true,
		"hub.url":                                             true,
		"homepage.widget.key":                                 false,
		"homepage.widget.password":                            false,
		"hub.api_token":                                       false,
		"traefik.http.middlewares.auth.basicauth.users":       false,
		"traefik.http.routers.app.rule":                       false,
		"com.example.secret":                                  false,
		"COM.DOCKER.COMPOSE.PROJECT":                          false,
		"com.docker.compose.":                                 true,
		"com.docker.composeX.project":                         false,
		"homepage.widget.custom.headers.Authorization-Bearer": false,
	}
	for k, want := range cases {
		if got := LabelAllowed(k); got != want {
			t.Errorf("%s: %v, want %v", k, got, want)
		}
	}
}

func TestSanitizeRefiltersLabels(t *testing.T) {
	c := Container{
		ID: id,
		// An agent that would forget to filter.
		Labels: map[string]string{
			"com.docker.compose.project":                    "media",
			"traefik.http.middlewares.auth.basicauth.users": "admin:$apr1$hash",
		},
		RedactedLabels: []string{"com.docker.compose.project", "other"},
	}
	if err := c.Sanitize(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Labels, map[string]string{"com.docker.compose.project": "media"}) {
		t.Fatalf("labels %v", c.Labels)
	}
	want := []string{"other", "traefik.http.middlewares.auth.basicauth.users"}
	if !reflect.DeepEqual(c.RedactedLabels, want) {
		t.Fatalf("redacted %v", c.RedactedLabels)
	}
}

func TestSanitizeLimits(t *testing.T) {
	bad := []Container{
		{ID: "abc"},
		{ID: strings.ToUpper(id)},
		{ID: id, Name: strings.Repeat("x", maxText+1)},
		{ID: id, Labels: map[string]string{"hub.url": strings.Repeat("x", maxLabelValue+1)}},
		{ID: id, Ports: []Port{{PrivatePort: 70000}}},
	}
	for i, c := range bad {
		if err := c.Sanitize(); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}
