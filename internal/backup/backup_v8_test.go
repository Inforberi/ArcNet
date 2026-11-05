package backup

import (
	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
	"strings"
	"testing"
)

func TestV8HysteriaCheckpointsRoundTripAndLegacyRejection(t *testing.T) {
	s := sample()
	s.Version = 8
	u := s.Users[0]
	s = s.WithUsage(usage.State{Checkpoints: []usage.Counter{{UserID: u.ID, Identity: u.UUID, Stream: "hysteria2", Epoch: "instance", StartedAt: 1, ObservedAt: 100, Upload: 10, Download: 20}}})
	blob, err := Encrypt(s, "backup-password-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(blob, "backup-password-long-enough")
	if err != nil || got.Version != 8 || got.Usage == nil || got.Usage.Checkpoints[0].Download != 20 {
		t.Fatal("checkpoint lost", err)
	}
	s.Version = 7
	if s.Validate() == nil {
		t.Fatal("Hysteria accounting accepted in legacy payload")
	}
	s.Usage = nil
	s = s.WithDevices(devices.State{{UserID: u.ID, Identity: u.UUID, Version: 1}})
	s.Protocols = []Protocol{{Kind: "hysteria2", Address: s.VPN.Address, SNI: s.VPN.Address, Port: 8444, TLSCert: "/etc/arcvpn/tls/fullchain.pem", TLSKey: "/etc/arcvpn/tls/privkey.pem", Enabled: true}}
	s.Credentials = []store.ProtocolCredential{{UserID: u.ID, Kind: "hysteria2", Secret: strings.Repeat("a", 43)}}
	s.Version = 8
	if err := s.Validate(); err != nil {
		t.Fatal("valid v8 Hysteria device mode rejected", err)
	}
	s.Version = 7
	if s.Validate() == nil {
		t.Fatal("new Hysteria device capability mislabeled as v7")
	}
}
