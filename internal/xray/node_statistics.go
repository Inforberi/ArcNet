package xray

import (
	"encoding/json"
	"fmt"

	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
)

const NodeStatisticsSocket = "/run/arcvpn-node-xray/xray-stats.sock"

// Device email labels carry their owner for accounting; access UUIDs and all
// exported client profiles stay unchanged. Only a read-only API is exposed.
func WithNodeStatistics(data []byte, users []store.User) ([]byte, error) {
	var root map[string]any
	if json.Unmarshal(data, &root) != nil {
		return nil, usage.ErrInvalid
	}
	labels := map[string]string{}
	for _, u := range users {
		if u.DeviceAccess != nil {
			for _, d := range u.DeviceAccess.Devices {
				labels["user-device-"+d.ID+"@local"] = fmt.Sprintf("user-%d-device-%s@local", u.ID, d.ID)
			}
		}
	}
	ins, ok := root["inbounds"].([]any)
	if !ok {
		return nil, usage.ErrInvalid
	}
	for _, raw := range ins {
		in, ok := raw.(map[string]any)
		if !ok {
			return nil, usage.ErrInvalid
		}
		settings, ok := in["settings"].(map[string]any)
		if !ok {
			return nil, usage.ErrInvalid
		}
		clients, ok := settings["clients"].([]any)
		if !ok {
			return nil, usage.ErrInvalid
		}
		for _, raw := range clients {
			c, ok := raw.(map[string]any)
			if !ok {
				return nil, usage.ErrInvalid
			}
			name, _ := c["email"].(string)
			if replacement := labels[name]; replacement != "" {
				c["email"] = replacement
			}
		}
	}
	root["stats"] = map[string]any{}
	root["api"] = map[string]any{"tag": "arcvpn-stats", "listen": NodeStatisticsSocket, "services": []string{"StatsService"}}
	root["policy"] = map[string]any{"levels": map[string]any{"0": map[string]bool{"statsUserUplink": true, "statsUserDownlink": true}}}
	out, e := json.Marshal(root)
	if e != nil {
		return nil, e
	}
	if _, e = usage.NodeClients(out); e != nil {
		return nil, e
	}
	return out, nil
}
