package app

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
	"time"
)

func (a *App) sampleUserTraffic(ctx context.Context, users []store.User, r store.Reality, profiles []store.Protocol, counters []usage.Counter) {
	kinds := []string{}
	if r.Enabled {
		kinds = append(kinds, "vless-reality")
	}
	complete := true
	for _, p := range profiles {
		if !p.Enabled {
			continue
		}
		switch p.Kind {
		case protocols.XHTTP, protocols.Hysteria2, protocols.WireGuard:
			kinds = append(kinds, p.Kind)
		default:
			complete = false
		}
	}
	if a.Store.CheckPackageCoverage(ctx, 1, 0) != nil {
		complete = false
	}
	owners := make([]usage.TrafficUser, 0, len(users))
	for _, u := range users {
		required := make([]string, 0, len(kinds))
		for _, kind := range kinds {
			if u.Allows(kind) {
				required = append(required, kind)
			}
		}
		owners = append(owners, usage.TrafficUser{ID: u.ID, Identity: u.UUID, Kinds: required})
	}
	a.UserTraffic.Update(owners, counters, time.Now().Unix(), complete)
}
