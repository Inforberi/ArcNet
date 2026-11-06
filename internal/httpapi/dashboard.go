package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/arcvpn/arcvpn/internal/metrics"
	"github.com/arcvpn/arcvpn/internal/store"
)

type serverDashboard struct {
	ID           string                   `json:"id"`
	Name         string                   `json:"name"`
	Address      string                   `json:"address"`
	Version      string                   `json:"version"`
	Agent        string                   `json:"agent"`
	Xray         string                   `json:"xray"`
	VPN          string                   `json:"vpn"`
	VPNCheckedAt int64                    `json:"vpn_checked_at"`
	Route        string                   `json:"route"`
	LastSeen     int64                    `json:"last_seen"`
	Stale        bool                     `json:"stale"`
	Legacy       bool                     `json:"legacy"`
	Telemetry    *store.ObservedTelemetry `json:"telemetry"`
}
type fleetItem struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Address  string   `json:"address"`
	Agent    string   `json:"agent"`
	Xray     string   `json:"xray"`
	VPN      string   `json:"vpn"`
	LastSeen int64    `json:"last_seen"`
	Stale    bool     `json:"stale"`
	Legacy   bool     `json:"legacy"`
	CPU      *float64 `json:"cpu_percent"`
	Memory   *float64 `json:"memory_percent"`
}

type serviceSnapshot struct {
	sync.Mutex
	name  string
	state string
	until time.Time
}

func (s *Server) serviceState(ctx context.Context, name string) string {
	s.serviceCache.Lock()
	defer s.serviceCache.Unlock()
	if s.serviceCache.name == name && time.Now().Before(s.serviceCache.until) {
		return s.serviceCache.state
	}
	call, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output, _ := exec.CommandContext(call, "/usr/bin/systemctl", "is-active", name).Output()
	state := strings.TrimSpace(string(output))
	switch state {
	case "active", "inactive", "failed", "activating", "deactivating":
	default:
		state = "unknown"
	}
	if ctx.Err() == nil {
		s.serviceCache.name, s.serviceCache.state, s.serviceCache.until = name, state, time.Now().Add(5*time.Second)
	}
	return state
}

