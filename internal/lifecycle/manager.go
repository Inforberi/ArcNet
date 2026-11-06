package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
)

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}
type Manager struct {
	Runner  Runner
	Guard   Guard
	Plan    func(context.Context) (store.RuntimePlan, error)
	Prepare func(context.Context) error
	Apply   func(context.Context) error
}

type Component struct {
	Name    string `json:"name"`
	Desired string `json:"desired"`
	Actual  string `json:"actual"`
	Error   string `json:"error,omitempty"`
}
type Report struct {
	Paused     bool        `json:"paused"`
	Components []Component `json:"components"`
	Warnings   []string    `json:"warnings,omitempty"`
}

var units = []struct{ name, unit string }{
	{"panel", "arcvpn-panel.service"}, {"xray", "arcvpn-xray.service"},
	{"hysteria", "arcvpn-hysteria.service"}, {"wireguard", "arcvpn-wireguard.service"},
	{"amneziawg", "arcvpn-amneziawg.service"}, {"socket", "arcvpn-control.socket"}, {"helper", "arcvpn-control.service"},
}

func (m Manager) command(ctx context.Context, args ...string) ([]byte, error) {
	call, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	return m.Runner.Run(call, "/usr/bin/systemctl", args...)
}
func (m Manager) state(ctx context.Context, unit string) (string, error) {
	out, err := m.command(ctx, "show", "--property=LoadState,ActiveState", "--no-pager", unit)
	if err != nil || len(out) > 4096 {
		return "unknown", errors.New("systemd state unavailable")
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			fields[k] = v
		}
	}
	if fields["LoadState"] == "not-found" {
		return "not-installed", nil
	}
	if fields["LoadState"] != "loaded" {
		return "unknown", errors.New("unit is masked or cannot be loaded")
	}
	switch fields["ActiveState"] {
	case "active", "inactive", "failed", "activating", "deactivating":
		return fields["ActiveState"], nil
	}
	return "unknown", errors.New("unexpected systemd state")
}
func (m Manager) action(ctx context.Context, action, unit string, optional bool) error {
	state, err := m.state(ctx, unit)
	if err != nil {
		return err
	}
	if state == "not-installed" && optional {
		return nil
	}
	if action == "stop" && (state == "inactive" || state == "failed") {
		return nil
	}
	if action == "start" && state == "active" {
		return nil
	}
	if _, err := m.command(ctx, action, unit); err != nil {
		return fmt.Errorf("%s %s failed", action, unit)
	}
	state, err = m.state(ctx, unit)
	if err != nil {
		return err
	}
	if (action == "stop" && state != "inactive" && state != "failed") || ((action == "start" || action == "restart") && state != "active") {
		return fmt.Errorf("%s did not reach requested state", unit)
	}
	return nil
}

func desired(plan store.RuntimePlan, paused bool) map[string]bool {
	return map[string]bool{"panel": !paused, "xray": !paused && plan.Xray, "hysteria": !paused && plan.Hysteria, "wireguard": !paused && plan.WireGuard, "amneziawg": !paused && plan.LegacyAmnezia, "socket": !paused, "helper": !paused}
}

func (m Manager) Status(ctx context.Context) (Report, error) {
	report := Report{Components: []Component{}}
	guardErr := m.Guard.Check()
	if guardErr != nil && !errors.Is(guardErr, ErrPaused) {
		return report, guardErr
	}
	report.Paused = errors.Is(guardErr, ErrPaused)
	plan, planErr := m.Plan(ctx)
	wanted := desired(plan, report.Paused)
	var allErr error
	if planErr != nil {
		report.Warnings = append(report.Warnings, "configuration unavailable")
		allErr = errors.New("configuration unavailable")
	}
	for _, item := range units {
		state, err := m.state(ctx, item.unit)
		component := Component{Name: item.name, Desired: "stopped", Actual: state}
		if wanted[item.name] {
			component.Desired = "running"
		}
		if item.name == "helper" && !report.Paused {
			component.Desired = "on-demand"
		}
		if planErr != nil && !report.Paused && item.name != "panel" && item.name != "socket" && item.name != "helper" {
			component.Desired = "unknown"
		}
		ok := (state == "active" && wanted[item.name]) || ((state == "inactive" || state == "not-installed") && !wanted[item.name]) || (item.name == "helper" && !report.Paused && (state == "active" || state == "inactive"))
		if err != nil || !ok {
			component.Error = "desired and observed state differ or are unknown"
			allErr = errors.New("local runtime is partially unavailable")
		}
		report.Components = append(report.Components, component)
	}
	return report, allErr
}

