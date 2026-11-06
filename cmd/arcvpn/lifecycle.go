package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/engines"
	"github.com/arcvpn/arcvpn/internal/lifecycle"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

type lifecycleExec struct{}

func (lifecycleExec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	// runuser can fork the service-user core. Cancellation must terminate the
	// entire private child group, not leave an orphan writer after cleanup.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 2 * time.Second
	var out boundedOutput
	out.limit = (32 << 10) + 1
	cmd.Stdout, cmd.Stderr = &out, io.Discard
	err := cmd.Run()
	if len(out.data) > 32<<10 {
		return nil, errors.New("management output exceeded limit")
	}
	return out.data, err
}

func localRuntimeApp(st *store.Store, dir string) *app.App {
	return &app.App{Store: st, RuntimeCheck: (lifecycle.Guard{}).Check, Core: xray.Controller{Binary: "/opt/arcvpn/bin/xray", Path: filepath.Join(dir, "xray.json"), Service: "arcvpn-xray.service", ControlSocket: "/run/arcvpn-control.sock"}, Engines: &engines.Manager{Dir: dir, HelperSocket: "/run/arcvpn-control.sock", HysteriaBinary: "/opt/arcvpn/bin/hysteria", WGQuickBinary: "/usr/bin/wg-quick", AWGQuickBinary: "/opt/arcvpn/bin/awg-quick"}}
}

type lifecycleError struct {
	code int
	err  error
}

func (e lifecycleError) Error() string { return e.err.Error() }
func (e lifecycleError) Unwrap() error { return e.err }
func (e lifecycleError) ExitCode() int { return e.code }

func runtimeChild(ctx context.Context, runner lifecycle.Runner, command string) ([]byte, error) {
	call, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	return runner.Run(call, "/usr/sbin/runuser", "-u", "arcvpn", "--", "/usr/bin/env", "ARCVPN_DATA_DIR=/var/lib/arcvpn", "/opt/arcvpn/bin/arcvpn-core", command)
}

func runLifecycle(args []string) error {
	if os.Geteuid() != 0 {
		return errors.New("use sudo arcvpn for local lifecycle management")
	}
	if len(args) < 1 || len(args) > 2 {
		return lifecycleError{2, errors.New("lifecycle: start|stop|restart | restart panel|xray|hysteria|wireguard | status [--json] | logs [panel|xray|hysteria|wireguard]")}
	}
	action, component := args[0], ""
	if len(args) == 2 {
		component = args[1]
	}
	if action != "start" && action != "stop" && action != "restart" && action != "status" && action != "logs" {
		return lifecycleError{2, errors.New("invalid lifecycle action")}
	}
	if (action == "status" && component != "" && component != "--json") || (action != "status" && component != "" && component != "panel" && component != "xray" && component != "hysteria" && component != "wireguard") {
		return lifecycleError{2, errors.New("invalid lifecycle component")}
	}
	if (action == "start" || action == "stop") && component != "" {
		return lifecycleError{2, errors.New("start/stop operate on the whole local installation")}
	}
	signals, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signals, 3*time.Minute)
	defer cancel()
	runner := lifecycleExec{}
	if action == "logs" {
		units := map[string]string{"panel": "arcvpn-panel.service", "xray": "arcvpn-xray.service", "hysteria": "arcvpn-hysteria.service", "wireguard": "arcvpn-wireguard.service"}
		args := []string{"--namespace=arcvpn", "--no-pager", "--quiet", "-n", "80", "--output=short-iso"}
		for _, key := range []string{"panel", "xray", "hysteria", "wireguard"} {
			if component == "" || component == key {
				args = append(args, "--unit", units[key])
			}
		}
		out, err := runner.Run(ctx, "/usr/bin/journalctl", args...)
		if err != nil {
			return errors.New("ArcVPN journals unavailable")
		}
		_, err = os.Stdout.Write(out)
		return err
	}
	manager := lifecycle.Manager{Runner: runner, Plan: func(ctx context.Context) (store.RuntimePlan, error) {
		out, err := runtimeChild(ctx, runner, "runtime-plan")
		if err != nil || len(out) > 4096 {
			return store.RuntimePlan{}, errors.New("runtime policy unavailable")
		}
		return parseRuntimePlan(out)
	}, Prepare: func(ctx context.Context) error { _, err := runtimeChild(ctx, runner, "runtime-prepare"); return err }, Apply: func(ctx context.Context) error { _, err := runtimeChild(ctx, runner, "runtime-apply"); return err }}
	var report lifecycle.Report
	var err error
	if action == "status" {
		report, err = manager.Status(ctx)
	} else {
		report, err = manager.Execute(ctx, action, component)
	}
	if component == "--json" {
		if writeErr := json.NewEncoder(os.Stdout).Encode(report); writeErr != nil {
			return writeErr
		}
	} else {
		for _, c := range report.Components {
			fmt.Fprintf(os.Stdout, "%s: desired=%s actual=%s", c.Name, c.Desired, c.Actual)
			if c.Error != "" {
				fmt.Fprintf(os.Stdout, " (%s)", c.Error)
			}
			fmt.Fprintln(os.Stdout)
		}
		for _, warning := range report.Warnings {
			fmt.Fprintln(os.Stdout, "Предупреждение:", warning)
		}
	}
	if errors.Is(err, lifecycle.ErrBusy) {
		return lifecycleError{3, err}
	}
	return err
}

func parseRuntimePlan(out []byte) (store.RuntimePlan, error) {
	if len(out) > 4096 {
		return store.RuntimePlan{}, errors.New("runtime policy exceeded limit")
	}
	var value struct {
		Xray          *bool `json:"xray"`
		Hysteria      *bool `json:"hysteria"`
		WireGuard     *bool `json:"wireguard"`
		LegacyAmnezia *bool `json:"legacy_amnezia"`
	}
	decoder := json.NewDecoder(bytes.NewReader(out))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.Xray == nil || value.Hysteria == nil || value.WireGuard == nil || value.LegacyAmnezia == nil {
		return store.RuntimePlan{}, errors.New("invalid or incomplete runtime policy")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return store.RuntimePlan{}, errors.New("invalid runtime policy trailing data")
	}
	return store.RuntimePlan{Xray: *value.Xray, Hysteria: *value.Hysteria, WireGuard: *value.WireGuard, LegacyAmnezia: *value.LegacyAmnezia}, nil
}
