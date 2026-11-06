package app

import (
	"context"
	"errors"

	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

type CascadeProbeBundle struct {
	store.CascadeProbeGrant
	Protocol   int    `json:"protocol"`
	Controller string `json:"controller"`
	Link       string `json:"link"`
}

func (a *App) CreateCascadeProbe(ctx context.Context, id string, version int64) (CascadeProbeBundle, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out CascadeProbeBundle
	deployment, e := a.Store.CascadeDeployment(ctx, id)
	if e != nil {
		return out, e
	}
	for _, node := range []string{deployment.EntryNode, deployment.ExitNode} {
		snapshot, e := a.Store.AcknowledgedNodeConfig(ctx, node)
		if e != nil {
			return out, e
		}
		if e := noderuntime.ValidateCascadeConfig(snapshot.Config); e != nil {
			return out, e
		}
	}
	profile, e := a.Store.NodeReality(ctx, deployment.EntryNode)
	if e != nil {
		return out, e
	}
	users, e := a.Store.ListUsers(ctx)
	if e != nil {
		return out, e
	}
	var user *store.User
	for i := range users {
		if users[i].Enabled && a.UserAllowed(ctx, users[i].ID) == nil {
			user = &users[i]
			break
		}
	}
	if user == nil {
		return out, errors.New("no enabled credential available for independent cascade probe")
	}
	grant, e := a.Store.CreateCascadeProbeGrant(ctx, id, version)
	if e != nil {
		return out, e
	}
	return CascadeProbeBundle{CascadeProbeGrant: grant, Protocol: 1, Link: xray.Link(profile, *user)}, nil
}
func (a *App) PublishCascade(ctx context.Context, id string, version int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	deployment, e := a.Store.CascadeDeployment(ctx, id)
	if e != nil {
		return e
	}
	for _, node := range []string{deployment.EntryNode, deployment.ExitNode} {
		snapshot, e := a.Store.AcknowledgedNodeConfig(ctx, node)
		if e != nil {
			return e
		}
		if e := noderuntime.ValidateCascadeConfig(snapshot.Config); e != nil {
			return e
		}
	}
	return a.Store.PublishCascade(ctx, id, version)
}
