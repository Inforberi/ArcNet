package nodebootstrap

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignedReleaseSHA256CheckedAfterRootOwnedCopy(t *testing.T) {
	agent, xray := []byte("signed-agent"), []byte("signed-xray")
	id := "af1b8595-c790-4a4a-83ad-8aee0ef4c305"
	install := installBody("/tmp/arcvpn-stage4-signed", id, "0.6.1-stage4", agent, xray)
	upgrade, err := upgradeBody("/tmp/arcvpn-stage4-signed", id, "0.6.2-stage4", agent, xray)
	if err != nil {
		t.Fatal(err)
	}
	for name, script := range map[string]string{"install": install, "upgrade": upgrade} {
		for _, payload := range [][]byte{agent, xray} {
			digest := fmt.Sprintf("%x", sha256.Sum256(payload))
			if !strings.Contains(script, "'"+digest+"'") {
				t.Fatalf("%s lost signed SHA-256 for payload", name)
			}
		}
		if strings.Contains(script, "__AGENT_SHA__") || strings.Contains(script, "__XRAY_SHA__") {
			t.Fatal("unexpanded digest placeholders")
		}
		if strings.Index(script, "sha256sum -c --status") < strings.Index(script, "install -m 0755 \"$stage/xray\"") {
			t.Fatalf("%s verified untrusted staged path before root copy", name)
		}
		cmd := exec.Command("sh", "-n")
		cmd.Stdin = strings.NewReader(script)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s invalid shell %s: %v", name, output, err)
		}
	}
}

func TestHTTPSControllerPreflightFailClosed(t *testing.T) {
	if err := validateProbeOrigin("http://localhost"); err == nil {
		t.Fatal("insecure controller accepted")
	}
	if err := validateProbeOrigin("https://example.com/?x=1"); err == nil {
		t.Fatal("query string accepted")
	}
	if err := validateProbeOrigin("https://user@example.com"); err == nil {
		t.Fatal("credentials accepted")
	}
	if err := validateProbeOrigin("https://example.com"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-n")
	cmd.Stdin = strings.NewReader(controllerHTTPSPreflight)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("invalid HTTPS shell: %v %s", err, output)
	}
	if strings.Contains(controllerHTTPSPreflight, "--insecure") || strings.Contains(controllerHTTPSPreflight, "-k ") || strings.Contains(controllerHTTPSPreflight, "--location") {
		t.Fatal("HTTPS probe bypassed trust or followed redirects")
	}
	for _, tc := range []struct {
		result          string
		blocked, warned bool
	}{
		{"reachable", false, false}, {"unverified_no_curl", false, true}, {"unreachable", true, false}, {"unexpected_status", true, false}, {"bad", true, false},
	} {
		p := Preflight{Items: map[string]string{}}
		addControllerReachability(&p, tc.result)
		if (len(p.Blockers) > 0) != tc.blocked || (len(p.Warnings) > 0) != tc.warned || p.Items["controller_https"] != tc.result {
			t.Fatalf("probe %q: %+v", tc.result, p)
		}
	}
}

func TestHTTPSProbeShellDoesNotLeakOrMutate(t *testing.T) {
	temp := t.TempDir()
	script := filepath.Join(temp, "curl")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 204"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-s", "--", "https://example.com")
	cmd.Stdin = strings.NewReader(controllerHTTPSPreflight)
	cmd.Env = append(os.Environ(), "PATH="+temp)
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != "controller_https=reachable\n" {
		t.Fatalf("healthy probe: %q %v", out, err)
	}
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 7"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("sh", "-s", "--", "https://example.com")
	cmd.Stdin = strings.NewReader(controllerHTTPSPreflight)
	cmd.Env = append(os.Environ(), "PATH="+temp)
	out, err = cmd.CombinedOutput()
	if err != nil || string(out) != "controller_https=unreachable\n" {
		t.Fatalf("failed probe: %q %v", out, err)
	}
}

