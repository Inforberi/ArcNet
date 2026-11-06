package app

import (
	"context"
)

// ApplyLocalRuntime uses the same reconciliation as the panel worker, while
// the root coordinator keeps the panel stopped and serializes update/lifecycle.
func (a *App) ApplyLocalRuntime(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.reconcilePackagesLocked(ctx, true)
}

// FinalLocalUsage does not start or stop a service. Missing observations stay
// unknown, and existing review gates are never cleared by a CLI operation.
func (a *App) FinalLocalUsage(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	err := a.collectUsageLocked(ctx)
	if err != nil {
		_ = a.Store.SetAccountingState(ctx, "source_unavailable")
	}
	return err
}
