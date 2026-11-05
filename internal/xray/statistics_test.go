package xray

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/store"
)

func TestStatisticsParserSeparatesTransportsAndRejectsInvalidCounters(t *testing.T) {
	users := []store.User{{ID: 1, UUID: "11111111-1111-4111-8111-111111111111"}}
	data := []byte(`{"stat":[{"name":"user>>>user-1@local>>>traffic>>>uplink","value":"100"},{"name":"user>>>user-1@local>>>traffic>>>downlink","value":200},{"name":"user>>>xhttp-user-1@local>>>traffic>>>uplink","value":"300"}]}`)
	items, err := ParseStatistics(data, users, true, "epoch", 1, 100)
	if err != nil || len(items) != 2 || items[0].Upload != 100 || items[0].Download != 200 || items[1].Upload != 300 || items[1].Download != 0 {
		t.Fatal(items, err)
	}
	for _, blob := range []string{`garbage`, `{"stat":[{"name":"user>>>user-1@local>>>traffic>>>uplink","value":"-1"}]}`, `{"stat":[{"name":"user>>>user-1@local>>>traffic>>>uplink","value":1.5}]}`, string(data) + `{}`, `{"stat":[{"name":"user>>>user-1@local>>>traffic>>>uplink","value":"1"},{"name":"user>>>user-1@local>>>traffic>>>uplink","value":"2"}]}`} {
		if _, err := ParseStatistics([]byte(blob), users, false, "epoch", 1, 100); err == nil {
			t.Fatal("invalid stats accepted", blob)
		}
	}
	items, err = ParseStatistics([]byte(`{}`), users, false, "epoch", 1, 100)
	if err != nil || len(items) != 1 || items[0].Upload+items[0].Download != 0 {
		t.Fatal("confirmed empty response", items, err)
	}
}
func TestStatisticsConfigOnlyExposesPrivateStatsService(t *testing.T) {
	c := Controller{Path: filepath.Join(t.TempDir(), "xray.json")}
	blob, err := c.withStatistics([]byte(`{"inbounds":[],"outbounds":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if json.Unmarshal(blob, &root) != nil {
		t.Fatal("bad JSON")
	}
	api := root["api"].(map[string]any)
	if api["listen"] != c.StatisticsSocket() || strings.Contains(string(blob), "HandlerService") || strings.Contains(string(blob), "127.0.0.1:") {
		t.Fatal("unsafe stats API")
	}
	if _, _, err := c.Statistics(context.Background(), nil, false); err == nil {
		t.Fatal("absent API became zero traffic")
	}
	if err := os.WriteFile(c.StatisticsSocket(), []byte("not a socket"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := socketEpoch(c.StatisticsSocket()); err == nil {
		t.Fatal("regular file accepted as stats socket")
	}
}
