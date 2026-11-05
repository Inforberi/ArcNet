package nodeagent

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/arcvpn/arcvpn/internal/metrics"
)

// Resource telemetry is a separate optional v1 message, preserving the existing
// heartbeat contract. Older controllers return 404; their agent keeps working.
func sendTelemetry(ctx context.Context, client *http.Client, c Configuration, sampler *metrics.Sampler) error {
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	status, err := request(call, client, c, http.MethodPost, "/api/nodes/telemetry", struct {
		Version  int              `json:"version"`
		NodeID   string           `json:"node_id"`
		Snapshot metrics.Snapshot `json:"snapshot"`
	}{Version: 1, NodeID: c.NodeID, Snapshot: sampler.Sample()}, nil)
	if err != nil {
		return errors.New("telemetry request unavailable")
	}
	if status == 404 || status == 204 || status == 429 {
		return nil
	}
	return errors.New("resource telemetry rejected")
}
