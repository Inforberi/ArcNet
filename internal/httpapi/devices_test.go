package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
)

// Opt-in disposable browser stand. Uses the real middleware/store and synthetic
// credentials; the VPN executable is absent so apply stays visibly pending.
func TestDevicesBrowserFixture(t *testing.T) {
	if os.Getenv("ARCVPN_DEVICE_BROWSER") != "1" {
		t.Skip("opt-in browser stand")
	}
	s, _ := monitoringFixture(t)
	ctx := context.Background()
	if err := s.App.Store.CreateAdmin(ctx, "fixture-admin", "unused-browser-hash"); err != nil {
		t.Fatal(err)
	}
	if err := s.App.Store.InsertUser(ctx, store.User{Name: "Device fixture", UUID: "11111111-1111-4111-8111-111111111111", Subscription: strings.Repeat("t", 43), Enabled: true, CreatedAt: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
	if err := s.App.Store.SaveReality(ctx, store.Reality{Address: "198.51.100.25", Port: 443, Target: "www.example.com", SNI: "www.example.com", Public: strings.Repeat("B", 43), Private: strings.Repeat("A", 43), ShortID: "0123456789abcdef", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("ARCVPN_DEVICE_BROWSER_HYSTERIA") == "1" {
		if err := s.App.Store.UpsertProtocol(ctx, store.Protocol{Kind: "hysteria2", Enabled: true, Address: "198.51.100.25", SNI: "198.51.100.25", Port: 8444, TLSCert: "/etc/arcvpn/tls/fullchain.pem", TLSKey: "/etc/arcvpn/tls/privkey.pem"}); err != nil {
			t.Fatal(err)
		}
		if err := s.App.Store.SaveCredentials(ctx, []store.ProtocolCredential{{UserID: 1, Kind: "hysteria2", Secret: strings.Repeat("a", 43)}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.WithWebRoot("../../frontend/dist"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.Handle("/", s.Routes())
	mux.HandleFunc("POST /fixture-done", func(w http.ResponseWriter, r *http.Request) { done <- struct{}{}; w.WriteHeader(204) })
	server := &http.Server{Addr: "0.0.0.0:18391", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	defer server.Close()
	t.Log("browser fixture ready on :18391")
	select {
	case <-done:
	case err := <-result:
		t.Fatal(err)
	case <-time.After(2 * time.Minute):
		t.Fatal("browser stand timed out")
	}
}

func TestDeviceAPIAuthorizationRetirementAndRevocation(t *testing.T) {
	s, cookie := monitoringFixture(t)
	ctx := context.Background()
	if err := s.App.Store.InsertUser(ctx, store.User{Name: "Alice", UUID: "11111111-1111-4111-8111-111111111111", Subscription: strings.Repeat("t", 43), Enabled: true, CreatedAt: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
	if err := s.App.Store.SaveReality(ctx, store.Reality{Address: "198.51.100.25", Port: 443, Target: "www.example.com", SNI: "www.example.com", Public: strings.Repeat("B", 43), Private: strings.Repeat("A", 43), ShortID: "0123456789abcdef", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	h := s.Routes()
	request := func(method, path, body string, session, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://panel.example"+path, strings.NewReader(body))
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
	if w := request("GET", "/api/users/1/devices", "", false, false); w.Code != 401 {
		t.Fatal("anonymous devices", w.Code)
	}
	body := `{"name":"Phone","expected_version":0}`
	if w := request("POST", "/api/users/1/devices", body, true, false); w.Code != 403 {
		t.Fatal("missing CSRF", w.Code)
	}
	if w := request("POST", "/api/users/1/devices", body, true, true); w.Code != 200 || !strings.Contains(w.Body.String(), `"runtime_state":"apply_failed"`) {
		t.Fatal("policy save/pending apply", w.Code, w.Body.String())
	}
	record, _ := s.App.Store.UserDevices(ctx, 1)
	d := record.Devices[0]
	w := request("GET", "/api/users/1/devices", "", true, false)
	if strings.Contains(w.Body.String(), d.Token) || strings.Contains(w.Body.String(), d.UUID) {
		t.Fatal("list disclosed credentials")
	}
	for _, path := range []string{"/sub/" + strings.Repeat("t", 43), "/api/users/1/export?format=raw", "/api/users/1/qr?format=vless", "/api/users/1/subscription"} {
		if w := request("GET", path, "", true, false); w.Code != 404 {
			t.Fatal("common credential export", path, w.Code)
		}
	}
	w = request("GET", "/api/users/1/devices/"+d.ID+"/links", "", true, false)
	var exported struct {
		URL   string   `json:"url"`
		Links []string `json:"links"`
	}
	if json.Unmarshal(w.Body.Bytes(), &exported) != nil || w.Code != 200 || !strings.HasPrefix(exported.URL, "https://panel.example/device/") || !strings.Contains(exported.Links[0], d.UUID) {
		t.Fatal("device export", w.Code)
	}
	if w := request("GET", "/device/"+d.Token, "", false, false); w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("public device subscription", w.Code)
	}
	if w := request("POST", "/api/users/1/devices", body, true, true); w.Code != 409 {
		t.Fatal("stale CAS", w.Code)
	}
	if w := request("DELETE", "/api/users/1/devices/"+d.ID, `{"expected_version":1}`, true, false); w.Code != 403 {
		t.Fatal("revoke missing CSRF", w.Code)
	}
	if w := request("DELETE", "/api/users/1/devices/"+d.ID, `{"expected_version":1}`, true, true); w.Code != 200 {
		t.Fatal("revoke", w.Code)
	}
	if w := request("GET", "/device/"+d.Token, "", false, false); w.Code != 404 {
		t.Fatal("revoked token accepted", w.Code)
	}
	var logs bytes.Buffer
	s.log = slog.New(slog.NewTextHandler(&logs, nil))
	s.App.Store = nil
	if w := request("GET", "/device/"+d.Token, "", false, false); w.Code != 500 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("panic response", w.Code)
	}
	if strings.Contains(logs.String(), d.Token) || !strings.Contains(logs.String(), "/device/{token}") {
		t.Fatal("panic leaked token or lost route")
	}
}
