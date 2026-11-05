package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/arcvpn/arcvpn/internal/backup"
	"github.com/arcvpn/arcvpn/internal/routes"
	"github.com/arcvpn/arcvpn/internal/routingprofile"
	"github.com/arcvpn/arcvpn/internal/store"
)

// ExportState takes a point-in-time snapshot within the application mutation lock.
func (a *App) ExportState(ctx context.Context) (backup.Snapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.exportLocked(ctx)
}
func (a *App) exportLocked(ctx context.Context) (backup.Snapshot, error) {
	users, err := a.Store.ListUsers(ctx)
	if err != nil {
		return backup.Snapshot{}, err
	}
	v, err := a.Store.ReadReality(ctx)
	if errors.Is(err, store.ErrNotFound) {
		v = store.Reality{}
	} else if err != nil {
		return backup.Snapshot{}, err
	}
	profiles, err := a.Store.ListProtocols(ctx)
	if err != nil {
		return backup.Snapshot{}, err
	}
	creds, err := a.Store.ListProtocolCredentials(ctx)
	if err != nil {
		return backup.Snapshot{}, err
	}
	nodes, policies, err := a.Store.RouteBackupState(ctx)
	if err != nil {
		return backup.Snapshot{}, err
	}
	routingProfiles, err := a.Store.RoutingProfiles(ctx)
	if err != nil {
		return backup.Snapshot{}, err
	}
	budget, err := a.Store.LogBudgetMB(ctx)
	if err != nil {
		return backup.Snapshot{}, err
	}
	if v.Address == "" {
		return backup.FromStore(nil, users).WithProtocols(profiles, creds).WithManagement(nodes, budget).WithRoutes(policies).WithRoutingProfiles(routingProfiles), nil
	}
	return backup.FromStore(&v, users).WithProtocols(profiles, creds).WithManagement(nodes, budget).WithRoutes(policies).WithRoutingProfiles(routingProfiles), nil
}

// RestoreState replaces only VPN state and rolls it back if applying Xray fails.
// The administrator account, sessions and audit records are never imported.
func (a *App) RestoreState(ctx context.Context, candidate backup.Snapshot) error {
	if err := candidate.Validate(); err != nil {
		return err
	}
	if candidate.VPN == nil {
		return errors.New("restoring a backup without a configured VPN is not yet supported")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	old, err := a.exportLocked(ctx)
	if err != nil {
		return err
	}
	oldVPN, oldUsers := old.ToStore()
	newVPN, newUsers := candidate.ToStore()
	oldProfiles, oldCreds := old.ToProtocols()
	newProfiles, newCreds := candidate.ToProtocols()
	var importedNodes *[]store.NodeIdentity
	var importedBudget *int
	var importedRoutes *[]routes.Policy
	var importedRoutingProfiles *[]routingprofile.Profile
	if candidate.Version >= 3 {
		importedNodes = &candidate.Nodes
		importedBudget = candidate.LogBudgetMB
	}
	if candidate.Version >= 4 {
		importedRoutes = &candidate.Routes
	}
	if candidate.Version >= 5 {
		importedRoutingProfiles = &candidate.RoutingProfiles
	}
	if err = a.Store.ReplaceFullStateWithProfiles(ctx, newVPN, newUsers, newProfiles, newCreds, importedNodes, importedBudget, importedRoutes, importedRoutingProfiles); err != nil {
		return err
	}
	if err = a.applyAll(ctx, *newVPN, newUsers); err != nil {
		undoCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		dbErr := a.Store.ReplaceFullStateWithProfiles(undoCtx, oldVPN, oldUsers, oldProfiles, oldCreds, &old.Nodes, old.LogBudgetMB, &old.Routes, &old.RoutingProfiles)
		if dbErr != nil {
			return fmt.Errorf("Xray update failed (%v), database rollback also failed: %w", err, dbErr)
		}
		if oldVPN != nil {
			if recovery := a.applyAll(undoCtx, *oldVPN, oldUsers); recovery != nil {
				return fmt.Errorf("Xray update failed (%v), recovery of previous Xray configuration failed: %w", err, recovery)
			}
		}
		return fmt.Errorf("backup not restored; previous database state restored: %w", err)
	}
	a.Store.Event(ctx, "backup.restored", fmt.Sprintf("users=%d protocols=%d", len(newUsers), len(newProfiles)))
	return nil
}
