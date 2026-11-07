package api_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"hub/server/internal/api"
)

func TestReadyzIsPublicAndSaysOnlyReady(t *testing.T) {
	e, _, _ := widgetEnv(t, "")
	status, body := e.do(t, e.anon, http.MethodGet, "/readyz", "", nil)
	if status != http.StatusOK || string(body) != "ok\n" {
		t.Errorf("%d %q", status, body)
	}
}

func TestStatusNeedsASessionAndCountsProblems(t *testing.T) {
	e, _, _ := widgetEnv(t, "http://192.0.2.1:1")
	if status, _ := e.do(t, e.anon, http.MethodGet, "/api/v1/status", "", nil); status != http.StatusUnauthorized {
		t.Errorf("without a session: %d", status)
	}
	var st api.HubStatus
	e.sendJSON(t, http.MethodGet, "/api/v1/status", "", &st)
	ids := []string{}
	for _, c := range st.Checks {
		ids = append(ids, c.ID+":"+c.Level)
	}
	// The address of widgetEnv was saved as answering: nothing is wrong yet.
	if strings.Join(ids, " ") != "db:ok agents:ok widgets:ok urls:ok" || st.Problems != 0 {
		t.Errorf("%v %d", ids, st.Problems)
	}
	if st.MigrationsApplied == 0 || st.MigrationsApplied != st.MigrationsExpected {
		t.Errorf("%+v", st)
	}
}

func TestRequestsAreLoggedWithoutQueryOrCookie(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)

	e, _, _ := widgetEnv(t, "")
	_, _ = e.do(t, e.user, http.MethodGet, "/api/v1/services?secret=chut", "", nil)
	_, _ = e.do(t, e.anon, http.MethodGet, "/readyz", "", nil)
	time.Sleep(10 * time.Millisecond)

	out := buf.String()
	if !strings.Contains(out, "path=/api/v1/services") || !strings.Contains(out, "status=200") {
		t.Errorf("request not logged: %s", out)
	}
	if strings.Contains(out, "chut") || strings.Contains(out, "hub_session") {
		t.Errorf("query or cookie in the logs: %s", out)
	}
	if strings.Contains(out, "path=/readyz") {
		t.Errorf("a passing health check is noise: %s", out)
	}
}
