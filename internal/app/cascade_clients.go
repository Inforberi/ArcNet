package app

import (
	"encoding/json"
	"reflect"

	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/store"
)

// Preserve the acknowledged listeners, including every client UUID, flow,
// email and REALITY setting. Changing the exit never regenerates client access.
func preserveCascadeClients(left, right, baseA, baseB []byte) ([]byte, []byte, error) {
	configs := [][]byte{left, right}
	baselines := [][]byte{baseA, baseB}
	for i := range configs {
		if noderuntime.ValidateConfig(baselines[i]) != nil && noderuntime.ValidateStatisticsConfig(baselines[i]) != nil {
			return nil, nil, store.ErrCascadeNotReady
		}
		var next, previous map[string]any
		if json.Unmarshal(configs[i], &next) != nil || json.Unmarshal(baselines[i], &previous) != nil {
			return nil, nil, store.ErrCascadeNotReady
		}
		generated := next["inbounds"].([]any)[0].(map[string]any)
		existing := previous["inbounds"].([]any)[0].(map[string]any)
		// Everything except the list of clients must match the selected profile.
		for _, key := range []string{"listen", "port", "protocol", "tag", "streamSettings", "sniffing"} {
			if !reflect.DeepEqual(generated[key], existing[key]) {
				return nil, nil, store.ErrCascadeNotReady
			}
		}
		want := generated["settings"].(map[string]any)["clients"].([]any)
		have := existing["settings"].(map[string]any)["clients"].([]any)
		if len(want) != len(have) {
			return nil, nil, store.ErrCascadeNotReady
		}
		expected := map[string]any{}
		for _, value := range want {
			client := value.(map[string]any)
			expected[client["id"].(string)] = value
		}
		for _, value := range have {
			client := value.(map[string]any)
			if !reflect.DeepEqual(expected[client["id"].(string)], value) {
				return nil, nil, store.ErrCascadeNotReady
			}
		}
		next["inbounds"].([]any)[0] = existing
		var e error
		configs[i], e = json.Marshal(next)
		if e != nil {
			return nil, nil, e
		}
	}
	return configs[0], configs[1], nil
}
func cascadeClientCount(raw []byte) int {
	var value struct {
		Inbounds []struct {
			Settings struct {
				Clients []json.RawMessage `json:"clients"`
			} `json:"settings"`
		} `json:"inbounds"`
	}
	if json.Unmarshal(raw, &value) != nil || len(value.Inbounds) != 1 {
		return 0
	}
	return len(value.Inbounds[0].Settings.Clients)
}
