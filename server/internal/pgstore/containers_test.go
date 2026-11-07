package pgstore_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"hub/server/internal/agentproto"
	"hub/server/internal/api"
)

func TestServiceContainers(t *testing.T) {
	e := newDiscoveryEnv(t, "mini")
	app := e.container("tz", "tz", "org/tz:1", "running")
	app.Status = "Up 3 days (healthy)"
	app.Ports = []agentproto.Port{
		{IP: "0.0.0.0", PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
		{IP: "::", PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
		{PrivatePort: 9000, Type: "tcp"},
	}
	app.Labels["org.example.token"] = "v4lue-n0t-shown"
	app.RedactedLabels = []string{"com.example.secret"}
	worker := e.container("tz", "worker", "org/tz:1", "running")
	worker.Status = "Up 2 seconds (health: starting)"
	db := e.container("tz-db", "postgres", "postgres:18", "exited")
	db.Status = "Exited (0) 6 hours ago"
	other := e.container("other", "other", "org/other:1", "running")
	e.snapshot(t, "mini", app, worker, db, other)
	if err := e.store.UpdateService(e.ctx, "tz", api.ServiceEdit{Name: "TZ", Category: "X", Links: []string{"tz-db"}}); err != nil {
		t.Fatal(err)
	}
	// The worker is recreated: the old one is gone, a new one takes its name.
	worker2 := e.container("tz", "worker", "org/tz:2", "running")
	worker2.Status = "Up 1 second (unhealthy)"
	e.snapshot(t, "mini", app, worker2, db, other)

	res, err := e.store.ServiceContainers(e.ctx, "tz", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.HostID != "mini" || res.HostName != "mini" || res.HostState != api.AgentConnected || res.LastSeenAt == nil {
		t.Fatalf("host %+v", res)
	}
	var got []string
	for _, c := range res.Containers {
		got = append(got, c.Name+" "+c.Role+" "+c.Health+" "+c.StateLabel)
	}
	want := []string{"tz-tz main healthy en cours", "tz-worker support unhealthy en cours", "tz-db-postgres support  arrêté"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("containers %q", got)
	}
	first := res.Containers[0]
	if first.ID != app.ID[:12] || first.Image != "org/tz:1" || first.Project != "tz" || first.Linked ||
		first.ComposeProject != "tz" || first.ComposeService != "tz" || first.FirstSeenAt.IsZero() {
		t.Fatalf("app %+v", first)
	}
	if len(first.Ports) != 1 || first.Ports[0] != (api.PublishedPort{IP: "0.0.0.0", Public: 8080, Private: 80, Type: "tcp"}) {
		t.Fatalf("ports %+v", first.Ports)
	}
	// Two Compose labels and the plain one, plus the redacted key: never a value.
	if first.Labels != 4 || first.MaskedLabels != 1 {
		t.Fatalf("labels %d, masked %d", first.Labels, first.MaskedLabels)
	}
	if body, _ := json.Marshal(res); strings.Contains(string(body), "v4lue-n0t-shown") || strings.Contains(string(body), "com.example") {
		t.Fatalf("a label leaked: %s", body)
	}
	if !res.Containers[2].Linked || res.Containers[2].Project != "tz-db" || len(res.Containers[2].Ports) != 0 {
		t.Fatalf("linked %+v", res.Containers[2])
	}
	if res.RemovedTotal != 1 || len(res.Removed) != 1 || res.Removed[0].ID != worker.ID[:12] ||
		res.Removed[0].RemovedAt == nil || res.Removed[0].Role != "" || res.Removed[0].Health != "starting" {
		t.Fatalf("removed %+v", res.Removed)
	}

	// Past the window, the agent counts as gone; the containers stay as last seen.
	res, err = e.store.ServiceContainers(e.ctx, "tz", time.Now().Add(time.Hour))
	if err != nil || res.HostState != api.AgentOffline || len(res.Containers) != 3 {
		t.Fatalf("offline: %v, %+v", err, res)
	}

	for _, slug := range []string{"nope", "tz-db"} {
		if _, err := e.store.ServiceContainers(e.ctx, slug, time.Now()); err != api.ErrNotFound {
			t.Errorf("%s: %v", slug, err)
		}
	}
}
