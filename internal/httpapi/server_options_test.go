package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/arcvpn/arcvpn/internal/app"
)

func TestServerOptionsAPIAndClientOnlyEdit(t *testing.T) {
	s, cookie := firstProtocolServer(t)
	ctx := context.Background()
	if err := s.App.Setup(ctx, "198.51.100.25", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	r, err := s.App.Store.ReadReality(ctx)
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"revision": app.ConnectionRevision(app.RealityProfile(r)), "port": r.Port, "sni": r.SNI, "target": r.Target, "enabled": true, "server_options": map[string]any{"sniffing": false}}
	request := func(path, method string, body any, csrf bool) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "https://example.test"+path, bytes.NewReader(b))
		req.AddCookie(cookie)
		if csrf {
			req.Header.Set("X-CSRF-Token", "fixture-csrf")
		}
		w := httptest.NewRecorder()
		s.Routes().ServeHTTP(w, req)
		return w
	}
	if w := request("/api/connections/vless-reality", "PUT", body, false); w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code)
	}
	if w := request("/api/connections/vless-reality", "PUT", body, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	got, err := s.App.Store.ReadReality(ctx)
	if err != nil || got.ServerOptions.SniffingEnabled("vless-reality") || got.Private != r.Private {
		t.Fatal("setting/identity lost", err)
	}
	blob, err := os.ReadFile(s.App.Core.Path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if json.Unmarshal(blob, &cfg) != nil {
		t.Fatal("config invalid")
	}
	if cfg["inbounds"].([]any)[0].(map[string]any)["sniffing"].(map[string]any)["enabled"] != false {
		t.Fatal("runtime not updated")
	}
	body["revision"] = app.ConnectionRevision(app.RealityProfile(got))
	body["fingerprint"] = "firefox"
	if err = os.WriteFile(s.App.Core.Binary, []byte("#!/bin/sh\nexit 73\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if w := request("/api/connections/vless-reality", "PUT", body, true); w.Code != 200 {
		t.Fatal("equal server options restarted engine", w.Code)
	}
	if w := request("/api/connections/vless-reality", "PUT", body, true); w.Code != 409 {
		t.Fatal("stale revision accepted", w.Code)
	}
}
