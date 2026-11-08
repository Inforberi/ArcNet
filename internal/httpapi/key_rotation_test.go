package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/protocols"
)

func TestKeyRotationAPIRequiresPlanAndCSRF(t *testing.T) {
	s, cookie := firstProtocolServer(t)
	ctx := context.Background()
	p, err := s.App.AddProtocol(ctx, protocols.WireGuard, "198.51.100.1", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.App.AddUser(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body any, auth, csrf bool) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "https://example.test"+path, bytes.NewReader(data))
		if auth {
			r.AddCookie(cookie)
		}
		if csrf {
			r.Header.Set("X-CSRF-Token", "fixture-csrf")
		}
		w := httptest.NewRecorder()
		s.Routes().ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/api/connections/wireguard/rotation", nil, false, false); w.Code != 401 {
		t.Fatal("unauthenticated plan", w.Code)
	}
	w := request("GET", "/api/connections/wireguard/rotation", nil, true, false)
	var plan app.KeyRotationPlan
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil || plan.Affected != 1 || strings.Contains(w.Body.String(), p.PrivateKey) {
		t.Fatal("invalid preview", w.Code)
	}
	body := map[string]any{"revision": plan.Revision, "confirmation": plan.Confirmation}
	if w = request("POST", "/api/connections/wireguard/rotate", body, true, false); w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code)
	}
	if w = request("POST", "/api/connections/wireguard/rotate", body, true, true); w.Code != 200 {
		t.Fatal("rotation failed", w.Code)
	}
	if w = request("POST", "/api/connections/wireguard/rotate", body, true, true); w.Code != 409 {
		t.Fatal("replay accepted", w.Code)
	}
	got, _ := s.App.Store.GetProtocol(ctx, p.Kind)
	w = request("GET", fmt.Sprintf("/api/users/%d/connections", u.ID), nil, true, false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), got.PublicKey) || got.PublicKey == p.PublicKey {
		t.Fatal("new key not exported", w.Code)
	}
}
