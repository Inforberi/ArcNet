package httpapi

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/store"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUserPreferencesAPIAuthCSRFAndTitle(t *testing.T) {
	s, cookie := monitoringFixture(t)
	ctx := context.Background()
	if err := s.App.Store.InsertUser(ctx, store.User{Name: "Alice", UUID: "11111111-1111-4111-8111-111111111111", Subscription: strings.Repeat("t", 43), Enabled: true, CreatedAt: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
	h := s.Routes()
	request := func(method, body string, session, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://panel.example/api/users/1/preferences", strings.NewReader(body))
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
	body := `{"name":"New name","expected_version":0,"preferences":{"title":"Personal","group":"Team","access_mode":"subscription","allowed_kinds":null}}`
	if w := request("GET", "", false, false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request("PUT", body, true, false); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := request("PUT", body, true, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("PUT", body, true, true); w.Code != 409 {
		t.Fatal("stale accepted", w.Code)
	}
	w := request("GET", "", true, false)
	if w.Code != 200 || strings.Contains(w.Body.String(), strings.Repeat("t", 43)) || !strings.Contains(w.Body.String(), "Personal") {
		t.Fatal(w.Code, w.Body.String())
	}
	u, _ := s.App.Store.ByID(ctx, 1)
	if userSubscriptionTitle(u) != "Personal" {
		t.Fatal("title not used")
	}
}