// Quiesce prevents new helper starts before stopping the controller, then
// drains accounting while engines are still alive. Failure to stop the panel
// aborts, so a live writer can never race a root engine operation.
func (m Manager) quiesce(ctx context.Context, report *Report) error {
	if err := m.Guard.Pause(); err != nil {
		return err
	}
	if err := m.action(ctx, "stop", units[0].unit, true); err != nil {
		return err
	}
	if m.Prepare != nil {
		if err := m.Prepare(ctx); err != nil {
			report.Warnings = append(report.Warnings, "final counters unavailable; accounting may require review")
		}
	}
	var allErr error
	for _, index := range []int{5, 6, 4, 3, 2, 1} {
		if err := m.action(ctx, "stop", units[index].unit, true); err != nil {
			allErr = errors.Join(allErr, err)
		}
	}
	return allErr
}

func (m Manager) Execute(ctx context.Context, action, component string) (Report, error) {
	if action != "start" && action != "stop" && action != "restart" {
		return Report{}, errors.New("invalid lifecycle action")
	}
	if component != "" && component != "all" && component != "panel" && component != "xray" && component != "hysteria" && component != "wireguard" {
		return Report{}, errors.New("invalid lifecycle component")
	}
	if (action == "start" || action == "stop") && component != "" && component != "all" {
		return Report{}, errors.New("start/stop require all components")
	}
	unlock, err := m.Guard.Lock()
	if err != nil {
		return Report{}, err
	}
	defer unlock()
	report := Report{}
	// A component restart also uses the coordinated path: applying current
	// policy must precede any listener, including the legacy restart aliases.
	plan := store.RuntimePlan{}
	if action != "stop" {
		plan, err = m.Plan(ctx)
		if err != nil {
			return report, errors.New("cannot read runtime policy; no service changed")
		}
	}
	if action == "stop" && errors.Is(m.Guard.Check(), ErrPaused) {
		if current, err := m.Status(ctx); err == nil {
			return current, nil
		}
	}
	if component != "" && component != "all" {
		if err := m.Guard.Check(); err != nil {
			return report, err
		}
		if !desired(plan, false)[component] {
			return report, errors.New("component is disabled or not configured; no service changed")
		}
		if err := m.Guard.Pause(); err != nil {
			return report, err
		}
		if err := m.action(ctx, "stop", units[0].unit, false); err != nil {
			return m.failed(ctx, report, err, false)
		}
		if m.Prepare != nil && m.Prepare(ctx) != nil {
			report.Warnings = append(report.Warnings, "final counters unavailable; accounting may require review")
		}
		if err := m.action(ctx, "start", units[5].unit, false); err != nil {
			return m.failed(ctx, report, err, true)
		}
		if err := m.Guard.Resume(); err != nil {
			return m.failed(ctx, report, err, true)
		}
		if err := m.Apply(ctx); err != nil {
			return m.failed(ctx, report, errors.New("runtime reconciliation failed"), true)
		}
		if component != "panel" {
			for _, item := range units {
				if item.name == component {
					if err := m.action(ctx, "restart", item.unit, false); err != nil {
						return m.failed(ctx, report, err, true)
					}
				}
			}
		}
		if err := m.action(ctx, "start", units[0].unit, false); err != nil {
			return m.failed(ctx, report, err, true)
		}
		status, err := m.Status(ctx)
		status.Warnings = append(status.Warnings, report.Warnings...)
		return status, err
	}
	if action == "start" && m.Guard.Check() == nil {
		current, err := m.Status(ctx)
		if err == nil {
			return current, nil
		}
	}
	if err := m.quiesce(ctx, &report); err != nil {
		return m.failed(ctx, report, err, false)
	}
	if action != "stop" {
		if err := m.action(ctx, "start", units[5].unit, false); err != nil {
			return m.failed(ctx, report, err, false)
		}
		if err := m.Guard.Resume(); err != nil {
			return m.failed(ctx, report, err, true)
		}
		if err := m.Apply(ctx); err != nil {
			return m.failed(ctx, report, errors.New("runtime apply failed; configurations preserved"), true)
		}
		if err := m.action(ctx, "start", units[0].unit, false); err != nil {
			return m.failed(ctx, report, err, true)
		}
	}
	status, statusErr := m.Status(ctx)
	status.Warnings = append(status.Warnings, report.Warnings...)
	return status, statusErr
}

func (m Manager) failed(ctx context.Context, report Report, cause error, cleanup bool) (Report, error) {
	call, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
	defer cancel()
	if cleanup {
		if err := m.quiesce(call, &report); err != nil {
			cause = errors.Join(cause, fmt.Errorf("cleanup incomplete: %w", err))
		}
	}
	status, statusErr := m.Status(call)
	status.Warnings = append(status.Warnings, report.Warnings...)
	return status, errors.Join(cause, statusErr)
}
