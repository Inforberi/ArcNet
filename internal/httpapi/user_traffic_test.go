package httpapi

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUserTrafficAPIIsCachedBoundedAndPrivate(t *testing.T) {
	s, cookie := monitoringFixture(t)
	ctx := context.Background()
	user := store.User{Name: "Alice", UUID: "11111111-1111-4111-8111-111111111111", Subscription: strings.Repeat("t", 43), Enabled: true, CreatedAt: time.Now().Unix()}
	if err := s.App.Store.InsertUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	users, err := s.App.Store.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	user = users[0]
	at := time.Now().Unix()
	owners := []usage.TrafficUser{{ID: user.ID, Identity: user.UUID, Kinds: []string{"vless-reality"}}}
	for _, timestamp := range []int64{at - 5, at} {
		s.App.UserTraffic.Update(owners, []usage.Counter{{UserID: user.ID, Identity: user.UUID, Stream: "vless-reality", Epoch: "one", StartedAt: 1, ObservedAt: timestamp, Upload: timestamp * 10, Download: timestamp * 20}}, timestamp, true)
	}
	h := s.Routes()
	get := func(query string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "https://panel.example/api/users/traffic"+query, nil)
		if auth {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := get("?ids=1", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	w := get("?ids=1&history_id=1", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"upload_bps":10`) || !strings.Contains(w.Body.String(), `"online":null`) || strings.Contains(w.Body.String(), user.UUID) || strings.Contains(w.Body.String(), user.Subscription) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, query := range []string{"", "?ids=0", "?ids=1,1", "?ids=1&history_id=2", "?ids=" + strings.Repeat("1,", 201) + "2"} {
		if w := get(query, true); w.Code != 400 {
			t.Fatal(query, w.Code)
		}
	}
	if err = s.App.Store.DeleteUser(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	w = get("?ids=1", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatal("deleted user leaked", w.Body.String())
	}
}
