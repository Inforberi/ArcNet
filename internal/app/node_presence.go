package app

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/usage"
	"time"
)

func (a *App) RecordNodePresence(ctx context.Context, node, secret string, report usage.NodePresenceReport) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	rows, err := a.Store.ValidateNodePresence(ctx, node, secret, report)
	if err != nil {
		return err
	}
	a.UserPresence.Observe(rows, time.Now().Unix())
	return nil
}
