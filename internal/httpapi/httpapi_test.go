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
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func TestAuthCreateSubscribeDisable(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := &store.Store{DB: database}
	ctx := context.Background()
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("admin-password-really-secret-1234")
	if err != nil {
		t.Fatal(err)
	}
	if err = st.CreateAdmin(ctx, "admin", hash); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "xray")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nif [ \"$1\" = x25519 ];then echo 'PrivateKey: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA';echo 'Password: BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB';exit 0;fi\nif [ \"$1\" = run ];then exit 0;fi\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Store: st, Core: xray.Controller{Binary: binary, Path: filepath.Join(dir, "xray.json"), SkipRestart: true}}
	h := New(a, slog.Default()).Routes()
	request := func(method, path string, body any, csrf string, cookie *http.Cookie) *httptest.ResponseRecorder {
		data := []byte(nil)
		if body != nil {
			data, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, "http://example.test"+path, bytes.NewReader(data))
		req.RemoteAddr = "127.0.0.1:4444"
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	unauthorized := request("GET", "/api/users", nil, "", nil)
	if unauthorized.Code != 401 {
		t.Fatalf("expected 401 got %d", unauthorized.Code)
	}
	login := request("POST", "/api/login", map[string]string{"username": "admin", "password": "admin-password-really-secret-1234"}, "", nil)
	if login.Code != 200 {
		t.Fatalf("login %d %s", login.Code, login.Body.String())
	}
	var response map[string]any
	_ = json.Unmarshal(login.Body.Bytes(), &response)
	csrf := response["csrf"].(string)
	cookie := login.Result().Cookies()[0]
	noCSRF := request("POST", "/api/users", map[string]string{"name": "Phone"}, "", cookie)
	if noCSRF.Code != 403 {
		t.Fatal("CSRF guard disabled")
	}
	setup := request("POST", "/api/vpn", map[string]any{"address": "198.51.100.15", "target": "www.example.com", "port": 443}, csrf, cookie)
	if setup.Code != 201 {
		t.Fatalf("setup %d: %s", setup.Code, setup.Body.String())
	}
	create := request("POST", "/api/users", map[string]string{"name": "Phone"}, csrf, cookie)
	if create.Code != 201 {
		t.Fatalf("create %d: %s", create.Code, create.Body.String())
	}
	var user store.User
	if err := json.Unmarshal(create.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	subscription := request("GET", "/api/users/1/subscription", nil, "", cookie)
	if subscription.Code != 200 {
		t.Fatalf("subscription %d", subscription.Code)
	}
	var sub map[string]string
	_ = json.Unmarshal(subscription.Body.Bytes(), &sub)
	if !strings.Contains(sub["link"], "vless://") || !strings.Contains(sub["url"], "/sub/") {
		t.Fatal("missing subscription")
	}
	token := strings.TrimPrefix(sub["url"], "http://example.test")
	public := request("GET", token, nil, "", nil)
	if public.Code != 200 || !strings.Contains(public.Body.String(), "vless://") {
		t.Fatalf("public subscription %d %s", public.Code, public.Body.String())
	}

	logs := request("GET", "/api/settings/logs", nil, "", cookie)
	if logs.Code != 200 || !strings.Contains(logs.Body.String(), `"budget_mb":500`) {
		t.Fatalf("log settings default: %d %s", logs.Code, logs.Body.String())
	}
	saveLogs := request("POST", "/api/settings/logs", map[string]int{"budget_mb": 200}, csrf, cookie)
	if saveLogs.Code != 200 {
		t.Fatalf("log settings save: %d %s", saveLogs.Code, saveLogs.Body.String())
	}
	logs = request("GET", "/api/settings/logs", nil, "", cookie)
	if !strings.Contains(logs.Body.String(), `"budget_mb":200`) {
		t.Fatal("log limit did not persist")
	}
	invalidLogs := request("POST", "/api/settings/logs", map[string]int{"budget_mb": 9999}, csrf, cookie)
	if invalidLogs.Code != 400 {
		t.Fatal("invalid log budget accepted")
	}
	qr := request("GET", "/api/users/1/qr?format=subscription", nil, "", cookie)
	if qr.Code != 200 || !strings.Contains(qr.Body.String(), "<svg") {
		t.Fatalf("QR not rendered: %d %s", qr.Code, qr.Body.String())
	}
	unauthQR := request("GET", "/api/users/1/qr?format=subscription", nil, "", nil)
	if unauthQR.Code != 401 {
		t.Fatal("QR credentials leaked without session")
	}
	exportedURI := request("GET", "/api/users/1/export?format=raw", nil, "", cookie)
	if exportedURI.Code != 200 || !strings.HasPrefix(exportedURI.Body.String(), "vless://") {
		t.Fatalf("VLESS export: %d", exportedURI.Code)
	}
	backupResult := request("POST", "/api/backup/export", map[string]string{"password": "strong-archive-password-1234"}, csrf, cookie)
	if backupResult.Code != 200 {
		t.Fatalf("backup export: %d %s", backupResult.Code, backupResult.Body.String())
	}
	var exported map[string]string
	if err = json.Unmarshal(backupResult.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(exported["content"], "www.example.com") {
		t.Fatal("plaintext backup credentials exposed")
	}
	preview := request("POST", "/api/backup/preview", map[string]string{"archive": exported["content"], "password": "strong-archive-password-1234"}, csrf, cookie)
	if preview.Code != 200 {
		t.Fatalf("backup preview: %d %s", preview.Code, preview.Body.String())
	}
	wrong := request("POST", "/api/backup/import", map[string]string{"archive": exported["content"], "password": "wrong-password-long", "confirm": "REPLACE_ARCVPN_STATE"}, csrf, cookie)
	if wrong.Code != 422 {
		t.Fatalf("wrong password accepted: %d", wrong.Code)
	}
	missingConfirm := request("POST", "/api/backup/import", map[string]string{"archive": exported["content"], "password": "strong-archive-password-1234"}, csrf, cookie)
	if missingConfirm.Code != 400 {
		t.Fatal("restore without confirmation accepted")
	}
	disable := request("PATCH", "/api/users/1", map[string]bool{"enabled": false}, csrf, cookie)
	if disable.Code != 200 {
		t.Fatalf("disable %d %s", disable.Code, disable.Body.String())
	}
	correctImport := request("POST", "/api/backup/import", map[string]string{"archive": exported["content"], "password": "strong-archive-password-1234", "confirm": "REPLACE_ARCVPN_STATE"}, csrf, cookie)
	if correctImport.Code != 200 {
		t.Fatalf("restore: %d %s", correctImport.Code, correctImport.Body.String())
	}
	restored := request("GET", token, nil, "", nil)
	if restored.Code != 200 || !strings.Contains(restored.Body.String(), "vless://") {
		t.Fatalf("subscription token not preserved: %d", restored.Code)
	}
	// A user disabled again must immediately lose public subscription access.
	disableAgain := request("PATCH", "/api/users/1", map[string]bool{"enabled": false}, csrf, cookie)
	if disableAgain.Code != 200 {
		t.Fatal("cannot disable restored user")
	}
	unavailable := request("GET", token, nil, "", nil)
	if unavailable.Code != 404 {
		t.Fatalf("disabled subscription still works: %d", unavailable.Code)
	}
}

func TestSPAAndUnknownAPI(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := &store.Store{DB: database}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Store: st}
	s := New(a, slog.Default())
	if err := s.WithPublicURL("http://invalid.example/"); err == nil {
		t.Fatal("HTTP public origin accepted")
	}
	if err := s.WithPublicURL("https://198.51.100.4:8443"); err != nil {
		t.Fatal(err)
	}
	handler := s.Routes()
	for _, path := range []string{"/", "/users/42", "/routing"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1"+path, nil))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "ArcVPN") {
			t.Fatalf("broken SPA path %q: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1/api/no-such-handler", nil))
	if rec.Code != 404 || strings.Contains(rec.Body.String(), "ArcVPN") {
		t.Fatal("unknown API returned SPA instead of 404")
	}
}