// One bounded collector is owned by the serve lifecycle. It keeps history when
// no browser is open; there is no goroutine or polling loop per administrator.
func (s *Server) RunTelemetry(ctx context.Context) {
	defer s.monitoring.once.Do(func() { close(s.monitoring.done) })
	for ctx.Err() == nil {
		call, cancel := context.WithTimeout(ctx, 5*time.Second)
		s.telemetryWriteFailed.Store(s.App.Store.RecordLocalTelemetry(call, s.telemetry.Sample()) != nil)
		n, err := s.App.Store.MonitoringSeconds(call)
		if err != nil {
			n = store.DefaultMonitoringSeconds
		}
		cancel()
		s.monitoring.notify()
		timer := time.NewTimer(time.Duration(n) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.monitoring.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func localService(ctx context.Context, s *Server) (string, string, error) {
	reality, err := s.App.Store.OptionalReality(ctx)
	if err != nil {
		return "unknown", "", err
	}
	address := reality.Address
	if server, err := s.App.Store.LocalServer(ctx); err == nil {
		address = server.Address
	} else if !errors.Is(err, store.ErrNotFound) {
		return "unknown", "", err
	}
	profiles, err := s.App.Store.ListProtocols(ctx)
	if err != nil {
		return "unknown", address, err
	}
	configured, enabled := reality.Address != "", reality.Enabled
	for _, p := range profiles {
		if p.Kind == "vless-xhttp" {
			configured = true
			enabled = enabled || p.Enabled
		}
	}
	if !configured {
		return "not-configured", address, nil
	}
	if !enabled {
		return "disabled", address, nil
	}
	if s.App.Core.SkipRestart {
		return "unverified", address, nil
	}
	if s.App.Core.Service == "" {
		return "unknown", address, nil
	}
	return s.serviceState(ctx, s.App.Core.Service), address, nil
}
func (s *Server) localDashboard(ctx context.Context) (serverDashboard, bool, error) {
	snapshot := s.telemetry.Sample()
	stored := !s.telemetryWriteFailed.Load()
	state, address, err := localService(ctx, s)
	if err != nil {
		return serverDashboard{}, stored, err
	}
	return serverDashboard{ID: "local", Name: "Сервер панели", Address: address, Agent: "local", Xray: state, VPN: "unknown", Route: "not-verified", LastSeen: time.Now().Unix(), Telemetry: &store.ObservedTelemetry{ReceivedAt: snapshot.SampledAt, Snapshot: snapshot}}, stored, nil
}
func remoteDashboard(node store.Node, observed *store.ObservedTelemetry) serverDashboard {
	result := serverDashboard{ID: node.ID, Name: node.Name, Address: node.Control.Address, Version: node.Version, Agent: "offline", Xray: "unknown", VPN: node.Control.Probe, VPNCheckedAt: node.Control.ProbeCheckedAt, Route: "not-verified", LastSeen: node.LastSeen, Stale: true, Telemetry: observed, Legacy: observed == nil}
	if result.VPN == "" {
		result.VPN = "unknown"
	}
	if !node.Online && result.VPN == "pass" {
		result.VPN = "stale"
	}
	if node.Online {
		result.Agent = "online"
		result.Xray = node.XrayState
	}
	if observed != nil {
		age := time.Now().Unix() - observed.ReceivedAt
		result.Stale = !node.Online || age < 0 || age > metrics.FreshForSeconds
		return result
	}
	if node.LastSeen > 0 {
		sample := metrics.Snapshot{SampledAt: node.LastSeen}
		if node.MemoryTotal > 0 {
			sample.MemoryTotal = &node.MemoryTotal
			sample.MemoryAvailable = &node.MemoryFree
		}
		sample.Load1 = &node.Load1
		sample.UptimeSeconds = &node.Uptime
		result.Telemetry = &store.ObservedTelemetry{ReceivedAt: node.LastSeen, Snapshot: sample}
		result.Stale = !node.Online
	}
	return result
}
func summarizeServer(server serverDashboard) fleetItem {
	result := fleetItem{ID: server.ID, Name: server.Name, Address: server.Address, Agent: server.Agent, Xray: server.Xray, VPN: server.VPN, LastSeen: server.LastSeen, Stale: server.Stale, Legacy: server.Legacy}
	if !server.Stale && server.Telemetry != nil {
		data := server.Telemetry.Snapshot
		result.CPU = data.CPUPercent
		if data.MemoryTotal != nil && *data.MemoryTotal > 0 && data.MemoryAvailable != nil {
			v := 100 * float64(*data.MemoryTotal-*data.MemoryAvailable) / float64(*data.MemoryTotal)
			result.Memory = &v
		}
	}
	return result
}
func (s *Server) dashboardFleet(ctx context.Context) ([]fleetItem, error) {
	local, _, err := s.localDashboard(ctx)
	if err != nil {
		return nil, err
	}
	nodes, err := s.App.Store.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	observed, err := s.App.Store.FleetTelemetry(ctx)
	if err != nil {
		return nil, err
	}
	result := []fleetItem{summarizeServer(local)}
	for _, node := range nodes {
		var snapshot *store.ObservedTelemetry
		if value, ok := observed[node.ID]; ok {
			snapshot = &value
		}
		result = append(result, summarizeServer(remoteDashboard(node, snapshot)))
	}
	return result, nil
}
func (s *Server) serverTelemetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var selected serverDashboard
	stored := true
	if id == "local" {
		var err error
		selected, stored, err = s.localDashboard(r.Context())
		if err != nil {
			fail(w, 500, "Не удалось получить состояние сервера")
			return
		}
	} else {
		nodes, err := s.App.Store.ListNodes(r.Context())
		if err != nil {
			fail(w, 500, "Не удалось получить серверы")
			return
		}
		found := false
		for _, node := range nodes {
			if node.ID != id {
				continue
			}
			observed, err := s.App.Store.LatestTelemetry(r.Context(), id)
			if err != nil {
				fail(w, 500, "Не удалось прочитать телеметрию")
				return
			}
			selected = remoteDashboard(node, observed)
			found = true
			break
		}
		if !found {
			fail(w, 404, "Сервер не найден")
			return
		}
	}
	history := []store.ObservedTelemetry{}
	includeHistory := r.URL.Query().Get("history") != "0"
	if includeHistory {
		var err error
		history, err = s.App.Store.TelemetryHistory(r.Context(), id)
		if err != nil {
			history = []store.ObservedTelemetry{}
			stored = false
		}
	}
	respond(w, 200, map[string]any{"sampled_at": time.Now().Unix(), "server": selected, "history": history, "history_available": stored, "history_included": includeHistory, "telemetry_fresh_for": metrics.FreshForSeconds, "history_scope": "host-network-and-resources", "vpn_traffic_verified": false})
}
func (s *Server) nodeTelemetry(w http.ResponseWriter, r *http.Request) {
	var report struct {
		Version  int              `json:"version"`
		NodeID   string           `json:"node_id"`
		Snapshot metrics.Snapshot `json:"snapshot"`
	}
	if !decode(w, r, &report) {
		return
	}
	token := agentBearer(r)
	if report.NodeID == "local" || report.NodeID == "" || token == "" {
		http.Error(w, "Unauthorized", 401)
		return
	}
	if report.Version != 1 {
		fail(w, 400, "Unsupported telemetry version")
		return
	}
	err := s.App.Store.RecordTelemetry(r.Context(), report.NodeID, token, report.Snapshot)
	if errors.Is(err, store.ErrNodeUnauthorized) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	if errors.Is(err, store.ErrTelemetryRateLimit) {
		w.Header().Set("Retry-After", "5")
		fail(w, 429, "Telemetry rate limited")
		return
	}
	if err != nil {
		fail(w, 400, "Invalid or unavailable telemetry")
		return
	}
	if r.Header.Get("X-ArcVPN-Telemetry") == "2" {
		n, err := s.App.Store.MonitoringSeconds(r.Context())
		if err != nil {
			fail(w, 503, "Monitoring settings unavailable")
			return
		}
		respond(w, 200, map[string]int{"interval_seconds": n})
		return
	}
	w.WriteHeader(204)
}
