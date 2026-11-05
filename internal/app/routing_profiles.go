package app

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/routingprofile"
)

func (a *App) SaveRoutingProfile(ctx context.Context, p routingprofile.Profile, expected int64) (routingprofile.Profile, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Store.SaveRoutingProfile(ctx, p, expected)
}
func (a *App) DeleteRoutingProfile(ctx context.Context, id string, expected int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Store.DeleteRoutingProfile(ctx, id, expected)
}
