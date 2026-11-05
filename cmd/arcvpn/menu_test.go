package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Only stub executables are available to the root-operated wrapper in this
// fixture. No real service is queried, restarted or modified.
func TestManagementWrapperDispatchesOnlyAllowlistedCommands(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root wrapper fixture in disposable Linux container")
	}
	root, err := filepath.Abs("../../deploy/arcvpn-menu.sh")
	if err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	source, err := os.ReadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(temp, "arcvpn-menu.sh")
	if err = os.WriteFile(root, source, 0755); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(temp, "calls")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$0\" \"$@\" >> \"$ARCVPN_WRAPPER_TEST_LOG\"\ncase \"$0\" in *systemctl) printf '0\\n';; esac\n"
	for _, name := range []string{"runuser", "systemctl", "journalctl"} {
		if err := os.WriteFile(filepath.Join(temp, name), []byte(stub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		args        []string
		input, want string
		valid       bool
	}{
		{args: []string{"settings", "--json"}, want: "settings\n--panel-pid\n0\n--json", valid: true},
		{args: []string{"password", "reset", "--password-stdin"}, want: "admin-reset\n--password-stdin", valid: true},
		{args: []string{"password", "reset", "--reset-2fa"}, want: "admin-reset\n--reset-2fa", valid: true},
		{args: []string{"sessions", "revoke"}, want: "admin-revoke-sessions", valid: true},
		{args: []string{"logs", "panel"}, want: "--namespace=arcvpn", valid: true},
		{args: []string{"restart", "xray"}, want: "restart\narcvpn-xray.service", valid: true},
		{args: []string{"restart", "sshd"}, valid: false},
		{args: []string{"sessions", "unknown"}, valid: false},
		{args: []string{}, input: "7\nyes\n0\n", want: "admin-reset", valid: true},
	}
	for _, tc := range cases {
		_ = os.Remove(record)
		command := exec.Command("bash", append([]string{root}, tc.args...)...)
		command.Env = append(os.Environ(), "PATH="+temp+":"+os.Getenv("PATH"), "ARCVPN_WRAPPER_TEST_LOG="+record)
		command.Stdin = strings.NewReader(tc.input)
		output, err := command.CombinedOutput()
		if (err == nil) != tc.valid {
			t.Fatalf("args=%v: %v %s", tc.args, err, output)
		}
		called, _ := os.ReadFile(record)
		if tc.valid && !strings.Contains(string(called), tc.want) {
			t.Fatalf("dispatch %v: %s", tc.args, called)
		}
		if !tc.valid && len(called) > 0 {
			t.Fatalf("unapproved service command: %s", called)
		}
		if len(tc.args) == 0 && strings.Contains(string(called), "--reset-2fa") {
			t.Fatal("default menu recovery removed MFA")
		}
	}
}
