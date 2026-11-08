package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestClientOptionsAPIAndSubscription(t *testing.T) {
	s, cookie := firstProtocolServer(t)
	ctx := context.Background()
	if err := s.App.Setup(ctx, "198.51.100.25", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	u, err := s.App.AddUser(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.App.AddProtocol(ctx, protocols.XHTTP, "198.51.100.25", 2087, "www.example.com", "www.example.com")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.App.Core.Path)
	if err != nil {
		t.Fatal(err)
	}
	// Any accidental engine apply must fail; export-only edits must still succeed.
	if err = os.WriteFile(s.App.Core.Binary, []byte("#!/bin/sh\nexit 73\n"), 0700); err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body any, csrf bool) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "https://example.test"+path, bytes.NewReader(b))
		r.AddCookie(cookie)
		if csrf {
			r.Header.Set("X-CSRF-Token", "fixture-csrf")
		}
		w := httptest.NewRecorder()
		s.Routes().ServeHTTP(w, r)
		return w
	}
	body := map[string]any{"revision": app.ConnectionRevision(p), "port": p.Port, "sni": p.SNI, "target": p.Target, "enabled": true, "client_options": map[string]any{"xhttp_mode": "packet-up"}}
	if w := request("PUT", "/api/connections/vless-xhttp", body, false); w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code)
	}
	if w := request("PUT", "/api/connections/vless-xhttp", body, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	after, err := os.ReadFile(s.App.Core.Path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("client setting changed runtime", err)
	}
	got, err := s.App.Store.GetProtocol(ctx, p.Kind)
	if err != nil || got.ClientOptions.Mode() != "packet-up" || got.PrivateKey != p.PrivateKey {
		t.Fatal("options/identity lost", err)
	}
	if w := request("PUT", "/api/connections/vless-xhttp", body, true); w.Code != 409 {
		t.Fatal("stale revision accepted", w.Code)
	}
	body["revision"] = app.ConnectionRevision(got)
	body["client_options"] = map[string]any{"mtu": 1420}
	if w := request("PUT", "/api/connections/vless-xhttp", body, true); w.Code != 422 {
		t.Fatal("wrong protocol options accepted", w.Code)
	}
	w := request("GET", fmt.Sprintf("/api/users/%d/subscription", u.ID), nil, false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "mode=packet-up") {
		t.Fatal("subscription lost mode", w.Code)
	}
}
