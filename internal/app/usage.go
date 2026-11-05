package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
)

func (a *App) effectiveUsers(ctx context.Context, users []store.User) ([]store.User, error) {
	packages, err := a.Store.Packages(ctx)
	if err != nil {
		return nil, err
	}
	fresh, _, err := a.Store.AccountingFresh(ctx)
	if err != nil {
		return nil, err
	}
	result := append([]store.User{}, users...)
	for i, u := range result {
		if p, ok := packages[u.ID]; ok && p.Reason(time.Now().Unix(), fresh) != "" {
			result[i].Enabled = false
		}
	}
	return a.xrayUsers(ctx, result)
}
func (a *App) collectUsageLocked(ctx context.Context) error {
	r, err := a.Store.ReadReality(ctx)
	if err != nil {
		return err
	}
	if !r.Enabled {
		return errors.New("local Xray is disabled")
	}
	users, err := a.Store.ListUsers(ctx)
	if err != nil {
		return err
	}
	users, err = a.xrayUsers(ctx, users)
	if err != nil {
		return err
	}
	counters, observed, err := a.Core.Statistics(ctx, users, true)
	if err != nil {
		return err
	}
	return a.Store.RecordUsage(ctx, counters, observed)
}

func (a *App) reconcilePackagesLocked(ctx context.Context, force bool) error {
	r, err := a.Store.ReadReality(ctx)
	if err != nil {
		return err
	}
	users, err := a.Store.ListUsers(ctx)
	if err != nil {
		return err
	}
	effective, err := a.effectiveUsers(ctx, users)
	if err != nil {
		return err
	}
	allowed := make([]string, 0, len(effective))
	for _, u := range effective {
		if u.Enabled {
			allowed = append(allowed, u.UUID)
		}
	}
	deviceState, err := a.Store.DeviceState(ctx)
	if err != nil {
		return err
	}
	blob, _ := json.Marshal(struct {
		Allowed []string
		Devices any
	}{allowed, deviceState})
	digest := sha256.Sum256(blob)
	key := hex.EncodeToString(digest[:])
	if !force && key == a.usageApplied {
		return nil
	}
	if err := a.applyAll(ctx, r, users); err != nil {
		_ = a.Store.SetAccountingState(context.Background(), "apply_failed")
		return errors.New("package access update could not be applied")
	}
	state := "applied"
	if a.Core.SkipRestart {
		state = "unverified"
	}
	if err := a.Store.SetAccountingState(ctx, state); err != nil {
		return err
	}
	a.usageApplied = key
	return nil
}

// RunUsage owns the only local accounting loop. No collector is created by a
// browser request. Failed apply is retried without reverting consumed bytes.
func (a *App) RunUsage(ctx context.Context, report func(error)) {
	prepared := false
	for ctx.Err() == nil {
		call, cancel := context.WithTimeout(ctx, 40*time.Second)
		a.mu.Lock()
		if !prepared {
			err := a.reconcilePackagesLocked(call, true)
			if err == nil {
				prepared = true
			} else if !errors.Is(err, store.ErrNotFound) && ctx.Err() == nil {
				report(errors.New("user accounting preparation unavailable"))
			}
		}
		if prepared {
			if err := a.collectUsageLocked(call); err != nil && ctx.Err() == nil {
				_ = a.Store.SetAccountingState(call, "source_unavailable")
			}
			if err := a.reconcilePackagesLocked(call, false); err != nil && ctx.Err() == nil {
				report(err)
			}
		}
		a.mu.Unlock()
		cancel()
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (a *App) SaveUserPackage(ctx context.Context, id, quota, expires, version int64, reset bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.Store.CheckPackageCoverage(ctx, quota, expires); err != nil {
		return err
	}
	if quota > 0 || reset {
		if err := a.collectUsageLocked(ctx); err != nil {
			return errors.New("fresh traffic counters required before changing a finite package or resetting usage")
		}
	}
	if err := a.Store.SavePackage(ctx, id, quota, expires, version, reset); err != nil {
		return err
	}
	// The persisted policy is authoritative even if runtime is temporarily
	// unavailable. The API returns its pending state and the worker retries.
	_ = a.reconcilePackagesLocked(ctx, true)
	return nil
}

func (a *App) UserAllowed(ctx context.Context, id int64) error {
	devices, err := a.Store.UserDevices(ctx, id)
	if err != nil {
		return err
	}
	if devices.Version > 0 {
		return store.ErrNotFound
	}
	allowed, _, err := a.Store.UserAccess(ctx, id)
	if err != nil {
		return err
	}
	if !allowed {
		return store.ErrNotFound
	}
	return nil
}

// Failed or uncontrolled restarts retain the review gate. A successful apply
// records its observed new epoch immediately, without a false unplanned reset.
func (a *App) recordAppliedUsage(ctx context.Context, users []store.User, previousEpoch string) {
	if counters, at, err := a.Core.Statistics(ctx, users, true); err == nil {
		if err := a.Store.RecordUsageAfterApply(ctx, counters, at, previousEpoch); err != nil {
			_ = a.Store.SetAccountingState(ctx, "source_unavailable")
		}
	}
}
