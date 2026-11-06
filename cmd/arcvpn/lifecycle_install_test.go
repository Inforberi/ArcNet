package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/lifecycle"
)

func TestInstallerUnitsShareTheRuntimePauseBoundary(t *testing.T) {
	data, err := os.ReadFile("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, name := range []string{"panel", "xray", "hysteria", "wireguard", "amneziawg"} {
		_, rest, ok := strings.Cut(source, `cat > "$SYSTEMD/arcvpn-`+name+`.service" <<UNIT`)
		if !ok {
			t.Fatal("unit template missing", name)
		}
		unit, _, _ := strings.Cut(rest, "\nUNIT")
		if !strings.Contains(unit, "ConditionPathExists=!"+lifecycle.Directory+"/paused") {
			t.Fatal("unit can bypass global stop", name)
		}
	}
	if !strings.Contains(source, "d "+lifecycle.Directory+" 0755 root root -") || !strings.Contains(source, "/run/arcvpn-update "+lifecycle.Directory) {
		t.Fatal("boot directory or updater write boundary drifted")
	}
}

func TestCertificateHookKeepsRenewalButDoesNotWakeStoppedPanel(t *testing.T) {
	data, err := os.ReadFile("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(data), "cat > \"$OPT/bin/arcvpn-cert-deploy\" <<DEPLOY\n")
	if !ok {
		t.Fatal("deploy hook missing")
	}
	hook, _, ok := strings.Cut(rest, "\nDEPLOY")
	if !ok {
		t.Fatal("deploy hook terminator missing")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "paused")
	log := filepath.Join(dir, "calls")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$(basename \"$0\")\" \"$@\" >> \"$ARCVPN_HOOK_TEST_LOG\"\n"
	for _, name := range []string{"install", "systemctl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(stub), 0700); err != nil {
			t.Fatal(err)
		}
	}
	hook = strings.ReplaceAll(hook, lifecycle.Directory+"/paused", marker)
	hook = strings.ReplaceAll(hook, "/usr/bin/systemctl", filepath.Join(dir, "systemctl"))
	path := filepath.Join(dir, "hook")
	if err := os.WriteFile(path, []byte(hook), 0700); err != nil {
		t.Fatal(err)
	}
	for _, paused := range []bool{true, false} {
		if paused {
			if err := os.WriteFile(marker, nil, 0600); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
		}
		_ = os.Remove(log)
		cmd := exec.Command("bash", path)
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "ARCVPN_HOOK_TEST_LOG="+log, "FULLCHAIN=fixture-cert", "PRIVATEKEY=fixture-key", "CONFIG_DIR="+dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
		calls, _ := os.ReadFile(log)
		if strings.Count(string(calls), "install\n") != 2 {
			t.Fatal("renewed files were not installed", string(calls))
		}
		if strings.Contains(string(calls), "try-restart") == paused {
			t.Fatal("paused panel wake policy", string(calls))
		}
	}
}
