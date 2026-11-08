package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

func (a *App) CreateUsers(ctx context.Context, names []string, group string) ([]store.User, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(names) < 1 || len(names) > 100 {
		return nil, store.ErrInvalidUserPreferences
	}
	if _, err := a.Store.LocalServer(ctx); err != nil {
		return nil, err
	}
	if a.RuntimeCheck != nil {
		if err := a.RuntimeCheck(); err != nil {
			return nil, err
		}
	}
	profiles, err := a.Store.ListProtocols(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]store.User, 0, len(names))
	for _, name := range names {
		uuid, err := auth.UUID()
		if err != nil {
			return nil, err
		}
		token, err := auth.Secret(32)
		if err != nil {
			return nil, err
		}
		users = append(users, store.User{Name: strings.TrimSpace(name), UUID: uuid, Subscription: token, Enabled: true, CreatedAt: time.Now().Unix(), Preferences: store.UserPreferences{Group: group}})
	}
	created, err := a.Store.CreateUsers(ctx, users, func(u store.User) ([]store.ProtocolCredential, error) {
		items := make([]store.ProtocolCredential, 0, len(profiles))
		for _, p := range profiles {
			c, err := protocols.NewCredential(p.ConnectionID(), u)
			if err != nil {
				return nil, err
			}
			items = append(items, c)
		}
		return items, nil
	})
	if err != nil {
		return nil, err
	}
	_ = a.reconcilePackagesLocked(ctx, true)
	a.Store.Event(ctx, "user.bulk.created", fmt.Sprintf("count=%d", len(created)))
	return created, nil
}
func (a *App) BulkEditUsers(ctx context.Context, items []store.UserBatchSelection, in store.UserBatchChanges) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(items) < 1 || len(items) > 100 || !in.Valid() {
		return store.ErrInvalidUserPreferences
	}
	runtime := in.QuotaBytes != nil || in.ExpiresAt != nil || in.Connections != nil
	if runtime && a.RuntimeCheck != nil {
		if err := a.RuntimeCheck(); err != nil {
			return err
		}
	}
	if in.Connections != nil {
		profiles, err := a.Store.ListProtocols(ctx)
		if err != nil {
			return err
		}
		for _, p := range profiles {
			if p.Kind == protocols.AmneziaWG {
				return store.ErrUserAssignmentsUnavailable
			}
		}
	}
	if in.QuotaBytes != nil || in.ExpiresAt != nil {
		packages, err := a.Store.Packages(ctx)
		if err != nil {
			return err
		}
		var quota, expires int64
		for _, item := range items {
			p, ok := packages[item.ID]
			if !ok {
				return store.ErrNotFound
			}
			if in.QuotaBytes != nil {
				p.QuotaBytes = *in.QuotaBytes
			}
			if in.ExpiresAt != nil {
				p.ExpiresAt = *in.ExpiresAt
			}
			quota = max(quota, p.QuotaBytes)
			expires = max(expires, p.ExpiresAt)
		}
		if err = a.Store.CheckPackageCoverage(ctx, quota, expires); err != nil {
			return err
		}
		if quota > 0 {
			if err = a.collectUsageLocked(ctx); err != nil {
				return fmt.Errorf("fresh traffic counters required: %w", err)
			}
		}
	}
	if err := a.Store.BulkEditUsers(ctx, items, in); err != nil {
		return err
	}
	if runtime {
		_ = a.reconcilePackagesLocked(ctx, true)
	}
	a.Store.Event(ctx, "user.bulk.edited", fmt.Sprintf("count=%d", len(items)))
	return nil
}
