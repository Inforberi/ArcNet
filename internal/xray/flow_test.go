package xray

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestVLESSFlowServerAndExport(t *testing.T) {
	r := store.Reality{Address: "198.51.100.15", Port: 443, Target: "www.example.com", SNI: "www.example.com", Private: testPrivate, Public: testPublic, ShortID: "0123456789abcdef", Enabled: true}
	u := store.User{ID: 1, UUID: "11111111-1111-4111-8111-111111111111", Enabled: true}
	for _, flow := range []string{"", "none"} {
		r.ServerOptions.Flow = flow
		for _, managed := range []bool{false, true} {
			user := u
			if managed {
				user.DeviceAccess = &devices.Record{Devices: []devices.Device{{ID: "fixture", UUID: u.UUID}}}
			}
			blob, err := Generate(r, []store.User{user})
			if err != nil {
				t.Fatal(err)
			}
			var cfg map[string]any
			_ = json.Unmarshal(blob, &cfg)
			clients := cfg["inbounds"].([]any)[0].(map[string]any)["settings"].(map[string]any)["clients"].([]any)
			_, present := clients[0].(map[string]any)["flow"]
			if present != (flow != "none") {
				t.Fatal("server flow mismatch")
			}
			link, err := url.Parse(Link(r, u))
			if err != nil || link.Query().Get("flow") != r.ServerOptions.VLESSFlow() || link.Query().Get("security") != "reality" {
				t.Fatal("client flow or security mismatch", err)
			}
		}
	}
}
