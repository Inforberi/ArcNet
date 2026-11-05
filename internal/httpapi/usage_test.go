package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
)

func TestPackageAPIAuthCSRFUnknownUsageAndBlockedExports(t *testing.T) {
	s, cookie := monitoringFixture(t)
	ctx := context.Background()
	u := store.User{Name: "Alice", UUID: "11111111-1111-4111-8111-111111111111", Subscription: strings.Repeat("t", 43), Enabled: true, CreatedAt: time.Now().Unix()}
	if err := s.App.Store.InsertUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	users, _ := s.App.Store.ListUsers(ctx)
	u = users[0]
	if err := s.App.Store.SaveReality(ctx, store.Reality{Address: "198.51.100.25", Port: 443, Target: "www.example.com", SNI: "www.example.com", Public: strings.Repeat("B", 43), Private: strings.Repeat("A", 43), ShortID: "0123456789abcdef", Enabled: true}); err != nil {
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
	if w := request("GET", "/api/users/usage", "", false, false); w.Code != 401 {
		t.Fatal("anonymous usage")
	}
	w := request("GET", "/api/users/usage", "", true, false)
	if w.Code != 200 || strings.Contains(w.Body.String(), u.UUID) || strings.Contains(w.Body.String(), u.Subscription) || !strings.Contains(w.Body.String(), `"observed_at":0`) {
		t.Fatal("unknown or credentials mishandled", w.Code)
	}
	body := `{"quota_bytes":1000,"expires_at":0,"expected_version":0,"reset_usage":false}`
	if w := request("PUT", "/api/users/1/package", body, true, false); w.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	if w := request("PUT", "/api/users/1/package", body, true, true); w.Code != 422 {
		t.Fatal("finite quota without counters accepted", w.Code)
	}
	if err := s.App.Store.SavePackage(ctx, u.ID, 0, time.Now().Unix()-1, 0, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/users/1/subscription", "/api/users/1/export", "/api/users/1/qr"} {
		if w := request("GET", path, "", true, false); w.Code != 404 {
			t.Fatal("expired export allowed", path, w.Code)
		}
	}
	if w := request("GET", "/sub/"+u.Subscription, "", false, false); w.Code != http.StatusNotFound {
		t.Fatal("expired subscription allowed")
	}
}
