package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/engines"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func TestStage2APIAuthorizationAndExports(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, e := db.Open(filepath.Join(dir, "panel.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer database.Close()
	st := &store.Store{DB: database}
	if e = st.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	hash, e := auth.HashPassword("test-password-long-enough-2026")
	if e != nil {
		t.Fatal(e)
	}
	if e = st.CreateAdmin(ctx, "admin", hash); e != nil {
		t.Fatal(e)
	}
	stub := filepath.Join(dir, "xray")
	if e = os.WriteFile(stub, []byte("#!/bin/sh\nif [ \"$1\" = x25519 ];then echo 'PrivateKey: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA';echo 'Password: BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB';exit 0;fi\nif [ \"$1\" = run ];then exit 0;fi\nexit 1\n"), 0755); e != nil {
		t.Fatal(e)
	}
	a := &app.App{Store: st, Core: xray.Controller{Binary: stub, Path: filepath.Join(dir, "xray.json"), SkipRestart: true}, Engines: &engines.Manager{Dir: filepath.Join(dir, "engines"), SkipRestart: true}}
	handler := New(a, slog.Default()).Routes()
	req := func(method, path string, body any, csrf string, cookie *http.Cookie) *httptest.ResponseRecorder {
		var b []byte
		if body != nil {
			b, _ = json.Marshal(body)
		}
		request := httptest.NewRequest(method, "https://vpn.example"+path, bytes.NewReader(b))
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		if csrf != "" {
			request.Header.Set("X-CSRF-Token", csrf)
		}
		if cookie != nil {
			request.AddCookie(cookie)
		}
		request.RemoteAddr = "192.0.2.1:5400"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, request)
		return rec
	}
	login := req("POST", "/api/login", map[string]string{"username": "admin", "password": "test-password-long-enough-2026"}, "", nil)
	if login.Code != 200 {
		t.Fatalf("login: %s", login.Body.String())
	}
	var session struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &session)
	cookie := login.Result().Cookies()[0]
	setup := req("POST", "/api/vpn", map[string]any{"address": "198.51.100.71", "target": "www.example.com", "port": 443}, session.CSRF, cookie)
	if setup.Code != 201 {
		t.Fatalf("Reality: %d %s", setup.Code, setup.Body.String())
	}
	create := req("POST", "/api/users", map[string]string{"name": "Phone"}, session.CSRF, cookie)
	if create.Code != 201 {
		t.Fatalf("new user: %d %s", create.Code, create.Body.String())
	}
	noCSRF := req("POST", "/api/protocols", map[string]string{"kind": "hysteria2"}, "", cookie)
	if noCSRF.Code != 403 {
		t.Fatal("protocol modifications allowed without CSRF")
	}
	noAuth := req("GET", "/api/protocols", nil, "", nil)
	if noAuth.Code != 401 {
		t.Fatal("protocol information exposed to anonymous user")
	}
	for _, kind := range []string{"vless-xhttp", "hysteria2", "wireguard", "amneziawg"} {
		protocol := req("POST", "/api/protocols", map[string]any{"kind": kind, "address": "198.51.100.71"}, session.CSRF, cookie)
		if protocol.Code != 201 {
			t.Fatalf("protocol %s: %d %s", kind, protocol.Code, protocol.Body.String())
		}
	}
	profiles := req("GET", "/api/protocols", nil, "", cookie)
	if profiles.Code != 200 {
		t.Fatal("protocols listing failed")
	}
	if strings.Contains(profiles.Body.String(), `"private_key"`) || strings.Contains(profiles.Body.String(), `"obfs_key"`) {
		t.Fatal("server keys leaked through protocol list")
	}
	connections := req("GET", "/api/users/1/connections", nil, "", cookie)
	if connections.Code != 200 || !strings.Contains(connections.Body.String(), "AmneziaWG 3.1") || !strings.Contains(connections.Body.String(), "hysteria2://") {
		t.Fatalf("connections: %d %s", connections.Code, connections.Body.String())
	}
	exported := req("GET", "/api/users/1/connections/wireguard/export", nil, "", cookie)
	if exported.Code != 200 || !strings.Contains(exported.Body.String(), "[Interface]") || !strings.Contains(exported.Body.String(), "PrivateKey") {
		t.Fatal("WireGuard config export failed")
	}
	qr := req("GET", "/api/users/1/connections/amneziawg/export?format=qr", nil, "", cookie)
	if qr.Code != 200 || !strings.Contains(qr.Body.String(), "<svg") {
		t.Fatalf("Amnezia QR export failed: %d %s", qr.Code, qr.Body.String())
	}
	noAuthExport := req("GET", "/api/users/1/connections/wireguard/export", nil, "", nil)
	if noAuthExport.Code != 401 {
		t.Fatal("private key accessible without authorization")
	}
	subscription := req("GET", "/api/users/1/subscription", nil, "", cookie)
	if subscription.Code != 200 || !strings.Contains(subscription.Body.String(), "hysteria2://") {
		t.Fatalf("subscription: %d %s", subscription.Code, subscription.Body.String())
	}
	backupExport := req("POST", "/api/backup/export", map[string]string{"password": "backup-password-long-enough-123"}, session.CSRF, cookie)
	if backupExport.Code != 200 {
		t.Fatalf("backup: %d %s", backupExport.Code, backupExport.Body.String())
	}
	var envelope map[string]string
	_ = json.Unmarshal(backupExport.Body.Bytes(), &envelope)
	preview := req("POST", "/api/backup/preview", map[string]string{"archive": envelope["content"], "password": "backup-password-long-enough-123"}, session.CSRF, cookie)
	if preview.Code != 200 {
		t.Fatalf("backup preview: %d %s", preview.Code, preview.Body.String())
	}
	restore := req("POST", "/api/backup/import", map[string]string{"archive": envelope["content"], "password": "backup-password-long-enough-123", "confirm": "REPLACE_ARCVPN_STATE"}, session.CSRF, cookie)
	if restore.Code != 200 {
		t.Fatalf("backup restore: %d %s", restore.Code, restore.Body.String())
	}
	disable := req("PATCH", "/api/users/1", map[string]bool{"enabled": false}, session.CSRF, cookie)
	if disable.Code != 200 {
		t.Fatalf("disable user: %d %s", disable.Code, disable.Body.String())
	}
	deniedConnections := req("GET", "/api/users/1/connections", nil, "", cookie)
	if deniedConnections.Code != 200 || !strings.Contains(deniedConnections.Body.String(), `"items":[]`) {
		t.Fatalf("disabled user still lists connections: %d %s", deniedConnections.Code, deniedConnections.Body.String())
	}
	if deniedConnections.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("disabled user connections must not be cached")
	}
	for _, endpoint := range []string{
		"/api/users/1/subscription",
		"/api/users/1/qr?format=vless",
		"/api/users/1/qr?format=subscription",
		"/api/users/1/export?format=raw",
		"/api/users/1/export?format=base64",
		"/api/users/1/connections/wireguard/export",
	} {
		export := req("GET", endpoint, nil, "", cookie)
		if export.Code != 404 && export.Code != 409 {
			t.Errorf("disabled VPN user can still export active secrets from %s: HTTP %d", endpoint, export.Code)
		}
		if strings.Contains(export.Body.String(), "[Interface]") || strings.Contains(export.Body.String(), "vless://") {
			t.Errorf("disabled VPN user export leaked a usable connection from %s", endpoint)
		}
	}
}
