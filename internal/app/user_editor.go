package app

import (
	"context"
	"fmt"
	"reflect"

	"github.com/arcvpn/arcvpn/internal/store"
)

// Policy changes commit together; the existing worker retries pending runtime application.
func (a *App) SaveUserEditor(ctx context.Context, id int64, in store.UserEdit) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	old, err := a.Store.ByID(ctx, id)
	if err != nil {
		return err
	}
	assignments := !reflect.DeepEqual(old.Preferences.AllowedKinds, in.Preferences.AllowedKinds)
	if assignments {
		profiles, e := a.Store.ListProtocols(ctx)
		if e != nil {
			return e
		}
		for _, p := range profiles {
			if p.Kind == "amneziawg" {
				return store.ErrUserAssignmentsUnavailable
			}
		}
	}
	changed := assignments || old.Enabled != in.Enabled || in.Package != nil
	if changed && a.RuntimeCheck != nil {
		if err = a.RuntimeCheck(); err != nil {
			return err
		}
	}
	if p := in.Package; p != nil {
		if err = a.Store.CheckPackageCoverage(ctx, p.QuotaBytes, p.ExpiresAt); err != nil {
			return err
		}
		if p.QuotaBytes > 0 || p.ResetUsage {
			if err = a.collectUsageLocked(ctx); err != nil {
				return fmt.Errorf("fresh traffic counters required: %w", err)
			}
		}
	}
	if err = a.Store.SaveUserEditor(ctx, id, in, changed); err != nil {
		return err
	}
	if changed {
		_ = a.reconcilePackagesLocked(ctx, true)
	}
	a.Store.Event(ctx, "user.editor", fmt.Sprintf("ID %d", id))
	return nil
}
func (a *App) BulkUserAccess(ctx context.Context, items []store.UserSelection, enabled bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.RuntimeCheck != nil {
		if err := a.RuntimeCheck(); err != nil {
			return err
		}
	}
	if err := a.Store.BulkUserAccess(ctx, items, enabled); err != nil {
		return err
	}
	_ = a.reconcilePackagesLocked(ctx, true)
	a.Store.Event(ctx, "user.bulk.access", fmt.Sprintf("count=%d enabled=%t", len(items), enabled))
	return nil
}
