package app

import (
	"context"
	"errors"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

// DeployNode generates a fixed VLESS Reality configuration from the centrally
// managed users. Private keys are different on every node. No arbitrary JSON
// or remote shell command can be requested through the admin HTTP endpoint.
func (a *App) DeployNode(ctx context.Context, nodeID string) (store.NodeOperation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if restricted, err := a.Store.HasRestrictedPackages(ctx, false); err != nil {
		return store.NodeOperation{}, err
	} else if restricted {
		return store.NodeOperation{}, store.ErrUsageUnsupported
	}
	state, err := a.Store.NodeControl(ctx, nodeID)
	if err != nil {
		return store.NodeOperation{}, err
	}
	if state.Address == "" {
		return store.NodeOperation{}, errors.New("node needs a verified public endpoint")
	}
	r, err := a.Store.NodeReality(ctx, nodeID)
	if errors.Is(err, store.ErrNotFound) {
		local, e := a.Store.ReadReality(ctx)
		if e != nil {
			return store.NodeOperation{}, errors.New("configure local VPN first")
		}
		priv, pub, sid, e := xray.GenerateKeys(ctx, a.Core.Binary)
		if e != nil {
			return store.NodeOperation{}, e
		}
		r = store.Reality{Address: state.Address, Port: state.Port, Target: local.Target, SNI: local.SNI, Private: priv, Public: pub, ShortID: sid, Enabled: true}
		if e = xray.Validate(r); e != nil {
			return store.NodeOperation{}, e
		}
		if e = a.Store.PutNodeRealityIfAbsent(ctx, nodeID, r); e != nil {
			return store.NodeOperation{}, e
		}
		r, err = a.Store.NodeReality(ctx, nodeID)
	}
	if err != nil {
		return store.NodeOperation{}, err
	}
	if r.Address != state.Address || r.Port != state.Port {
		return store.NodeOperation{}, errors.New("node endpoint changed; identity migration required")
	}
	users, err := a.Store.ListUsers(ctx)
	if err != nil {
		return store.NodeOperation{}, err
	}
	payload, err := xray.Generate(r, users)
	if err != nil {
		return store.NodeOperation{}, err
	}
	return a.Store.QueueNodeDeployment(ctx, nodeID, payload)
}
