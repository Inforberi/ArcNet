package app

import (
	"context"
	"errors"

	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

// Called only after bearer authentication by the existing node poll. No extra
// timer: unchanged generations take only bounded indexed metadata reads.
func (a *App) ReconcileNodeCredentials(ctx context.Context, node string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	managed, e := a.Store.HasManagedDevices(ctx)
	if e != nil {
		return e
	}
	accounting, configured, e := a.Store.NodeUsageStatus(ctx, node)
	if e != nil {
		return e
	}
	if !managed && !accounting {
		return nil
	}
	state, e := a.Store.NodeControl(ctx, node)
	if e != nil {
		return e
	}
	if state.DeploymentStatus == "queued" || state.DeploymentStatus == "running" {
		return nil
	}
	generation, e := a.Store.CredentialGeneration(ctx)
	if e != nil {
		return e
	}
	queued, applied, operation, e := a.Store.NodeCredentialGeneration(ctx, node)
	if e != nil {
		return e
	}
	if applied == generation && operation == state.DeploymentID && state.DeploymentStatus == "applied" && (!accounting || configured) {
		return nil
	}
	if queued == generation && state.DeploymentStatus == "failed" {
		return nil
	} // explicit same-ID retry, never a new replay identity
	if finite, e := a.Store.HasFinitePackages(ctx); e != nil {
		return e
	} else if finite {
		return store.ErrUsageUnsupported
	}
	profile, e := a.Store.NodeReality(ctx, node)
	if errors.Is(e, store.ErrNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	if profile.Address != state.Address || profile.Port != state.Port {
		return store.ErrConflict
	}
	users, e := a.Store.ListUsers(ctx)
	if e != nil {
		return e
	}
	users, e = a.xrayUsers(ctx, users)
	if e != nil {
		return e
	}
	payload, e := xray.Generate(profile, users)
	if e != nil {
		return e
	}
	if accounting {
		presence, capabilityErr := a.Store.NodePresenceCapable(ctx, node)
		if capabilityErr != nil {
			return capabilityErr
		}
		payload, e = xray.WithNodeStatistics(payload, users, presence)
		if e != nil {
			return e
		}
		_, e = a.Store.QueueNodeUsageCredentials(ctx, node, payload, generation, profile.Address, profile.Port)
		return e
	}
	_, e = a.Store.QueueNodeCredentials(ctx, node, payload, generation, profile.Address, profile.Port)
	return e
}
