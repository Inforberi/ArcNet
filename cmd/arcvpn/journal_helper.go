package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/arcvpn/arcvpn/internal/firewall"
	"github.com/arcvpn/arcvpn/internal/nodeagent"
	"github.com/arcvpn/arcvpn/internal/ops"
	"github.com/arcvpn/arcvpn/internal/updater"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const journalDir = "/etc/systemd/journald@arcvpn.conf.d"
const journalConfig = journalDir + "/90-arcvpn-budget.conf"

func parseJournalBudget(line string) (int, error) {
	if !strings.HasPrefix(line, "journal-budget ") || !strings.HasSuffix(line, "\n") {
		return 0, errors.New("invalid journal-budget request")
	}
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(line, "journal-budget "), "\n"))
	if err != nil || n < 50 || n > 1024 {
		return 0, errors.New("journal budget out of range")
	}
	return n, nil
}
func journalBudgetRead() (int, error) {
	contents, err := os.ReadFile(journalConfig)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(contents), "\n") {
		if strings.HasPrefix(line, "SystemMaxUse=") && strings.HasSuffix(line, "M") {
			n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(line, "SystemMaxUse="), "M"))
			if err == nil && n >= 50 && n <= 1024 {
				return n, nil
			}
		}
	}
	return 0, errors.New("invalid ArcVPN journal config")
}
func journalBudgetWrite(limit int) error {
	if limit < 50 || limit > 1024 {
		return errors.New("invalid journal budget")
	}
	st, err := os.Lstat(journalDir)
	if err != nil || !st.IsDir() || st.Mode().Perm()&0022 != 0 || !rootOwned(st) {
		return errors.New("unsafe journal config directory")
	}
	cfgInfo, err := os.Lstat(journalConfig)
	if err != nil || !cfgInfo.Mode().IsRegular() || !rootOwned(cfgInfo) {
		return errors.New("unsafe journal config file")
	}
	previous, err := os.ReadFile(journalConfig)
	if err != nil {
		return err
	}
	current, err := journalBudgetRead()
	if err != nil {
		return err
	}
	if current == limit {
		return nil
	}
	updated := []byte(fmt.Sprintf("[Journal]\nStorage=persistent\nSystemMaxUse=%dM\nRuntimeMaxUse=%dM\nSystemMaxFileSize=16M\nRuntimeMaxFileSize=16M\n", limit, limit))
	if err := writeJournalConfig(updated); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = exec.CommandContext(ctx, "/usr/bin/systemctl", "try-restart", "systemd-journald@arcvpn.service").Run(); err != nil {
		_ = writeJournalConfig(previous)
		return errors.New("journal restart failed; previous settings restored")
	}
	// Vacuum only the ArcVPN namespace. This cannot remove unrelated host logs.
	_ = exec.CommandContext(ctx, "/usr/bin/journalctl", "--namespace=arcvpn", "--rotate", "--vacuum-size="+strconv.Itoa(limit)+"M").Run()
	return nil
}
func writeJournalConfig(data []byte) error {
	f, err := os.CreateTemp(journalDir, ".arcvpn-journal-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), journalConfig); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(journalConfig))
	if err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func rootOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0
}

// sendFirewall only invokes the fixed UFW adapter. It cannot enable/disable UFW
// or alter arbitrary SSH, administrator or third-party firewall rules.
// Mutation dispatch stays disabled until isolated-VM rollback acceptance.
func firewallReadCommand(line string) bool { return line == "firewall status\n" }

func sendFirewall(conn io.Writer, action string, rule ops.FirewallRule) {
	var result ops.FirewallStatus
	var err error
	srv := firewall.Service{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if action == "status" {
		result, err = srv.Status(ctx)
	} else {
		result, err = srv.Apply(ctx, action, rule)
	}
	if err != nil {
		_, _ = conn.Write([]byte("error\n"))
		return
	}
	_ = json.NewEncoder(conn).Encode(result)
}

func updateCommand(line string) (string, bool) {
	switch line {
	case "update status\n":
		return "status", true
	case "update verify\n":
		return "verify", true
	case "update apply\n":
		return "apply", true
	}
	return "", false
}
func sendUpdate(conn io.Writer, action string) {
	status := ops.UpdateStatus{}
	ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	output, unitErr := exec.CommandContext(ctx, "/usr/bin/systemctl", "show", "arcvpn-update.service", "-p", "LoadState", "-p", "ActiveState", "-p", "Result", "-p", "ExecMainStartTimestampMonotonic").Output()
	unit, parseErr := parseUpdateUnit(output)
	if unitErr != nil || parseErr != nil {
		_, _ = conn.Write([]byte("error\n"))
		return
	}
	status.Busy = unit.Busy
	status.Result = unit.Result
	status.StartedAt = unit.StartedAt
	if action != "status" && status.Busy {
		_, _ = conn.Write([]byte("error\n"))
		return
	}
	_, publicErr := updater.ReadPublicKey(updater.PublicKeyFile)
	status.Trusted = publicErr == nil
	if action == "status" {
		_, err := os.Stat(updater.StagedArchive)
		status.Staged = err == nil
		if status.Trusted && status.Staged {
			if manifest, e := (updater.Installer{CurrentVersion: nodeagent.Version}).Verify(); e == nil {
				status.Version = manifest.Version
			}
		}
	} else {
		manifest, err := (updater.Installer{CurrentVersion: nodeagent.Version}).Verify()
		if err != nil {
			_, _ = conn.Write([]byte("error\n"))
			return
		}
		status.Staged = true
		status.Version = manifest.Version
		if action == "apply" {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			if e := exec.CommandContext(ctx, "/usr/bin/systemctl", "start", "--no-block", "arcvpn-update.service").Run(); e != nil {
				_, _ = conn.Write([]byte("error\n"))
				return
			}
			status.Accepted = true
		}
	}
	_ = json.NewEncoder(conn).Encode(status)
}

func parseUpdateUnit(output []byte) (ops.UpdateStatus, error) {
	values := map[string]string{}
	for _, line := range strings.Split(string(output), "\n") {
		p := strings.SplitN(line, "=", 2)
		if len(p) == 2 {
			values[p[0]] = p[1]
		}
	}
	started, err := strconv.ParseUint(values["ExecMainStartTimestampMonotonic"], 10, 64)
	if values["LoadState"] != "loaded" || values["Result"] == "" || err != nil {
		return ops.UpdateStatus{}, errors.New("unknown update unit")
	}
	state := ops.UpdateStatus{Result: values["Result"], StartedAt: started}
	switch values["ActiveState"] {
	case "active", "activating", "deactivating":
		state.Busy = true
		return state, nil
	case "inactive", "failed":
		return state, nil
	}
	return ops.UpdateStatus{}, errors.New("unknown update state")
}
