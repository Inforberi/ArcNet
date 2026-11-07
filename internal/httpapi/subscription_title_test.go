package httpapi

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSubscriptionTitleIsSafeUTF8AndBounded(t *testing.T) {
	for _, name := range []string{"Леша", "", "\r\nInjected: value", strings.Repeat("я", 100)} {
		w := httptest.NewRecorder()
		setSubscriptionTitle(w, name)
		title, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(w.Header().Get("Profile-Title"), "base64:"))
		if err != nil || !utf8.Valid(title) || utf8.RuneCount(title) > 25 || strings.ContainsAny(string(title), "\r\n") || !strings.HasPrefix(string(title), "ArcNet") {
			t.Fatal("invalid profile title")
		}
	}
}
func TestSubscriptionTitleDeliveredToClient(t *testing.T) {
	s, _ := firstProtocolServer(t)
	ctx := context.Background()
	if _, err := s.App.AddProtocol(ctx, "hysteria2", "198.51.100.25", 8444, "", ""); err != nil {
		t.Fatal(err)
	}
	u, err := s.App.AddUser(ctx, "Леша")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, httptest.NewRequest("GET", "https://example.test/sub/"+u.Subscription, nil))
	if w.Code != 200 || w.Header().Get("Profile-Title") != "base64:"+base64.StdEncoding.EncodeToString([]byte("ArcNet · Леша")) {
		t.Fatal("subscription title not delivered", w.Code)
	}
	if !strings.HasPrefix(w.Body.String(), "hysteria2://") {
		t.Fatal("URI body contract changed")
	}
}
