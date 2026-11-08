package httpapi

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/store"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUserEditorAndBulkAPI(t *testing.T) {
	s, cookie := monitoringFixture(t)
	if err := s.App.Store.InsertUser(context.Background(), store.User{Name: "Alice", UUID: "11111111-1111-4111-8111-111111111111", Subscription: strings.Repeat("t", 43), Enabled: true, CreatedAt: time.Now().Unix()}); err != nil {
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
	path := "/api/users/1/editor"
	body := `{"name":"Unified","expected_version":0,"expected_enabled":true,"enabled":false,"preferences":{"title":"Personal"},"package":{"quota_bytes":0,"expires_at":0,"expected_version":0,"schedule":{"frequency":"monthly","timezone":"Europe/Moscow"}}}`
	if w := request("GET", path, "", false, false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request("PUT", path, body, true, false); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := request("PUT", path, body, true, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("PUT", path, body, true, true); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("GET", path, "", true, false); w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":false`) || strings.Contains(w.Body.String(), strings.Repeat("t", 43)) {
		t.Fatal(w.Code, w.Body.String())
	}
	body = `{"enabled":true,"items":[{"id":1,"expected_version":1,"expected_enabled":false}]}`
	if w := request("POST", "/api/users/bulk-access", body, true, false); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := request("POST", "/api/users/bulk-access", body, true, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("POST", "/api/users/bulk-access", body, true, true); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
}
