package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/qrcode"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestDirectQRSelectsRequestedProtocolAndRejectsStaleSelection(t *testing.T) {
	links := []string{"vless://11111111-1111-4111-8111-111111111111@9.9.9.9:443?security=reality#TV", "hysteria2://existing-secret@1.1.1.1:443?sni=example.com#TV"}
	r := httptest.NewRequest("GET", "https://panel.example/qr?selection="+auth.HashToken(links[1]), nil)
	w := httptest.NewRecorder()
	writeSelectedAccessQR(w, r, links)
	expected, e := qrcode.SVG(links[1])
	if e != nil {
		t.Fatal(e)
	}
	if w.Code != 200 || w.Body.String() != string(expected) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("QR did not encode selected direct connection")
	}
	w = httptest.NewRecorder()
	writeSelectedAccessQR(w, r, links[:1])
	if w.Code != 409 || strings.Contains(w.Body.String(), "existing-secret") {
		t.Fatal("stale selection exported")
	}
}
func TestDeviceDirectQRRequiresSessionAndStopsAfterRevocation(t *testing.T) {
	s, cookie := monitoringFixture(t)
	ctx := context.Background()
	if e := s.App.Store.InsertUser(ctx, store.User{Name: "TV", UUID: "11111111-1111-4111-8111-111111111111", Subscription: strings.Repeat("t", 43), Enabled: true, CreatedAt: time.Now().Unix()}); e != nil {
		t.Fatal(e)
	}
	if e := s.App.Store.SaveReality(ctx, store.Reality{Address: "198.51.100.25", Port: 443, Target: "www.example.com", SNI: "www.example.com", Public: strings.Repeat("B", 43), Private: strings.Repeat("A", 43), ShortID: "0123456789abcdef", Enabled: true}); e != nil {
		t.Fatal(e)
	}
	if e := s.App.ChangeUserDevices(ctx, 1, 0, "TV", "", nil); e != nil {
		t.Fatal(e)
	}
	record, e := s.App.Store.UserDevices(ctx, 1)
	if e != nil {
		t.Fatal(e)
	}
	device := record.Devices[0]
	links, e := s.App.DeviceLinks(ctx, 1, device.ID)
	if e != nil || len(links) != 1 {
		t.Fatal("device direct links", e)
	}
	path := "https://panel.example/api/users/1/devices/" + device.ID + "/qr?selection=" + auth.HashToken(links[0])
	handler := s.Routes()
	subscriptionQR := httptest.NewRequest("GET", "https://panel.example/api/users/1/devices/"+device.ID+"/qr?format=subscription", nil)
	subscriptionQR.AddCookie(cookie)
	subscriptionReply := httptest.NewRecorder()
	handler.ServeHTTP(subscriptionReply, subscriptionQR)
	if subscriptionReply.Code != 200 || subscriptionReply.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("device subscription QR", subscriptionReply.Code)
	}
	r := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("anonymous credential QR")
	}
	r = httptest.NewRequest("GET", path, nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("authenticated device QR", w.Code)
	}
	if e := s.App.ChangeUserDevices(ctx, 1, record.Version, "", device.ID, nil); e != nil {
		t.Fatal(e)
	}
	r = httptest.NewRequest("GET", path, nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("revoked device QR exported", w.Code)
	}
}
