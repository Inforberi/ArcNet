package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestTelemetryNegotiatesCadenceAndRejectsInvalidSettings(t *testing.T) {
	for _, fixture := range []struct {
		body     string
		status   int
		retry    string
		delay    time.Duration
		rejected bool
	}{
		{`{"interval_seconds":5}`, 200, "", 5 * time.Second, false},
		{`{"interval_seconds":60}`, 200, "", time.Minute, false},
		{`{"interval_seconds":1}`, 200, "", time.Minute, true},
		{`{broken`, 200, "", time.Minute, true},
		{"", 204, "", time.Minute, false},
		{"", 404, "", time.Minute, false},
		{"", 429, "15", 15 * time.Second, false},
		{"", 429, "999999", 10 * time.Second, false},
	} {
		t.Run(fixture.body+fixture.retry+http.StatusText(fixture.status), func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-ArcVPN-Telemetry") != "2" {
					t.Error("cadence capability not advertised")
				}
				w.Header().Set("Retry-After", fixture.retry)
				w.WriteHeader(fixture.status)
				_, _ = w.Write([]byte(fixture.body))
			}))
			defer srv.Close()
			delay, err := telemetryReport(context.Background(), srv.Client(), Configuration{Controller: srv.URL, NodeID: "node-a", Secret: "fixture-secret"}, metrics.NewSampler(t.TempDir()))
			if delay != fixture.delay || (err != nil) != fixture.rejected {
				t.Fatal(delay, err)
			}
		})
	}
}

func TestIndependentTelemetryCancelsAnInFlightReport(t *testing.T) {
	started := make(chan struct{}, 1)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Consume the bounded report, then simulate an unavailable controller.
		var body any
		_ = json.NewDecoder(r.Body).Decode(&body)
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "node.json")
	blob, _ := json.Marshal(Configuration{Controller: srv.URL, NodeID: "node-a", Secret: strings.Repeat("x", 48)})
	if err := os.WriteFile(path, blob, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		runTelemetry(ctx, srv.Client(), path, func(error) { t.Error("shutdown reported as failure") })
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("independent telemetry did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("telemetry leaked on cancellation")
	}
}
