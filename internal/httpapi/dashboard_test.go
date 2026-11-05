package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/arcvpn/arcvpn/internal/metrics"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestDashboardSeparatesNodeServiceVPNAndStaleMetrics(t *testing.T) {
	now := time.Now().Unix()
	cpu := 42.0
	node := store.Node{ID: "node", Name: "server", Online: true, LastSeen: now, XrayState: "active", Control: store.NodeControl{Probe: "unknown"}}
	observed := &store.ObservedTelemetry{ReceivedAt: now, Snapshot: metrics.Snapshot{SampledAt: now, CPUs: 2, CPUPercent: &cpu}}
	selected := remoteDashboard(node, observed)
	if selected.Agent != "online" || selected.Xray != "active" || selected.VPN != "unknown" || selected.Route != "not-verified" || selected.Stale {
		t.Fatalf("false/incorrect health: %#v", selected)
	}
	node.Online = false
	node.Control.Probe = "pass"
	selected = remoteDashboard(node, observed)
	if selected.Xray != "unknown" || selected.VPN != "stale" || !selected.Stale || summarizeServer(selected).CPU != nil {
		t.Fatal("offline node showed healthy service/probe or fresh metrics")
	}
	node.Online = true
	observed.ReceivedAt = now - metrics.FreshForSeconds - 1
	if !remoteDashboard(node, observed).Stale {
		t.Fatal("old telemetry shown as current")
	}
	legacy := remoteDashboard(node, nil)
	if !legacy.Legacy || legacy.Telemetry == nil || legacy.Telemetry.Snapshot.CPUPercent != nil {
		t.Fatal("old agent invented extended telemetry")
	}
}

func TestDashboardAndTelemetryEndpointsEnforceSessionAndNodeIdentity(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	state := &store.Store{DB: database}
	if err = state.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	password := "Strong-admin-password-2026"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if err = state.CreateAdmin(ctx, "admin", hash); err != nil {
		t.Fatal(err)
	}
	enrollment, err := state.CreateEnrollmentForNode(ctx, "telemetry-node", "9.9.9.9", 443)
	if err != nil {
		t.Fatal(err)
	}
	node, secret, err := state.RegisterNode(ctx, enrollment)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(&app.App{Store: state}, slog.Default()).Routes()
	request := func(method, path string, body any, cookie *http.Cookie, bearer string) *httptest.ResponseRecorder {
		blob, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "https://203.0.113.4:8443"+path, bytes.NewReader(blob))
		r.RemoteAddr = "127.0.0.1:34000"
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/api/servers/local/telemetry", nil, nil, ""); w.Code != 401 {
		t.Fatal("anonymous dashboard access")
	}
	login := request("POST", "/api/login", map[string]string{"username": "admin", "password": password}, nil, "")
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	local := request("GET", "/api/servers/local/telemetry", nil, cookie, "")
	if local.Code != 200 || !strings.Contains(local.Body.String(), `"vpn":"unknown"`) || !strings.Contains(local.Body.String(), `"vpn_traffic_verified":false`) {
		t.Fatal(local.Body.String())
	}
	if history, _ := state.TelemetryHistory(ctx, "local"); len(history) != 0 {
		t.Fatal("GET unexpectedly persisted observations")
	}
	snapshot := metrics.Snapshot{SampledAt: time.Now().Unix(), Epoch: "fixture", CPUs: 2}
	body := map[string]any{"version": 1, "node_id": node.ID, "snapshot": snapshot}
	if w := request("POST", "/api/nodes/telemetry", body, nil, "foreign-secret"); w.Code != 401 {
		t.Fatal("foreign metric report accepted")
	}
	if w := request("POST", "/api/nodes/telemetry", body, nil, secret); w.Code != 204 {
		t.Fatal(w.Body.String())
	}
	if w := request("POST", "/api/nodes/telemetry", body, nil, secret); w.Code != 429 {
		t.Fatal("unlimited metric submissions")
	}
	body["node_id"] = "local"
	if w := request("POST", "/api/nodes/telemetry", body, nil, secret); w.Code != 401 {
		t.Fatal("node claimed local server")
	}
	if w := request("GET", "/api/servers/missing/telemetry", nil, cookie, ""); w.Code != 404 {
		t.Fatal("unknown server not rejected")
	}
	overview := request("GET", "/api/overview", nil, cookie, "")
	if overview.Code != 200 || !strings.Contains(overview.Body.String(), "telemetry-node") || strings.Contains(overview.Body.String(), secret) {
		t.Fatal("fleet unavailable or secret exposed")
	}
}

func TestLoginPasswordWorkIsBoundedWithoutChangingHashPolicy(t *testing.T) {
	server := New(&app.App{}, slog.Default())
	server.loginSlots <- struct{}{}
	if accepted, busy := server.verifyLoginPassword("ignored", "ignored"); accepted || !busy {
		t.Fatal("KDF admission limit bypassed")
	}
	<-server.loginSlots
	if accepted, busy := server.verifyLoginPassword("ignored", "invalid-stored-hash"); accepted || busy || len(server.loginSlots) != 0 {
		t.Fatal("KDF slot not released")
	}
}

func TestTelemetryWorkerStopsBeforeDatabaseClose(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	state := &store.Store{DB: database}
	if err = state.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := New(&app.App{Store: state}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); server.RunTelemetry(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if samples, _ := state.TelemetryHistory(context.Background(), "local"); len(samples) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("telemetry collector leaked past shutdown")
	}
	if samples, _ := state.TelemetryHistory(context.Background(), "local"); len(samples) != 1 {
		t.Fatal("collector did not persist its first observation")
	}
}

func TestLocalRecoveryRevokesLiveSessionAcrossDatabaseConnections(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.sqlite")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	state := &store.Store{DB: database}
	if err = state.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = state.CreateAdmin(ctx, "admin", "fixture-old-hash"); err != nil {
		t.Fatal(err)
	}
	token := "fixture-browser-session"
	if err = state.AddSession(ctx, auth.HashToken(token), "csrf", time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	handler := New(&app.App{Store: state}, slog.Default()).Routes()
	get := func() int {
		request := httptest.NewRequest("GET", "https://example.com/api/overview", nil)
		request.AddCookie(&http.Cookie{Name: "arcvpn_session", Value: token})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Code
	}
	if code := get(); code != 200 {
		t.Fatalf("initial session: %d", code)
	}
	recoveryDB, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer recoveryDB.Close()
	if err = (&store.Store{DB: recoveryDB}).RecoverAdmin(ctx, "fixture-new-hash", false); err != nil {
		t.Fatal(err)
	}
	if code := get(); code != 401 {
		t.Fatalf("session survived external recovery without restart: %d", code)
	}
}
