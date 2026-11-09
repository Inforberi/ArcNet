package engines

import (
	"github.com/arcvpn/arcvpn/internal/store"
	"strings"
	"testing"
)

func TestHysteriaOnlineMapsCredentialsWithoutInventingDevices(t *testing.T) {
	users := []store.User{{ID: 1, UUID: "11111111-1111-4111-8111-111111111111", Enabled: true}, {ID: 2, UUID: "22222222-2222-4222-8222-222222222222", Enabled: true}}
	creds := []store.ProtocolCredential{{UserID: 1, Kind: "hysteria2", Secret: strings.Repeat("a", 43)}, {UserID: 2, Kind: "hysteria2", Secret: strings.Repeat("b", 43)}}
	rows, err := ParseHysteriaOnline([]byte(`{"u1":2}`), users, creds, "epoch", 100)
	if err != nil || len(rows) != 2 || rows[0].Online == nil || !*rows[0].Online || *rows[1].Online {
		t.Fatal("online credential mapping", rows, err)
	}
	for _, raw := range []string{`null`, `{"u1":-1}`, `{"foreign":1}`, `{} {}`} {
		if _, err = ParseHysteriaOnline([]byte(raw), users, creds, "epoch", 100); err == nil {
			t.Fatal("invalid online accepted")
		}
	}
}
func TestWireGuardHandshakeNeverRequiresPrivateKeyOutput(t *testing.T) {
	key := strings.Repeat("A", 43) + "="
	stamps, err := ParseWireGuardHandshakes([]byte(key+"\t90\n"), 100)
	if err != nil || stamps[key] != 90 {
		t.Fatal(stamps, err)
	}
	for _, raw := range []string{key + " -1", key + " 106", key + " 1\n" + key + " 2", "private 10"} {
		if _, err = ParseWireGuardHandshakes([]byte(raw), 100); err == nil {
			t.Fatal("invalid handshake accepted")
		}
	}
}
