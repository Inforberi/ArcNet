package httpapi

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/store"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUserBatchAPIAuthAndCAS(t *testing.T) {
	s, cookie := monitoringFixture(t)
	if err := s.App.Store.InsertUser(context.Background(), store.User{Name: "Alice", UUID: "11111111-1111-4111-8111-111111111111", Subscription: strings.Repeat("t", 43), Enabled: true, CreatedAt: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
	h := s.Routes()
	call := func(method, path, body string, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://panel.example"+path, strings.NewReader(body))
		r.AddCookie(cookie)
		if csrf {
			r.Header.Set("X-CSRF-Token", "fixture-csrf")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	body := `{"items":[{"id":1,"expected_version":0,"expected_enabled":true}],"changes":{"group":"Team"}}`
	if w := call("PUT", "/api/users/bulk-edit", body, false); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := call("PUT", "/api/users/bulk-edit", body, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("PUT", "/api/users/bulk-edit", body, true); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("POST", "/api/users/bulk-create", `{"names":["Bob"]}`, false); w.Code != 403 {
		t.Fatal(w.Code)
	}
}
