package backup

import (
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestRemoteDeviceBackupV11PreservesCredentialsAndRejectsLegacyShape(t *testing.T) {
	user := store.User{ID: 1, Name: "TV", UUID: "11111111-1111-4111-8111-111111111111", Subscription: strings.Repeat("t", 43), Enabled: true, CreatedAt: time.Now().Unix()}
	state := devices.State{{UserID: 1, Identity: user.UUID, Version: 1, Limit: 2, Devices: []devices.Device{{ID: "22222222-2222-4222-8222-222222222222", Name: "TV", UUID: "33333333-3333-4333-8333-333333333333", Token: strings.Repeat("s", 43), CreatedAt: time.Now().Unix()}}}}
	snapshot := FromStore(nil, []store.User{user}).WithManagement([]store.NodeIdentity{{ID: "44444444-4444-4444-8444-444444444444", Name: "Node", TokenHash: strings.Repeat("a", 64), CreatedAt: time.Now().Unix()}}, 100).WithDevices(state)
	if snapshot.Version != Version {
		t.Fatal("version")
	}
	snapshot.Version = 11 // Keep this legacy compatibility fixture on its original format.
	if e := snapshot.Validate(); e != nil {
		t.Fatal("remote-only device snapshot invalid", e)
	}
	blob, e := Encrypt(snapshot, "private-v11-backup-fixture")
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := Decrypt(blob, "private-v11-backup-fixture")
	if e != nil || decoded.Devices == nil || (*decoded.Devices)[0].Devices[0].UUID != state[0].Devices[0].UUID {
		t.Fatal("device roundtrip", e)
	}
	snapshot.Version = 10
	if e := snapshot.Validate(); e == nil {
		t.Fatal("legacy accepted remote device state")
	}
}
