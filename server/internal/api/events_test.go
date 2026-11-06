package api_test

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEventsNameTheServiceAndNeedASession(t *testing.T) {
	e, _, _ := widgetEnv(t, fakeEmbyServer(t).URL)

	resp, err := e.anon.Get(e.srv.URL + "/api/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("without a session: %d", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.srv.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = e.user.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	lines := bufio.NewScanner(resp.Body)
	if !lines.Scan() || !strings.HasPrefix(lines.Text(), ":") {
		t.Fatalf("no opening comment: %q", lines.Text())
	}

	testBus.Publish("emby")
	var got []string
	for len(got) < 2 && lines.Scan() {
		if line := lines.Text(); line != "" {
			got = append(got, line)
		}
	}
	if len(got) != 2 || got[0] != "event: widgets" || got[1] != "data: emby" {
		t.Errorf("%v (%v)", got, lines.Err())
	}
}
