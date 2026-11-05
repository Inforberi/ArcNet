package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arcvpn/arcvpn/internal/metrics"
)

func TestTelemetryIsOptionalVersionedAndIdentityBound(t *testing.T) {
	for _, status := range []int{204, 404, 429, 401} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/nodes/telemetry" || r.Header.Get("Authorization") != "Bearer fixture-secret" {
					t.Error("incorrect telemetry identity")
				}
				var report struct {
					Version  int              `json:"version"`
					NodeID   string           `json:"node_id"`
					Snapshot metrics.Snapshot `json:"snapshot"`
				}
				if err := json.NewDecoder(r.Body).Decode(&report); err != nil || report.Version != 1 || report.NodeID != "node-a" {
					t.Errorf("wire: %#v %v", report, err)
				}
				w.WriteHeader(status)
			}))
			defer srv.Close()
			err := sendTelemetry(context.Background(), srv.Client(), Configuration{Controller: srv.URL, NodeID: "node-a", Secret: "fixture-secret"}, metrics.NewSampler(t.TempDir()))
			if (err != nil) != (status == 401) {
				t.Fatalf("status %d: %v", status, err)
			}
		})
	}
}
