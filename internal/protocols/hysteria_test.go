package protocols

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestHysteriaDeviceCredentialsReplaceCommonAccess(t *testing.T) {
	d := devices.Device{ID: "22222222-2222-4222-8222-222222222222", UUID: "33333333-3333-4333-8333-333333333333"}
	retired := d
	retired.ID = "44444444-4444-4444-8444-444444444444"
	retired.RevokedAt = 100
	u := store.User{ID: 1, Enabled: true, DeviceAccess: &devices.Record{Devices: []devices.Device{d, retired}}}
	data, err := renderHysteria(store.Protocol{Port: 8444}, []store.User{u}, []store.ProtocolCredential{{UserID: 1, Kind: Hysteria2, Secret: strings.Repeat("a", 43)}})
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Auth struct {
			Users map[string]string `json:"userpass"`
		} `json:"auth"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Auth.Users) != 1 || config.Auth.Users[HysteriaDeviceName(d.ID)] != d.UUID {
		t.Fatal("common or retired credential allowed")
	}
	u.Enabled = false
	data, err = renderHysteria(store.Protocol{Port: 8444}, []store.User{u}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), d.UUID) {
		t.Fatal("disabled user still allowed")
	}
}
