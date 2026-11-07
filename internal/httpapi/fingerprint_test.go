package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/protocols"
)

func TestFingerprintSettingsAndSubscription(t *testing.T) {
	s, cookie := firstProtocolServer(t)
	ctx := context.Background()
	if err := s.App.SetupWithFingerprint(ctx, "198.51.100.25", "www.example.com", 443, "firefox"); err != nil {
		t.Fatal(err)
	}
	u, err := s.App.AddUser(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	extra, err := s.App.AddProtocolWithFingerprint(ctx, protocols.XHTTP, "198.51.100.25", 8444, "www.example.com", "www.example.com", "safari")
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.App.Store.ReadReality(ctx)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := os.ReadFile(s.App.Core.Path)
	if err != nil {
		t.Fatal(err)
	}
	revision := app.ConnectionRevision(app.RealityProfile(old))
	request := func(method, path, body string, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://example.test"+path, strings.NewReader(body))
		r.AddCookie(cookie)
		if csrf {
			r.Header.Set("X-CSRF-Token", "fixture-csrf")
		}
		w := httptest.NewRecorder()
		s.Routes().ServeHTTP(w, r)
		return w
	}
	edit := func(fp string) string {
		b, _ := json.Marshal(map[string]any{"revision": revision, "port": 443, "target": old.Target, "sni": old.SNI, "enabled": true, "fingerprint": fp})
		return string(b)
	}
	if w := request("PUT", "/api/connections/vless-reality", edit("edge"), false); w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code)
	}
	if w := request("PUT", "/api/connections/vless-reality", edit("unsafe"), true); w.Code != 422 {
		t.Fatal("unsafe fingerprint", w.Code)
	}
	if w := request("PUT", "/api/connections/vless-reality", edit("edge"), true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	got, _ := s.App.Store.ReadReality(ctx)
	after, _ := os.ReadFile(s.App.Core.Path)
	if got.Fingerprint != "edge" || got.Private != old.Private || got.Public != old.Public || got.ShortID != old.ShortID || !bytes.Equal(runtime, after) {
		t.Fatal("client preference changed identity/runtime")
	}
	if w := request("PUT", "/api/connections/vless-reality", edit("firefox"), true); w.Code != 409 {
		t.Fatal("stale preference write", w.Code)
	}
	w := request("GET", fmt.Sprintf("/api/users/%d/subscription", u.ID), "", false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "fp=edge") || !strings.Contains(w.Body.String(), "fp=safari") {
		t.Fatal("subscription lost preference", w.Code, w.Body.String())
	}
	body, _ := json.Marshal(map[string]any{"revision": app.ConnectionRevision(extra), "port": extra.Port, "target": extra.Target, "sni": extra.SNI, "enabled": true, "path": "/custom-path"})
	if w := request("PUT", "/api/connections/vless-xhttp", string(body), true); w.Code != 200 {
		t.Fatal("path edit", w.Code, w.Body.String())
	}
	updated, _ := s.App.Store.GetProtocol(ctx, protocols.XHTTP)
	if updated.Path != "/custom-path" || updated.Fingerprint != "safari" || updated.PrivateKey != extra.PrivateKey || updated.PublicKey != extra.PublicKey {
		t.Fatal("path edit lost fingerprint/identity")
	}
}
