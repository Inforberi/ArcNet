// Package engines applies strictly generated configurations of auxiliary VPN
// daemons. It does not download binaries or accept arbitrary shell commands.
package engines

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

type Manager struct {
	Dir            string
	HelperSocket   string
	HysteriaBinary string
	WGQuickBinary  string
	AWGQuickBinary string
	SkipRestart    bool // Tests only. Never set by the production installer.
}

type target struct {
	kind, filename, command, validator string
	enabled                            bool
	data                               []byte
}

func (m Manager) targets(profiles []store.Protocol, users []store.User, credentials []store.ProtocolCredential) ([]target, error) {
	result := make([]target, 0, 3)
	for _, p := range profiles {
		if p.Kind == protocols.XHTTP || p.Kind == "vless-reality" {
			continue
		}
		t := target{kind: p.Kind, enabled: p.Enabled}
		switch p.Kind {
		case protocols.Hysteria2:
			t.filename = "hysteria.yaml"
			t.command = "hysteria"
			t.validator = m.HysteriaBinary
		case protocols.WireGuard:
			t.filename = "arcwg.conf"
			t.command = "wireguard"
			t.validator = m.WGQuickBinary
		case protocols.AmneziaWG:
			t.filename = "arcawg.conf"
			t.command = "amneziawg"
			t.validator = m.AWGQuickBinary
		default:
			return nil, protocols.ErrInvalid
		}
		config, err := protocols.Render(p, users, credentials)
		if err != nil {
			return nil, &OperationError{Kind: p.Kind, Stage: "configuration", Err: err}
		}
		if p.Kind == protocols.Hysteria2 {
			config, err = m.withHysteriaStatistics(config)
			if err != nil {
				return nil, err
			}
		}
		t.data = config
		result = append(result, t)
	}
	return result, nil
}

type priorState struct {
	data   []byte
	exists bool
	active bool
}

// Apply validates all generated configuration files before it changes or
// removes any service. On failure it restores both the files and the service
// states observed before the operation. A failed rollback is reported.
func (m Manager) Apply(ctx context.Context, profiles []store.Protocol, users []store.User, credentials []store.ProtocolCredential) error {
	if m.Dir == "" {
		return errors.New("engine directory not configured")
	}
	if err := os.MkdirAll(m.Dir, 0700); err != nil {
		return err
	}
	targets, err := m.targets(profiles, users, credentials)
	if err != nil {
		return err
	}
	all := []target{
		{kind: protocols.Hysteria2, filename: "hysteria.yaml", command: "hysteria"},
		{kind: protocols.WireGuard, filename: "arcwg.conf", command: "wireguard"},
		{kind: protocols.AmneziaWG, filename: "arcawg.conf", command: "amneziawg"},
	}
	prior := make(map[string]priorState, len(all))
	wanted := make(map[string]target, len(targets))
	for _, t := range targets {
		if _, exists := wanted[t.kind]; exists {
			return errors.New("duplicate protocol engine")
		}
		wanted[t.kind] = t
	}
	// Capture all old files, including profiles removed by a portable restore.
	// No service is stopped until every new config has passed validation.
	for _, t := range all {
		name := filepath.Join(m.Dir, t.filename)
		info, err := os.Lstat(name)
		if os.IsNotExist(err) {
			prior[t.kind] = priorState{}
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("engine config %s is not a regular file", name)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		prior[t.kind] = priorState{data: data, exists: true}
	}
	stageDir, err := os.MkdirTemp(m.Dir, ".stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageDir)
	for _, t := range targets {
		if t.enabled && !m.SkipRestart {
			if t.validator == "" {
				return &OperationError{Kind: t.kind, Stage: "runtime", Err: errors.New("engine binary not configured")}
			}
			info, err := os.Stat(t.validator)
			if err == nil && !info.Mode().IsRegular() {
				err = errors.New("engine binary is not a regular file")
			}
			if err != nil {
				return &OperationError{Kind: t.kind, Stage: "runtime", Err: err}
			}
		}
		stage := filepath.Join(stageDir, t.filename)
		if err := os.WriteFile(stage, t.data, 0600); err != nil {
			return err
		}
		if t.enabled && !m.SkipRestart && (t.kind == protocols.WireGuard || t.kind == protocols.AmneziaWG) {
			validateCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			err := exec.CommandContext(validateCtx, t.validator, "strip", stage).Run()
			cancel()
			if err != nil {
				// Some validators echo the offending configuration line, including
				// private keys. Never propagate their stdout/stderr into logs.
				return &OperationError{Kind: t.kind, Stage: "validation", Err: err}
			}
		}
	}
	// Query observed states before changing files. We restore those exact states
	// after a failed activation rather than starting formerly disabled services.
	if !m.SkipRestart {
		for _, t := range all {
			active, err := m.serviceActive(ctx, t.command)
			if err != nil {
				return &OperationError{Kind: t.kind, Stage: "status", Err: err}
			}
			v := prior[t.kind]
			v.active = active
			prior[t.kind] = v
		}
	}
	// A changed file is staged atomically; a removed file is deleted only after
	// its service is stopped. Any failure restores all snapshots.
	changed := make(map[string]bool, len(all))
	for _, t := range targets {
		old := prior[t.kind]
		if old.exists && bytes.Equal(old.data, t.data) {
			continue
		}
		if err := os.Rename(filepath.Join(stageDir, t.filename), filepath.Join(m.Dir, t.filename)); err != nil {
			return errors.Join(err, m.restoreFiles(prior, all, changed))
		}
		changed[t.kind] = true
	}
	var applyErr error
	_, receiptErr := os.Lstat("/run/arcvpn/wireguard-epoch.json")
	for _, t := range all {
		desired, present := wanted[t.kind]
		old := prior[t.kind]
		if !m.SkipRestart {
			switch {
			case present && desired.enabled && (changed[t.kind] || !old.active || (t.kind == protocols.WireGuard && os.IsNotExist(receiptErr))):
				applyErr = m.control(ctx, "restart", t.command)
			case (!present || !desired.enabled) && old.active:
				applyErr = m.control(ctx, "stop", t.command)
			}
			if applyErr != nil {
				applyErr = &OperationError{Kind: t.kind, Stage: "restart", Err: applyErr}
				break
			}
		}
		if !present && old.exists {
			if err := os.Remove(filepath.Join(m.Dir, t.filename)); err != nil {
				applyErr = fmt.Errorf("remove obsolete %s: %w", t.filename, err)
				break
			}
			changed[t.kind] = true
		}
	}
	if applyErr == nil {
		return nil
	}
	recoverCtx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	restoreErr := m.restoreFiles(prior, all, changed)
	if !m.SkipRestart {
		for _, t := range all {
			old := prior[t.kind]
			if old.active {
				restoreErr = errors.Join(restoreErr, m.control(recoverCtx, "restart", t.command))
			} else {
				restoreErr = errors.Join(restoreErr, m.control(recoverCtx, "stop", t.command))
			}
		}
	}
	if restoreErr != nil {
		kind := "vpn"
		var operation *OperationError
		if errors.As(applyErr, &operation) {
			kind = operation.Kind
		}
		return &OperationError{Kind: kind, Stage: "rollback", Err: errors.Join(applyErr, restoreErr)}
	}
	return fmt.Errorf("engine apply failed, previous files and service states restored: %w", applyErr)
}

// restoreFiles restores all paths changed by the current operation. If a prior
// file did not exist, its newly installed replacement is removed.
func (m Manager) restoreFiles(prior map[string]priorState, all []target, changed map[string]bool) error {
	var errs []error
	for _, t := range all {
		if !changed[t.kind] {
			continue
		}
		old := prior[t.kind]
		name := filepath.Join(m.Dir, t.filename)
		if !old.exists {
			if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
				errs = append(errs, err)
			}
			continue
		}
		tmp, err := os.CreateTemp(m.Dir, ".rollback-")
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err = tmp.Chmod(0600); err == nil {
			_, err = tmp.Write(old.data)
		}
		if closeErr := tmp.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(tmp.Name(), name)
		}
		if err != nil {
			errs = append(errs, err)
			_ = os.Remove(tmp.Name())
		}
	}
	return errors.Join(errs...)
}

