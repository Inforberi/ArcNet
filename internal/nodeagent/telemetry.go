package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/arcvpn/arcvpn/internal/metrics"
	"github.com/arcvpn/arcvpn/internal/store"
)

// Resource telemetry is a separate optional v1 message, preserving the existing
// heartbeat contract. Older controllers return 404; their agent keeps working.
func sendTelemetry(ctx context.Context, client *http.Client, c Configuration, sampler *metrics.Sampler) error {
	_, err := telemetryReport(ctx, client, c, sampler)
	return err
}

func telemetryReport(ctx context.Context, client *http.Client, c Configuration, sampler *metrics.Sampler) (time.Duration, error) {
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	blob, err := json.Marshal(struct {
		Version  int              `json:"version"`
		NodeID   string           `json:"node_id"`
		Snapshot metrics.Snapshot `json:"snapshot"`
	}{Version: 1, NodeID: c.NodeID, Snapshot: sampler.Sample()})
	if err != nil {
		return time.Minute, err
	}
	req, err := http.NewRequestWithContext(call, http.MethodPost, c.Controller+"/api/nodes/telemetry", bytes.NewReader(blob))
	if err != nil {
		return time.Minute, errors.New("invalid telemetry endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+c.Secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ArcVPN-Telemetry", "2")
	response, err := client.Do(req)
	if err != nil {
		return time.Minute, errors.New("telemetry request unavailable")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case 200:
		var settings struct {
			Seconds int `json:"interval_seconds"`
		}
		if json.NewDecoder(http.MaxBytesReader(nil, response.Body, 1024)).Decode(&settings) != nil || !store.ValidMonitoringSeconds(settings.Seconds) {
			return time.Minute, errors.New("invalid telemetry settings")
		}
		return time.Duration(settings.Seconds) * time.Second, nil
	case 404, 204: // Older controllers keep their former one-minute cadence.
		return time.Minute, nil
	case 429:
		n, _ := strconv.Atoi(response.Header.Get("Retry-After"))
		if n < 5 || n > 300 {
			n = 10
		}
		return time.Duration(n) * time.Second, nil
	}
	return time.Minute, errors.New("resource telemetry rejected")
}

// This lifecycle is separate from apply/target checks. Configuration is reread
// atomically on each report so credential rotation cannot race with this loop.
func runTelemetry(ctx context.Context, client *http.Client, configPath string, report func(error)) {
	sampler := metrics.NewSampler(filepath.Dir(configPath))
	failures := 0
	for ctx.Err() == nil {
		c, err := ReadConfig(configPath)
		delay := time.Minute
		if err == nil {
			delay, err = telemetryReport(ctx, client, c, sampler)
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			report(errors.New("agent resource telemetry unavailable"))
			if failures < 4 {
				failures++
			}
			delay = time.Duration(1<<failures) * 15 * time.Second
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
