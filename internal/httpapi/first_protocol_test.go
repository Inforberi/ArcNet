package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/engines"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func firstProtocolServer(t *testing.T) (*Server, *http.Cookie) {
	t.Helper()
	s, cookie := monitoringFixture(t)
	dir := t.TempDir()
	binary := filepath.Join(dir, "xray")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nif [ \"$1\" = x25519 ]; then echo 'PrivateKey: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'; echo 'Password: BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB'; exit 0; fi\nif [ \"$1\" = run ]; then exit 0; fi\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	s.App.Core = xray.Controller{Binary: binary, Path: filepath.Join(dir, "xray.json"), SkipRestart: true}
	s.App.Engines = &engines.Manager{Dir: filepath.Join(dir, "engines"), SkipRestart: true}
	if err := s.App.Store.CreateAdmin(context.Background(), "fixture-admin", "unused-fixture-hash"); err != nil {
		t.Fatal(err)
	}
	if err := s.WithPublicURL("https://198.51.100.25:8443"); err != nil {
		t.Fatal(err)
	}
	return s, cookie
}

func TestIndependentProtocolAPIAndSubscription(t *testing.T) {
	s, cookie := firstProtocolServer(t)
	h := s.Routes()
	request := func(method, path, body string, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://panel.example"+path, strings.NewReader(body))
		r.AddCookie(cookie)
		if csrf {
			r.Header.Set("X-CSRF-Token", "fixture-csrf")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	body := `{"kind":"hysteria2","address":"198.51.100.25","port":8444}`
	if w := request("POST", "/api/protocols", body, false); w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code)
	}
	if w := request("POST", "/api/protocols", body, true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("GET", "/api/vpn", "", false); w.Code != 200 || !strings.Contains(w.Body.String(), `"configured":false`) || !strings.Contains(w.Body.String(), `"address":"198.51.100.25"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("POST", "/api/users", `{"name":"First user"}`, true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/users/1/subscription", "/api/users/1/connections", "/api/users/1/export?format=raw"} {
		w := request("GET", path, "", false)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "hysteria2://") || strings.Contains(w.Body.String(), "vless://") {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
}

func TestIndependentWireGuardSubscriptionHasNoFabricatedURI(t *testing.T) {
	s, cookie := firstProtocolServer(t)
	ctx := context.Background()
	if _, err := s.App.AddProtocol(ctx, "wireguard", "198.51.100.25", 0, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.App.AddUser(ctx, "WG only"); err != nil {
		t.Fatal(err)
	}
	h := s.Routes()
	for _, path := range []string{"/api/users/1/subscription", "/api/users/1/qr?format=vless"} {
		r := httptest.NewRequest("GET", "https://panel.example"+path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if strings.Contains(path, "/qr?") {
			if w.Code != 409 {
				t.Fatal("empty direct QR accepted", w.Code)
			}
		} else if w.Code != 200 || !strings.Contains(w.Body.String(), `"link":""`) || !strings.Contains(w.Body.String(), `"links":[]`) || strings.Contains(w.Body.String(), "vless://") {
			t.Fatal("fabricated URI", w.Code, w.Body.String())
		}
	}
}

// Real HTTP/UI with disposable state and simulated engines, never a VPN proof.
func TestFirstProtocolBrowserFixture(t *testing.T) {
	if os.Getenv("ARCVPN_FIRST_PROTOCOL_BROWSER") != "1" {
		t.Skip("opt-in browser fixture")
	}
	s, _ := firstProtocolServer(t)
	if err := s.WithWebRoot("../../frontend/dist"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.Handle("/", s.Routes())
	mux.HandleFunc("POST /fixture-done", func(w http.ResponseWriter, r *http.Request) {
		select {
		case done <- struct{}{}:
		default:
		}
		w.WriteHeader(204)
	})
	server := &http.Server{Addr: "0.0.0.0:18392", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	defer server.Close()
	t.Log("first protocol browser fixture ready on :18392")
	select {
	case <-done:
	case err := <-result:
		t.Fatal(err)
	case <-time.After(2 * time.Minute):
		t.Fatal("browser fixture timed out")
	}
}