func TestReadbackScriptsRejectAmbiguousSSHCompletions(t *testing.T) {
	const installID = "af1b8595-c790-4a4a-83ad-8aee0ef4c305"
	release := Release{Version: "0.6.1-stage4", Agent: []byte("verified-agent"), Xray: []byte("verified-xray")}
	installed, err := installedReceiptScript(installID, release, false)
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := installedReceiptScript(installID, release, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(installed, "systemctl is-active --quiet arcvpn-node.service") {
		t.Fatal("unregistered install wrongly required agent to be active")
	}
	if !strings.Contains(upgraded, "systemctl is-active --quiet arcvpn-node.service") || !strings.Contains(upgraded, "systemctl is-active --quiet arcvpn-node-xray.service") {
		t.Fatal("upgrade receipt accepts inactive agent or Xray")
	}
	for _, script := range []string{installed, upgraded} {
		if strings.Contains(script, "__") || !strings.Contains(script, "sha256sum -c --status") {
			t.Fatal("receipt does not bind signed identity")
		}
		check := exec.Command("sh", "-n")
		check.Stdin = strings.NewReader(script)
		if out, err := check.CombinedOutput(); err != nil {
			t.Fatalf("invalid receipt shell: %v %q", err, out)
		}
	}
	for _, mode := range []UninstallMode{UninstallDetach, UninstallPurge} {
		script, err := removalReceiptScript(mode, installID)
		if err != nil {
			t.Fatal(err)
		}
		check := exec.Command("sh", "-n")
		check.Stdin = strings.NewReader(script)
		if out, err := check.CombinedOutput(); err != nil {
			t.Fatalf("invalid removal readback: %v %q", err, out)
		}
		if strings.Contains(script, "rm -") || strings.Contains(script, "ufw ") || strings.Contains(script, "systemctl stop") {
			t.Fatal("readback mutated remote host")
		}
		if mode == UninstallDetach && !strings.Contains(script, "systemctl is-active --quiet arcvpn-node-xray.service") {
			t.Fatal("detach accepts stopped VPN")
		}
		if mode == UninstallPurge && !strings.Contains(script, "! id arcvpn-xray") {
			t.Fatal("purge accepts partial account cleanup")
		}
	}
}

func TestSSHFailuresReportOnlyRedactedExitStatus(t *testing.T) {
	secret := "DO_NOT_LOG_PRIVATE_KEY_CONTENT"
	cmd := exec.Command("sh", "-c", "printf '%s\\n' '"+secret+"' >&2; exit 23")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected exit error")
	}
	result := classifySSHError(err, context.Background()).Error()
	if !strings.Contains(result, "exit 23") || strings.Contains(result, secret) {
		t.Fatalf("SSH error classification unsafe: %s", result)
	}
	deadline, cancel := context.WithCancel(context.Background())
	cancel()
	result = classifySSHError(err, deadline).Error()
	if strings.Contains(result, secret) || !strings.Contains(result, "canceled") {
		t.Fatalf("cancel classification unsafe: %s", result)
	}
}

func TestBadSignedUpgradeNeverStopsExistingVPN(t *testing.T) {
	id := "af1b8595-c790-4a4a-83ad-8aee0ef4c305"
	script, err := upgradeBody("/tmp/arcvpn-upgrade-integrity", id, "0.6.2-stage4", []byte("signed-agent"), []byte("signed-xray"))
	if err != nil {
		t.Fatal(err)
	}
	verified := strings.Index(script, "sha256sum -c --status")
	tested := strings.Index(script, ".xray-next run -test")
	switching := strings.LastIndex(script, "switch_started=1")
	stop := strings.LastIndex(script, "systemctl stop arcvpn-node.service")
	if verified < 0 || tested < verified || switching < tested || stop < switching {
		t.Fatalf("unsafe upgrade ordering: check=%d validation=%d switch=%d stop=%d", verified, tested, switching, stop)
	}
	if !strings.Contains(script, `if [ "$switch_started" -eq 1 ]; then`) {
		t.Fatal("validation failure needlessly restarts existing VPN")
	}
}
