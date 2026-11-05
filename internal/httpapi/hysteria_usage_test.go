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
	"github.com/arcvpn/arcvpn/internal/usage"
)

func TestHysteriaDeviceSubscriptionSharesUserQuota(t *testing.T) {
	s, cookie := monitoringFixture(t)
	ctx := context.Background()
	now := time.Now().Unix()
	u := store.User{Name: "Hysteria user", UUID: "11111111-1111-4111-8111-111111111111", Subscription: strings.Repeat("t", 43), Enabled: true, CreatedAt: now}
	if err := s.App.Store.InsertUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	p := store.Protocol{Kind: protocols.Hysteria2, Enabled: true, Address: "198.51.100.25", SNI: "198.51.100.25", Port: 8444, TLSCert: "/etc/arcvpn/tls/fullchain.pem", TLSKey: "/etc/arcvpn/tls/privkey.pem"}
	if err := s.App.Store.UpsertProtocol(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.App.Store.SaveReality(ctx, store.Reality{Address: p.Address, Port: 443, Target: "www.example.com", SNI: "www.example.com", Public: strings.Repeat("B", 43), Private: strings.Repeat("A", 43), ShortID: "0123456789abcdef", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.App.Store.SaveCredentials(ctx, []store.ProtocolCredential{{UserID: 1, Kind: protocols.Hysteria2, Secret: strings.Repeat("a", 43)}}); err != nil {
		t.Fatal(err)
	}
	h := s.Routes()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://panel.example"+path, strings.NewReader(body))
		r.AddCookie(cookie)
		r.Header.Set("X-CSRF-Token", "fixture-csrf")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("POST", "/api/users/1/devices", `{"name":"Phone","expected_version":0}`); w.Code != 200 {
		t.Fatal("Hysteria device rejected", w.Code)
	}
	devices, err := s.App.Store.UserDevices(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	d := devices.Devices[0]
	w := request("GET", "/api/users/1/devices/"+d.ID+"/links", "")
	var out struct {
		Links []string `json:"links"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != 200 || len(out.Links) != 2 {
		t.Fatal("Hysteria subscription missing", w.Code, err)
	}
	if !strings.Contains(out.Links[1], "hysteria2://"+protocols.HysteriaDeviceName(d.ID)) || strings.Contains(out.Links[1], "u1:") {
		t.Fatal("device exported common credential")
	}
	x := usage.Counter{UserID: 1, Identity: u.UUID, Stream: "vless-reality", Epoch: "xray", StartedAt: 1, ObservedAt: now, Upload: 10, Download: 20}
	if err := s.App.Store.RecordUsage(ctx, []usage.Counter{x}, now); err != nil {
		t.Fatal(err)
	}
	hy := x
	hy.Stream = "hysteria2"
	hy.Epoch = "hy"
	hy.Upload = 30
	hy.Download = 40
	if err := s.App.Store.RecordEngineUsage(ctx, "hysteria", []usage.Counter{hy}, now, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.App.Store.SavePackage(ctx, 1, 100, 0, 0, false); err != nil {
		t.Fatal(err)
	}
	if w := request("GET", "/device/"+d.Token, ""); w.Code != 404 {
		t.Fatal("Hysteria bypassed exhausted shared quota", w.Code)
	}
	if w := request("GET", "/api/users/1/devices/"+d.ID+"/links", ""); w.Code != 404 {
		t.Fatal("exhausted device export", w.Code)
	}
}
