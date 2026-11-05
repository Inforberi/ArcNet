package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestWireGuardDeviceExportRevocationAndPrivacy(t *testing.T) {
	s, cookie := monitoringFixture(t)
	ctx := context.Background()
	if err := s.App.Store.InsertUser(ctx, store.User{Name: "WG user", UUID: "11111111-1111-4111-8111-111111111111", Subscription: strings.Repeat("t", 43), Enabled: true, CreatedAt: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
	p, err := protocols.New(ctx, protocols.WireGuard, "198.51.100.25", 0, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.App.Store.UpsertProtocol(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.App.Store.SaveReality(ctx, store.Reality{Address: p.Address, Port: 443, Target: "www.example.com", SNI: "www.example.com", Public: strings.Repeat("B", 43), Private: strings.Repeat("A", 43), ShortID: "0123456789abcdef", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	h := s.Routes()
	request := func(method, path, body string, session bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://panel.example"+path, strings.NewReader(body))
		if session {
			r.AddCookie(cookie)
			r.Header.Set("X-CSRF-Token", "fixture-csrf")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("POST", "/api/users/1/devices", `{"name":"Phone","expected_version":0}`, true); w.Code != 200 {
		t.Fatal(w.Code)
	}
	record, err := s.App.Store.UserDevices(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	d := record.Devices[0]
	if d.WGPrivate == "" {
		t.Fatal("peer not generated")
	}
	list := request("GET", "/api/users/1/devices", "", true)
	if strings.Contains(list.Body.String(), d.WGPrivate) || strings.Contains(list.Body.String(), d.Token) {
		t.Fatal("credentials leaked in list")
	}
	link := request("GET", "/api/users/1/devices/"+d.ID+"/links", "", true)
	var exported struct {
		URL string `json:"wireguard_url"`
	}
	if json.Unmarshal(link.Body.Bytes(), &exported) != nil || exported.URL != "https://panel.example/device/"+d.Token+"/wireguard" {
		t.Fatal("missing WG download")
	}
	path := "/device/" + d.Token + "/wireguard"
	download := request("GET", path, "", false)
	if download.Code != 200 || !strings.Contains(download.Body.String(), "PrivateKey = "+d.WGPrivate) || !strings.Contains(download.Body.String(), "Address = "+d.WGAddress+"/32") || download.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("incorrect device configuration", download.Code)
	}
	if err := s.App.Store.SavePackage(ctx, 1, 0, 1, 0, false); err != nil {
		t.Fatal(err)
	}
	if w := request("GET", path, "", false); w.Code != 404 {
		t.Fatal("expired config exported")
	}
	if err := s.App.Store.SavePackage(ctx, 1, 0, 0, 1, false); err != nil {
		t.Fatal(err)
	}
	if w := request("DELETE", "/api/users/1/devices/"+d.ID, `{"expected_version":1}`, true); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request("GET", path, "", false); w.Code != 404 {
		t.Fatal("revoked config exported", w.Code)
	}
}