func (m Manager) serviceActive(ctx context.Context, kind string) (bool, error) {
	response, err := m.command(ctx, "status", kind)
	if err != nil {
		return false, err
	}
	switch response {
	case "active\n":
		return true, nil
	case "inactive\n":
		return false, nil
	default:
		return false, errors.New("restricted helper returned invalid service state")
	}
}

func (m Manager) control(ctx context.Context, action, kind string) error {
	response, err := m.command(ctx, action, kind)
	if err != nil {
		return err
	}
	if response != "ok\n" {
		return errors.New("restricted helper rejected operation")
	}
	return nil
}

func (m Manager) command(ctx context.Context, action, kind string) (string, error) {
	if m.HelperSocket == "" {
		return "", errors.New("privileged helper socket unavailable")
	}
	c, cancel := context.WithTimeout(ctx, 16*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(c, "unix", m.HelperSocket)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	deadline := time.Now().Add(15 * time.Second)
	if d, ok := c.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return "", err
	}
	if _, err := io.WriteString(conn, action+" "+kind+"\n"); err != nil {
		return "", err
	}
	answer, err := bufio.NewReader(io.LimitReader(conn, 128)).ReadString('\n')
	if err != nil {
		return "", err
	}
	return answer, nil
}

// RemoveDisabled clears stale secret-bearing generated files after a protocol
// has first been disabled and its systemd service stopped.
func (m Manager) RemoveDisabled(kind string) error {
	var name string
	switch kind {
	case protocols.XHTTP:
		return nil
	case protocols.Hysteria2:
		name = "hysteria.yaml"
	case protocols.WireGuard:
		name = "arcwg.conf"
	case protocols.AmneziaWG:
		name = "arcawg.conf"
	default:
		return protocols.ErrInvalid
	}
	if m.Dir == "" {
		return errors.New("engine directory not configured")
	}
	if err := os.Remove(filepath.Join(m.Dir, name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
