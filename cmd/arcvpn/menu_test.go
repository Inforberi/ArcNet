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
	source = []byte(strings.ReplaceAll(string(source), "/opt/arcvpn/bin/arcvpn-core", filepath.Join(temp, "arcvpn-core")))
	root = filepath.Join(temp, "arcvpn-menu.sh")
	if err = os.WriteFile(root, source, 0755); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(temp, "calls")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$0\" \"$@\" >> \"$ARCVPN_WRAPPER_TEST_LOG\"\ncase \"$0\" in *systemctl) printf '0\\n';; *arcvpn-core) if [ \"$1\" = help ]; then printf 'local management protocol: lifecycle-v1\\n'; fi;; esac\n"
	for _, name := range []string{"runuser", "systemctl", "journalctl", "arcvpn-core"} {
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
		{args: []string{"logs", "panel"}, want: "lifecycle\nlogs\npanel", valid: true},
		{args: []string{"restart", "xray"}, want: "lifecycle\nrestart\nxray", valid: true},
		{args: []string{"start"}, want: "lifecycle\nstart", valid: true},
		{args: []string{"stop"}, want: "lifecycle\nstop", valid: true},
		{args: []string{"restart"}, want: "lifecycle\nrestart", valid: true},
		{args: []string{"status", "--json"}, want: "lifecycle\nstatus\n--json", valid: true},
		{args: []string{"logs"}, want: "lifecycle\nlogs", valid: true},
		{args: []string{"start", "sshd"}, valid: false},
		{args: []string{"status", "--root=/tmp"}, valid: false},
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

func TestManagementWrapperRejectsOldCoreBeforePrivilegedCommand(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root wrapper fixture")
	}
	dir := t.TempDir()
	record := filepath.Join(dir, "calls")
	binary := filepath.Join(dir, "old-core")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$ARCVPN_WRAPPER_TEST_LOG\"\nif [ \"$1\" = help ];then printf 'arcvpn: serve | doctor | version\\n';exit 0;fi\nexit 99\n"
	if err := os.WriteFile(binary, []byte(stub), 0700); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../deploy/arcvpn-menu.sh")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "wrapper")
	source = []byte(strings.ReplaceAll(string(source), "/opt/arcvpn/bin/arcvpn-core", binary))
	if err := os.WriteFile(path, source, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", path, "stop")
	cmd.Env = append(os.Environ(), "ARCVPN_WRAPPER_TEST_LOG="+record)
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "lifecycle-v1") {
		t.Fatal(err, string(out))
	}
	calls, _ := os.ReadFile(record)
	if string(calls) != "help\n" {
		t.Fatal("old core received unsafe root command", string(calls))
	}
}
