package xray

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestDeviceRenderingAndAggregatedCounters(t *testing.T) {
	d := devices.Device{ID: "22222222-2222-4222-8222-222222222222", UUID: "33333333-3333-4333-8333-333333333333"}
	retired := devices.Device{ID: "44444444-4444-4444-8444-444444444444", UUID: "55555555-5555-4555-8555-555555555555", RevokedAt: 200}
	u := store.User{ID: 1, UUID: "11111111-1111-4111-8111-111111111111", Enabled: true, DeviceAccess: &devices.Record{Devices: []devices.Device{d, retired}}}
	clients := userClients(u, false, "xtls-rprx-vision")
	blob, _ := json.Marshal(clients)
	if len(clients) != 1 || !strings.Contains(string(blob), d.UUID) || strings.Contains(string(blob), u.UUID) || strings.Contains(string(blob), retired.UUID) {
		t.Fatal("credential allowlist", string(blob))
	}
	stats := `{"stat":[{"name":"user>>>user-1@local>>>traffic>>>uplink","value":"10"},{"name":"user>>>user-device-` + d.ID + `@local>>>traffic>>>uplink","value":"20"},{"name":"user>>>user-device-` + retired.ID + `@local>>>traffic>>>uplink","value":"30"},{"name":"user>>>xhttp-user-device-` + d.ID + `@local>>>traffic>>>downlink","value":"40"}]}`
	counters, err := ParseStatistics([]byte(stats), []store.User{u}, true, "epoch", 1, 100)
	if err != nil || len(counters) != 2 || counters[0].Upload != 60 || counters[1].Download != 40 || counters[0].Identity != u.UUID {
		t.Fatal(counters, err)
	}
	// A revoked credential's old counters still belong to the user; retiring a
	// credential must not subtract usage or produce a regression in the epoch.
}
