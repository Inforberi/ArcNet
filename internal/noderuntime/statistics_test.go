package noderuntime_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/usage"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func TestNodeStatisticsBoundaryAndParser(t *testing.T) {
	// Reuse the established valid runtime payload instead of inventing another
	// trusted helper surface. The statistics operation alone permits StatsService.
	base := bytes.ReplaceAll(validRequest(t, 1).Config, []byte("user-0@local"), []byte("user-1@local"))
	config, e := xray.WithNodeStatistics(base, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = noderuntime.ValidateStatisticsConfig(config); e != nil {
		t.Fatal(e)
	}
	if noderuntime.ValidateConfig(config) == nil {
		t.Fatal("legacy operation gained API access")
	}
	var root map[string]any
	if json.Unmarshal(config, &root) != nil {
		t.Fatal("config")
	}
	api := root["api"].(map[string]any)
	for _, bad := range []any{"127.0.0.1:10085", "/tmp/attacker.sock"} {
		api["listen"] = bad
		b, _ := json.Marshal(root)
		if noderuntime.ValidateStatisticsConfig(b) == nil {
			t.Fatal("arbitrary API endpoint accepted")
		}
	}
	api["listen"] = xray.NodeStatisticsSocket
	api["services"] = []string{"StatsService", "HandlerService"}
	b, _ := json.Marshal(root)
	if noderuntime.ValidateStatisticsConfig(b) == nil {
		t.Fatal("mutation API accepted")
	}
	clients := map[string]usage.NodeClient{"user-1@local": {UserID: 1}}
	good := []byte(`{"stat":[{"name":"user>>>user-1@local>>>traffic>>>uplink","value":"50"},{"name":"user>>>user-1@local>>>traffic>>>downlink","value":25}]}`)
	rows, e := noderuntime.ParseNodeStatistics(good, clients)
	if e != nil || len(rows) != 1 || rows[0].Upload != 50 || rows[0].Download != 25 {
		t.Fatal(rows, e)
	}
	for _, bad := range []string{
		`{"stat":[{"name":"user>>>foreign@local>>>traffic>>>uplink","value":10}]}`,
		`{"stat":[{"name":"user>>>user-1@local>>>traffic>>>uplink","value":-1}]}`,
		`{"stat":[{"name":"user>>>user-1@local>>>traffic>>>uplink","value":1.5}]}`,
		`{"stat":[{"name":"user>>>user-1@local>>>traffic>>>uplink","value":1},{"name":"user>>>user-1@local>>>traffic>>>uplink","value":2}]}`,
		`{"stat":[]} {}`,
	} {
		if _, e := noderuntime.ParseNodeStatistics([]byte(bad), clients); e == nil {
			t.Fatal("bad counters accepted")
		}
	}
}
