package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/store"
)

func monitoringFixture(t *testing.T) (*Server, *http.Cookie) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	state := &store.Store{DB: database}
	if err := state.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := state.AddSession(context.Background(), auth.HashToken("private-fixture-session"), "fixture-csrf", time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	return New(&app.App{Store: state}, slog.Default()), &http.Cookie{Name: "arcvpn_session", Value: "private-fixture-session"}
}

func TestMonitoringSettingsRequireSessionCSRFAndPersist(t *testing.T) {
	s, cookie := monitoringFixture(t)
	h := s.Routes()
	request := func(method string, n int, session, csrf bool) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]int{"interval_seconds": n})
		r := httptest.NewRequest(method, "https://panel.example/api/settings/monitoring", bytes.NewReader(body))
		if session {
			r.AddCookie(cookie)
		}
		if csrf {
			r.Header.Set("X-CSRF-Token", "fixture-csrf")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", 0, false, false); w.Code != 401 {
		t.Fatal("anonymous access")
	}
	if w := request("GET", 0, true, false); w.Code != 200 || !strings.Contains(w.Body.String(), `"interval_seconds":5`) {
		t.Fatal(w.Body.String())
	}
	if w := request("PUT", 10, true, false); w.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	if w := request("PUT", 1, true, true); w.Code != 400 {
		t.Fatal("unsafe interval accepted")
	}
	if w := request("PUT", 15, true, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := request("GET", 0, true, false); !strings.Contains(w.Body.String(), `"interval_seconds":15`) {
		t.Fatal("setting lost")
	}
}

func TestMonitoringStreamRefreshReconnectAndSessionRevocation(t *testing.T) {
	s, cookie := monitoringFixture(t)
	server := httptest.NewServer(s.Routes())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/monitoring/events", nil)
	request.AddCookie(cookie)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("not a private SSE stream")
	}
	reader := bufio.NewReader(response.Body)
	frame := func() string {
		t.Helper()
		var out strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			out.WriteString(line)
			if line == "\n" {
				return out.String()
			}
		}
	}
	if got := frame(); !strings.Contains(got, "event: refresh") || !strings.Contains(got, `"interval_seconds":5`) {
		t.Fatal(got)
	}
	if err := s.App.Store.SetMonitoringSeconds(ctx, 30); err != nil {
		t.Fatal(err)
	}
	s.monitoring.notify()
	if got := frame(); !strings.Contains(got, `"interval_seconds":30`) {
		t.Fatal(got)
	}
	if err := s.App.Store.DropSession(ctx, auth.HashToken(cookie.Value)); err != nil {
		t.Fatal(err)
	}
	s.monitoring.notify()
	if got := frame(); !strings.Contains(got, "event: unauthorized") || strings.Contains(got, "interval_seconds") {
		t.Fatal(got)
	}
	if _, err := reader.ReadByte(); err != io.EOF {
		t.Fatal("revoked stream did not close", err)
	}
	// A new connection after a revoke must fail before emitting metrics.
	response2, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response2.Body.Close()
	if response2.StatusCode != 401 {
		t.Fatal("reconnect bypassed revocation")
	}
}

func TestMonitoringHubBoundsAndCoalescesSlowConsumers(t *testing.T) {
	h := newMonitoringHub()
	var channels []chan struct{}
	for i := 0; i < 32; i++ {
		c, ok := h.subscribe()
		if !ok {
			t.Fatal("early limit")
		}
		channels = append(channels, c)
	}
	if _, ok := h.subscribe(); ok {
		t.Fatal("unbounded streams")
	}
	for i := 0; i < 100; i++ {
		h.notify()
	}
	for _, c := range channels {
		if len(c) != 1 {
			t.Fatal("unbounded pending events")
		}
		h.unsubscribe(c)
	}
	if _, ok := h.subscribe(); !ok {
		t.Fatal("slot leaked")
	}
}

func TestConnectionDefaultsUseTrustedOriginAndAvoidConfiguredPortCollisions(t *testing.T) {
	s, cookie := monitoringFixture(t)
	s.App.ReservedTCPPort = 443
	if err := s.WithPublicURL("https://198.51.100.25:443"); err != nil {
		t.Fatal(err)
	}
	h := s.Routes()
	r := httptest.NewRequest("GET", "https://attacker.example/api/vpn/defaults", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"address":"198.51.100.25"`) || !strings.Contains(w.Body.String(), `"vless-reality":444`) || !strings.Contains(w.Body.String(), `"vpn_verified":false`) || strings.Contains(w.Body.String(), "attacker") {
		t.Fatal(w.Body.String())
	}
}
