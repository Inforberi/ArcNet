package xray

import (
	"encoding/json"
	"testing"

	"github.com/arcvpn/arcvpn/internal/store"
)

func TestServerOptionsSniffingRender(t *testing.T) {
	off, on := false, true
	r := store.Reality{Address: "198.51.100.15", Port: 443, Target: "www.example.com", SNI: "www.example.com", Private: testPrivate, Public: testPublic, ShortID: "0123456789abcdef", Enabled: true, ServerOptions: store.ServerOptions{Sniffing: &off}}
	p := store.Protocol{Kind: "vless-xhttp", Address: r.Address, Port: 2087, Target: r.Target, SNI: r.SNI, PrivateKey: r.Private, PublicKey: r.Public, ShortID: r.ShortID, Path: "/fixture", Enabled: true, ServerOptions: store.ServerOptions{Sniffing: &on}}
	blob, err := GenerateWithProfiles(r, nil, []store.Protocol{p})
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err = json.Unmarshal(blob, &root); err != nil {
		t.Fatal(err)
	}
	rows := root["inbounds"].([]any)
	for i, want := range []bool{false, true} {
		sniff := rows[i].(map[string]any)["sniffing"].(map[string]any)
		if sniff["enabled"] != want || sniff["routeOnly"] != true {
			t.Fatal("sniffing semantics changed")
		}
	}
}
