package nodeagent

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/arcvpn/arcvpn/internal/noderuntime"
)

func reportNodeUsage(ctx context.Context, client *http.Client, c Configuration, socket string) error {
	call, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	caps, e := noderuntime.CallHelper(call, socket, noderuntime.ApplyRequest{ProtocolVersion: 1, Kind: "capabilities_v1"})
	if e != nil {
		return e
	}
	if caps.UsageProtocol != 1 {
		return nil
	}
	result, e := noderuntime.CallHelper(call, socket, noderuntime.ApplyRequest{ProtocolVersion: 1, Kind: "statistics_v1"})
	if e != nil {
		return e
	}
	result.Usage.NodeID = c.NodeID
	status, e := request(call, client, c, http.MethodPost, "/api/nodes/usage", result.Usage, nil)
	if e != nil {
		return e
	}
	if status == 404 {
		return nil
	} // Older controllers do not enable the accounting config.
	if status != http.StatusNoContent {
		return errors.New("node traffic report refused")
	}
	if caps.PresenceProtocol == 1 {
		// Presence cannot suppress successful accounting or debit the package.
		// Unavailable observations expire at the controller; no invented offline.
		if presence, e := noderuntime.CallHelper(call, socket, noderuntime.ApplyRequest{ProtocolVersion: 1, Kind: "presence_v1"}); e == nil && presence.Presence != nil {
			presence.Presence.NodeID = c.NodeID
			_, _ = request(call, client, c, http.MethodPost, "/api/nodes/presence", presence.Presence, nil)
		}
	}
	return nil
}

// Accounting cadence is independent of UI monitoring and long-running apply.
// Re-reading the atomic credential file makes rotation safe for this collector.
func runNodeUsage(ctx context.Context, client *http.Client, configPath string, report func(error)) {
	failures := 0
	for ctx.Err() == nil {
		c, e := ReadConfig(configPath)
		if e == nil {
			e = reportNodeUsage(ctx, client, c, "")
		}
		delay := 5 * time.Second
		if e != nil {
			if ctx.Err() != nil {
				return
			}
			if failures == 0 {
				report(errors.New("agent traffic accounting unavailable"))
			}
			if failures < 4 {
				failures++
			}
			delay = time.Duration(1<<failures) * 5 * time.Second
		} else {
			failures = 0
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
