package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/app"
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
