package app

import (
	"context"

	"github.com/arcvpn/arcvpn/internal/usage"
)

func (a *App) RecordNodeUsage(ctx context.Context, node, secret string, report usage.NodeReport) error {
	// Share the existing owner of restore/reset/delete/apply. A remote report
	// cannot enter between user deletion and its compensating rollback.
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Store.RecordNodeUsage(ctx, node, secret, report)
}
