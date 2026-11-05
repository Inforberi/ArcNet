package backup

import (
	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/usage"
	"testing"
)

func TestV9WireGuardDeviceAndAccounting(t *testing.T) {
	s := sample()
	u := s.Users[0]
	priv, pub, _ := devices.NewWireGuardKeys()
	state := devices.State{{UserID: u.ID, Identity: u.UUID, Version: 1, Devices: []devices.Device{{ID: "22222222-2222-4222-8222-222222222222", UUID: "33333333-3333-4333-8333-333333333333", Token: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Name: "Phone", CreatedAt: 100, WGPrivate: priv, WGPublic: pub, WGAddress: "10.76.255.254"}}}}
	s = s.WithDevices(state).WithUsage(usage.State{Checkpoints: []usage.Counter{{UserID: u.ID, Identity: u.UUID, Stream: "wireguard", Epoch: "boot/interface/nonce", StartedAt: 1, ObservedAt: 100, Upload: 10, Download: 20}}})
	blob, err := Encrypt(s, "backup-password-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(blob, "backup-password-long-enough")
	if err != nil || got.Version != 9 || (*got.Devices)[0].Devices[0].WGPrivate != priv || got.Usage.Checkpoints[0].Download != 20 {
		t.Fatal("peer or usage lost", err)
	}
	s.Version = 8
	if s.Validate() == nil {
		t.Fatal("new WG capability mislabeled as v8")
	}
}
