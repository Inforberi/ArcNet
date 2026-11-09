package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
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
	r, err := a.Store.OptionalReality(ctx)
	if err != nil {
		return err
	}
	users, err := a.Store.ListUsers(ctx)
	if err != nil {
		return err
	}
	users, err = a.xrayUsers(ctx, users)
	if err != nil {
		return err
	}
	var sampled []usage.Counter
	var xrayErr error
	profiles, err := a.Store.ListProtocols(ctx)
	if err != nil {
		return errors.Join(xrayErr, err)
	}
	xrayEnabled := r.Enabled
	for _, p := range profiles {
		if (p.Kind == protocols.XHTTP || p.Kind == "vless-reality") && p.Enabled {
			xrayEnabled = true
		}
	}
	if xrayEnabled {
		counters, observed, err := a.Core.Statistics(ctx, users, true)
		if err != nil {
			xrayErr = err
		} else {
			xrayErr = a.Store.RecordUsage(ctx, counters, observed)
			if xrayErr == nil {
				sampled = append(sampled, counters...)
			}
		}
	}
	var hysteriaErr error
	var wireGuardErr error
	for _, p := range profiles {
		if p.Kind == protocols.Hysteria2 && p.Enabled {
			var counters []usage.Counter
			_, counters, hysteriaErr = a.collectHysteriaUsage(ctx, users, "")
			if hysteriaErr == nil {
				sampled = append(sampled, counters...)
			}
		}
		if p.Kind == protocols.WireGuard && p.Enabled {
			var counters []usage.Counter
			_, counters, wireGuardErr = a.collectWireGuardUsage(ctx, users, "")
			if wireGuardErr == nil {
				sampled = append(sampled, counters...)
			}
		}
	}
	a.sampleUserTraffic(ctx, users, r, profiles, sampled)
	return errors.Join(xrayErr, hysteriaErr, wireGuardErr)
}

func (a *App) collectWireGuardUsage(ctx context.Context, users []store.User, previousEpoch string) (string, []usage.Counter, error) {
	if a.Engines == nil {
		return "", nil, errors.New("WireGuard engine unavailable")
	}
	if _, err := os.Stat(filepath.Join(a.Engines.Dir, "arcwg.conf")); err != nil {
		return "", nil, errors.New("WireGuard configuration unavailable")
	}
	creds, err := a.Store.ListProtocolCredentials(ctx)
	if err != nil {
		return "", nil, err
	}
	counters, at, err := a.Engines.WireGuardStatistics(ctx, users, creds)
	if err != nil {
		return "", nil, err
	}
	a.cacheWireGuardPresence(counters)
	if err := a.Store.RecordEngineUsage(ctx, "wireguard", counters, at, previousEpoch); err != nil {
		return "", nil, err
	}
	if len(counters) > 0 {
		return counters[0].Epoch, counters, nil
	}
	return "", counters, nil
}

func (a *App) collectHysteriaUsage(ctx context.Context, users []store.User, previousEpoch string) (string, []usage.Counter, error) {
	if a.Engines == nil {
		return "", nil, errors.New("Hysteria engine is not configured")
	}
	if _, err := os.Stat(filepath.Join(a.Engines.Dir, "hysteria.yaml")); err != nil {
		return "", nil, errors.New("Hysteria configuration unavailable")
	}
	credentials, err := a.Store.ListProtocolCredentials(ctx)
	if err != nil {
		return "", nil, err
	}
	counters, at, err := a.Engines.HysteriaStatistics(ctx, users, credentials)
	if err != nil {
		return "", nil, err
	}
	if err := a.Store.RecordEngineUsage(ctx, "hysteria", counters, at, previousEpoch); err != nil {
		return "", nil, err
	}
	if len(counters) > 0 {
		return counters[0].Epoch, counters, nil
	}
	return "", counters, nil
}

func (a *App) reconcilePackagesLocked(ctx context.Context, force bool) error {
	r, err := a.Store.OptionalReality(ctx)
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
	policies := make(map[int64][]string, len(users))
	for _, u := range users {
		policies[u.ID] = u.Preferences.AllowedKinds
	}
	blob, _ := json.Marshal(struct {
		Allowed  []string
		Devices  any
		Policies any
	}{allowed, deviceState, policies})
	digest := sha256.Sum256(blob)
	key := hex.EncodeToString(digest[:])
	stateBefore, err := a.Store.AccountingState(ctx)
	if err != nil {
		return err
	}
	if !force && key == a.usageApplied && (stateBefore == "applied" || stateBefore == "unverified") {
		return nil
	}
	if err := a.applyAll(ctx, r, users); err != nil {
		_ = a.Store.SetAccountingState(context.Background(), "apply_failed")
		return errors.New("package access update could not be applied")
	}
	state := "applied"
	if a.Core.SkipRestart || (a.Engines != nil && a.Engines.SkipRestart) {
		state = "unverified"
	}
	if err := a.Store.SetAccountingState(ctx, state); err != nil {
		return err
	}
	a.runtimeUncertain = false
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
		a.wireGuardPresence = nil
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
			} else if _, err := a.Store.ResetDuePackages(call, time.Now()); err != nil && ctx.Err() == nil {
				report(errors.New("scheduled package reset unavailable"))
			}
			if err := a.reconcilePackagesLocked(call, false); err != nil && ctx.Err() == nil {
				report(err)
			}
		}
		// Presence is independent of accounting readiness and adds no per-user poll.
		a.collectPresenceLocked(call)
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
