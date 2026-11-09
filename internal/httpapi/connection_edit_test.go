package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestConnectionManagementAPI(t *testing.T) {
	s, cookie := firstProtocolServer(t)
	handler := s.Routes()
	request := func(method, path, body string, csrf, session bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://example.test"+path, strings.NewReader(body))
		if session {
			r.AddCookie(cookie)
		}
		if csrf {
			r.Header.Set("X-CSRF-Token", "fixture-csrf")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if err := s.App.Setup(context.Background(), "198.51.100.25", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	w := request("GET", "/api/vpn", "", false, true)
	var current struct {
		Revision string `json:"revision"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &current); err != nil || len(current.Revision) != 64 || !current.Enabled {
		t.Fatal("missing revision/state", w.Body.String(), err)
	}
	body := `{"revision":"` + current.Revision + `","port":2083,"target":"www.apple.com","sni":"www.apple.com","enabled":true}`
	if w = request("PUT", "/api/connections/vless-reality", body, true, false); w.Code != 401 {
		t.Fatal("unauthenticated edit", w.Code)
	}
	if w = request("PUT", "/api/connections/vless-reality", body, false, true); w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code)
	}
	if w = request("PUT", "/api/connections/vless-reality", body, true, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request("PUT", "/api/connections/vless-reality", body, true, true); w.Code != 409 {
		t.Fatal("stale edit", w.Code)
	}
	r, _ := s.App.Store.ReadReality(context.Background())
	rev := app.ConnectionRevision(app.RealityProfile(r))
	if w = request("DELETE", "/api/connections/vless-reality", `{"revision":"`+rev+`"}`, true, true); w.Code != 409 {
		t.Fatal("active delete", w.Code)
	}
	body = `{"revision":"` + rev + `","port":2083,"target":"www.apple.com","sni":"www.apple.com","enabled":false}`
	if w = request("PUT", "/api/connections/vless-reality", body, true, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	r, _ = s.App.Store.ReadReality(context.Background())
	rev = app.ConnectionRevision(app.RealityProfile(r))
	if w = request("DELETE", "/api/connections/vless-reality", `{"revision":"`+rev+`"}`, false, true); w.Code != 403 {
		t.Fatal("delete CSRF", w.Code)
	}
	if w = request("DELETE", "/api/connections/vless-reality", `{"revision":"`+rev+`"}`, true, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request("GET", "/api/vpn", "", false, true); !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Fatal(w.Body.String())
	}
}

func TestAdditionalConnectionDeleteUsesIdentityAndCAS(t *testing.T) {
	s, cookie := firstProtocolServer(t)
	ctx := context.Background()
	if err := s.App.Setup(ctx, "198.51.100.25", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	p, err := s.App.AddProtocol(ctx, "vless-reality", "198.51.100.25", 2443, "www.example.com", "www.example.com")
	if err != nil {
		t.Fatal(err)
	}
	id := p.ConnectionID()
	if !store.IsAdditionalConnection(id) {
		t.Fatal("fixture is not additional")
	}
	if err = s.App.EditConnection(ctx, id, app.ConnectionRevision(p), app.ConnectionSettings{Port: p.Port, SNI: p.SNI, Target: p.Target, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	p, err = s.App.Store.GetProtocol(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	request := func(revision string) int {
		r := httptest.NewRequest("DELETE", "https://example.test/api/connections/"+id, strings.NewReader(`{"revision":"`+revision+`"}`))
		r.AddCookie(cookie)
		r.Header.Set("X-CSRF-Token", "fixture-csrf")
		w := httptest.NewRecorder()
		s.Routes().ServeHTTP(w, r)
		return w.Code
	}
	if code := request(strings.Repeat("0", 64)); code != 409 {
		t.Fatal("stale revision", code)
	}
	if code := request(app.ConnectionRevision(p)); code != 200 {
		t.Fatal("additional delete", code)
	}
	if _, err = s.App.Store.GetProtocol(ctx, id); err != store.ErrNotFound {
		t.Fatal("deleted row remains", err)
	}
	if _, err = s.App.Store.ReadReality(ctx); err != nil {
		t.Fatal("primary connection lost", err)
	}
}
