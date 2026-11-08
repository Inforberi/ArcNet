package protocols

import (
	"github.com/arcvpn/arcvpn/internal/store"
	"strings"
	"testing"
)

func TestUserAssignmentsFilterAuxiliaryIdentities(t *testing.T) {
	u := store.User{ID: 1, Enabled: true, Preferences: store.UserPreferences{AllowedKinds: []string{}}}
	creds := []store.ProtocolCredential{{UserID: 1, Kind: Hysteria2, Secret: strings.Repeat("a", 43)}, {UserID: 1, Kind: WireGuard, PublicKey: strings.Repeat("A", 43) + "=", Address: "10.76.0.2"}}
	for _, v := range HysteriaIdentities([]store.User{u}, creds) {
		if v.Enabled {
			t.Fatal("revoked Hysteria identity enabled")
		}
	}
	for _, v := range WireGuardPeers([]store.User{u}, creds) {
		if v.Enabled {
			t.Fatal("revoked WG peer enabled")
		}
	}
}
