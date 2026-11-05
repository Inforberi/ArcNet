package app

import (
	"context"

	"github.com/arcvpn/arcvpn/internal/routes"
)

// Route mutations share the restore lock: a failed VPN restore must not roll
// back a draft saved by another browser while the engines were being applied.
func (a *App) SaveRoute(ctx context.Context, p routes.Policy, version int64) (routes.Policy, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Store.SaveRoute(ctx, p, version)
}

func (a *App) DeleteRoute(ctx context.Context, id string, version int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Store.DeleteRoute(ctx, id, version)
}
