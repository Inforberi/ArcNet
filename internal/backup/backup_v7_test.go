package backup

import (
	"github.com/arcvpn/arcvpn/internal/devices"
	"strings"
	"testing"
)

func TestV7EncryptedDeviceCredentials(t *testing.T) {
	s := sample()
	u := s.Users[0]
	state := devices.State{{UserID: u.ID, Identity: u.UUID, Version: 1, Limit: 1, Devices: []devices.Device{{ID: "22222222-2222-4222-8222-222222222222", UUID: "33333333-3333-4333-8333-333333333333", Token: strings.Repeat("a", 43), Name: "Phone", CreatedAt: 100, RevokedAt: 200}}}}
	s = s.WithDevices(state)
	blob, err := Encrypt(s, "backup-password-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), state[0].Devices[0].Token) {
		t.Fatal("plaintext token")
	}
	got, err := Decrypt(blob, "backup-password-long-enough")
	if err != nil || got.Devices == nil || (*got.Devices)[0].Devices[0].RevokedAt != 200 {
		t.Fatal("device state lost", err)
	}
	s.Version = 6
	if s.Validate() == nil {
		t.Fatal("v6 accepted device payload")
	}
}
