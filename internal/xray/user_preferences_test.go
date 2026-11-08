package xray

import (
	"bytes"
	"encoding/json"
	"github.com/arcvpn/arcvpn/internal/store"
	"strings"
	"testing"
)

func TestUserAssignmentsSeparateTCPAndXHTTP(t *testing.T) {
	r := store.Reality{Address: "198.51.100.1", Port: 443, SNI: "www.example.com", Target: "www.example.com", Private: strings.Repeat("A", 43), Public: strings.Repeat("B", 43), ShortID: "0123456789abcdef", Enabled: true}
	p := store.Protocol{Kind: "vless-xhttp", Address: r.Address, Port: 2087, SNI: r.SNI, Target: r.Target, PrivateKey: r.Private, PublicKey: r.Public, ShortID: r.ShortID, Path: "/stream", Enabled: true}
	users := []store.User{{ID: 1, UUID: "11111111-1111-4111-8111-111111111111", Enabled: true, Preferences: store.UserPreferences{AllowedKinds: []string{"vless-reality"}}}, {ID: 2, UUID: "22222222-2222-4222-8222-222222222222", Enabled: true, Preferences: store.UserPreferences{AllowedKinds: []string{"vless-xhttp"}}}, {ID: 3, UUID: "33333333-3333-4333-8333-333333333333", Enabled: true, Preferences: store.UserPreferences{AllowedKinds: []string{}}}}
	raw, err := GenerateWithProfiles(r, users, []store.Protocol{p})
	if err != nil {
		t.Fatal(err)
	}
	var root struct {
		Inbounds []json.RawMessage `json:"inbounds"`
	}
	if err = json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	if len(root.Inbounds) != 2 {
		t.Fatal("expected two listeners")
	}
	for i, raw := range root.Inbounds {
		for j, u := range users {
			if bytes.Contains(raw, []byte(u.UUID)) != (i == j) {
				t.Fatal("credential crossed assignment boundary", i, j)
			}
		}
	}
}
