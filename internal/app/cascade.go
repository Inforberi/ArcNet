package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

// PrepareCascade is an internal diagnostic deployment boundary, deliberately
// not a public apply/publish endpoint until logical endpoint/probe gates exist.
func (a *App) PrepareCascade(ctx context.Context, routeID string, version int64, channelPort int) (store.CascadeDeployment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.prepareCascadeLocked(ctx, routeID, version, channelPort)
}

func (a *App) prepareCascadeLocked(ctx context.Context, routeID string, version int64, channelPort int) (store.CascadeDeployment, error) {
	policy, e := a.Store.RoutePolicy(ctx, routeID)
	if e != nil {
		return store.CascadeDeployment{}, e
	}
	if policy.Version != version {
		return store.CascadeDeployment{}, store.ErrConflict
	}
	if policy.Mode != "cascade" || policy.FailureMode != "closed" || len(policy.BackupExits) > 0 || policy.EntryNode == "local" || policy.PrimaryExit == "local" {
		return store.CascadeDeployment{}, store.ErrRouteInvalid
	}
	roster, e := a.Store.CascadeRoster(ctx)
	if e != nil {
		return store.CascadeDeployment{}, e
	}
	users, e := a.Store.ListUsers(ctx)
	if e != nil {
		return store.CascadeDeployment{}, e
	}
	entry, e := a.Store.NodeReality(ctx, policy.EntryNode)
	if e != nil {
		return store.CascadeDeployment{}, e
	}
	exit, e := a.Store.NodeReality(ctx, policy.PrimaryExit)
	if e != nil {
		return store.CascadeDeployment{}, e
	}
	for id, profile := range map[string]store.Reality{policy.EntryNode: entry, policy.PrimaryExit: exit} {
		state, e := a.Store.NodeControl(ctx, id)
		if e != nil {
			return store.CascadeDeployment{}, e
		}
		if state.Address != profile.Address || state.Port != profile.Port {
			return store.CascadeDeployment{}, errors.New("node endpoint changed; identity migration required")
		}
	}
	baseA, e := a.Store.AcknowledgedNodeConfig(ctx, policy.EntryNode)
	if e != nil {
		return store.CascadeDeployment{}, e
	}
	baseB, e := a.Store.AcknowledgedNodeConfig(ctx, policy.PrimaryExit)
	if e != nil {
		return store.CascadeDeployment{}, e
	}
	channel, e := xray.NewCascadeChannel(channelPort)
	if e != nil {
		return store.CascadeDeployment{}, e
	}
	left, right, e := xray.GenerateCascade(entry, exit, users, users, channel)
	if e != nil {
		return store.CascadeDeployment{}, e
	}
	left, right, e = preserveCascadeClients(left, right, baseA.Config, baseB.Config)
	if e != nil {
		return store.CascadeDeployment{}, e
	}
	for _, config := range [][]byte{left, right} {
		if e := noderuntime.ValidateCascadeConfig(config); e != nil {
			return store.CascadeDeployment{}, e
		}
	}
	// Explicit private serialization: public CascadeChannel marshals to {}.
	secret, e := json.Marshal(struct {
		ID      string `json:"id"`
		Private string `json:"private_key"`
		Public  string `json:"public_key"`
		Short   string `json:"short_id"`
		Port    int    `json:"port"`
	}{channel.ID, channel.Private, channel.Public, channel.ShortID, channel.Port})
	if e != nil {
		return store.CascadeDeployment{}, e
	}
	return a.Store.PrepareCascade(ctx, store.CascadePreparation{RouteID: routeID, ExpectedVersion: version, RosterDigest: roster, EntryBaselineDigest: baseA.Digest, ExitBaselineDigest: baseB.Digest, EntryConfig: left, ExitConfig: right, ChannelSecret: secret})
}
