package nodeagent

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/arcvpn/arcvpn/internal/publicnet"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/targetcheck"
)

// This typed read-only job runs in the unprivileged agent. It does not call the
// root helper, execute binaries, change Xray or mark any VPN probe as passed.
func PollTargetChecks(ctx context.Context, client *http.Client, c Configuration) error {
	var job *store.TargetJob
	status, err := request(ctx, client, c, http.MethodGet, "/api/nodes/target-checks/poll?node_id="+url.QueryEscape(c.NodeID), nil, &job)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || status == http.StatusNoContent {
		return nil
	} // older controller / no job
	if status != http.StatusOK || job == nil {
		return fmt.Errorf("target check poll rejected: HTTP %d", status)
	}
	if job.NodeID != c.NodeID {
		return fmt.Errorf("target check node mismatch")
	}
	checker := targetcheck.Checker{Guard: publicnet.New()}
	report, err := checker.Check(ctx, job.Candidates, c.NodeID)
	if err != nil {
		return err
	}
	status, err = request(ctx, client, c, http.MethodPost, "/api/nodes/target-checks/report", map[string]any{"id": job.ID, "node_id": c.NodeID, "digest": job.Digest, "report": report}, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("target check report rejected: HTTP %d", status)
	}
	return nil
}
