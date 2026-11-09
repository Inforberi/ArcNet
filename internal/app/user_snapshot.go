package app

import (
	"context"

	"github.com/arcvpn/arcvpn/internal/store"
)

// UsersSnapshot binds the list and application acknowledgement under the same
// mutation lock. A database read alone cannot acknowledge runtime recovery.
func (a *App) UsersSnapshot(ctx context.Context) ([]store.User, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	users, err := a.Store.ListUsers(ctx)
	if err != nil {
		return nil, false, err
	}
	state, err := a.Store.AccountingState(ctx)
	confirmed := err == nil && state == "applied" && a.usageApplied != "" && !a.runtimeUncertain && !a.Core.SkipRestart && (a.Engines == nil || !a.Engines.SkipRestart)
	return users, confirmed, err
}
