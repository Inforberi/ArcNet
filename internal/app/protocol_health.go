package app

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
)

// ProtocolHealth reports LOCAL daemon state, not reachability from a client.
// A running service can still be blocked by a firewall, routing or provider.
type ProtocolHealth struct {
	Kind     string `json:"kind"`
	Enabled  bool   `json:"enabled"`
	State    string `json:"state"`
	Evidence string `json:"evidence"`
}

func systemdState(ctx context.Context, unit string) string {
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	b, err := exec.CommandContext(c, "/usr/bin/systemctl", "is-active", unit).Output()
	raw := strings.TrimSpace(string(b))
	switch raw {
	case "active", "inactive", "failed", "activating", "deactivating":
		return raw
	}
	if err != nil {
		return "unknown"
	}
	return "unknown"
}
func (a *App) LocalProtocolHealth(ctx context.Context) ([]ProtocolHealth, error) {
	profiles, err := a.Store.ListProtocols(ctx)
	if err != nil {
		return nil, err
	}
	vpn, err := a.Store.ReadReality(ctx)
	if err != nil && err != store.ErrNotFound {
		return nil, err
	}
	items := make([]ProtocolHealth, 0, len(profiles)+1)
	if vpn.Address != "" {
		items = append(items, ProtocolHealth{Kind: "vless-reality", Enabled: vpn.Enabled, State: "unknown", Evidence: "service-only"})
	}
	for _, p := range profiles {
		items = append(items, ProtocolHealth{Kind: p.Kind, Enabled: p.Enabled, State: "unknown", Evidence: "service-only"})
	}
	services := map[string]string{"vless-reality": "arcvpn-xray.service", "vless-xhttp": "arcvpn-xray.service", "hysteria2": "arcvpn-hysteria.service", "wireguard": "arcvpn-wireguard.service", "amneziawg": "arcvpn-amneziawg.service"}
	for i := range items {
		item := &items[i]
		if !item.Enabled {
			item.State = "disabled"
			item.Evidence = "configuration"
			continue
		}
		if a.Core.SkipRestart || (a.Engines != nil && a.Engines.SkipRestart) {
			item.State = "unverified"
			item.Evidence = "test-mode"
			continue
		}
		item.State = systemdState(ctx, services[item.Kind])
	}
	return items, nil
}
