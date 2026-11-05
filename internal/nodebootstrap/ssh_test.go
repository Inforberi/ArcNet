package nodebootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"github.com/arcvpn/arcvpn/internal/updater"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSCPDestinationWithPublicIP(t *testing.T) {
	if actual := scpDestination("root", "9.9.9.9", "/tmp/arcvpn-release"); actual != "root@9.9.9.9:/tmp/arcvpn-release" {
		t.Fatalf("unexpected IPv4 scp target: %q", actual)
	}
	if actual := scpDestination("root", "2606:4700:4700::1111", "/tmp/arcvpn-release"); actual != "root@[2606:4700:4700::1111]:/tmp/arcvpn-release" {
		t.Fatalf("IPv6 scp target must be bracketed: %q", actual)
	}
}

func TestResolvePublicIPv6Literal(t *testing.T) {
	resolved, err := ResolvePublic(context.Background(), "2606:4700:4700::1111")
	if err != nil || resolved != "2606:4700:4700::1111" {
		t.Fatalf("public IPv6 literal must be supported without DNS: %q %v", resolved, err)
	}
	for _, host := range []string{"::1", "fe80::1", "2001:db8::1", "::ffff:8.8.8.8", "fe80::1%eth0", "2001:4860::1;whoami"} {
		if _, err := ResolvePublic(context.Background(), host); err == nil {
			t.Errorf("unsafe IPv6 target accepted: %q", host)
		}
	}
}

func TestPublicIPPolicy(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "100.64.0.2", "192.0.2.4", "198.51.100.42", "203.0.113.8", "::1", "fc00::1", "2001:db8::5", "::ffff:8.8.8.8"} {
		if PublicIP(netip.MustParseAddr(raw)) {
			t.Errorf("accepted private or reserved %q", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "9.9.9.9", "1.1.1.1", "2606:4700:4700::1111"} {
		if !PublicIP(netip.MustParseAddr(raw)) {
			t.Errorf("rejected routable %q", raw)
		}
	}
}
func TestSSHBootstrapValidation(t *testing.T) {
	for _, tc := range []struct {
		host, user string
		ssh, vpn   int
	}{
		{"127.0.0.1", "root", 22, 443}, {"8.8.8.8", "root", 443, 443}, {"8.8.8.8", "root;rm", 22, 443}, {"8.8.8.8", "root", 0, 443},
	} {
		if err := ValidateTarget(tc.host, tc.user, tc.ssh, tc.vpn); err == nil && tc.host != "127.0.0.1" {
			t.Errorf("accepted invalid target: %+v", tc)
		}
	}
	if _, err := ResolvePublic(context.Background(), "169.254.169.254"); err == nil {
		t.Fatal("metadata destination accepted")
	}
	if _, err := ResolvePublic(context.Background(), "192.0.2.10"); err == nil {
		t.Fatal("documentation IP accepted")
	}
	if strings.Contains(installScript, "--token ") || strings.Contains(installScript, "curl |") {
		t.Fatal("legacy insecure bootstrap leaked into stage 4")
	}
}
func TestSignedReleaseFailClosed(t *testing.T) {
	if _, err := LoadRelease("", ""); err == nil {
		t.Fatal("release accepted without signature")
	}
}

func TestRemoteShellCommandDoesNotExpandUntrustedArguments(t *testing.T) {
	// Regression: OpenSSH executes the arguments after the target through a
	// remote login shell. They must each be quoted before passing them to ssh.
	attack := "https://vpn.example/'; echo SHOULD_NOT_EXECUTE >&2; #"
	output, err := exec.Command("sh", "-c", remoteCommand("printf", "%s", attack)).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != attack {
		t.Fatalf("remote argument was modified or expanded: %q", output)
	}
}

func TestInstallerStrictOwnershipAndRollbackScript(t *testing.T) {
	for _, want := range []string{
		"/var/lib/arcvpn-node-root",
		"CapabilityBoundingSet=CAP_NET_BIND_SERVICE",
		"AmbientCapabilities=CAP_NET_BIND_SERVICE",
		"trap rollback_install EXIT",
		"trap - EXIT",
		"[ ! -e /etc/arcvpn-node ]",
		"[ ! -e /opt/arcvpn-node ]",
		"! id arcvpn-node",
	} {
		if !strings.Contains(installScript, want) {
			t.Fatalf("missing installer safety invariant: %s", want)
		}
	}
	// Only parse syntax; never execute the privileged installation on a test host.
	cmd := exec.Command("sh", "-n")
	cmd.Stdin = strings.NewReader(installScript)
	if err := cmd.Run(); err != nil {
		t.Fatalf("remote installation script is not valid POSIX sh: %v", err)
	}
}

func TestSignedNodeReleaseRoundTripAndTamperResistance(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Stage 4 signed release currently supports Linux amd64")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := updater.MakePackageWithComponents(binary, "0.6.0-stage4", private, map[string][]byte{"xray": binary})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "node-signed.zip")
	publicPath := filepath.Join(dir, "node-public.hex")
	if err := os.WriteFile(archivePath, bundle, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publicPath, []byte(hex.EncodeToString(public)), 0600); err != nil {
		t.Fatal(err)
	}
	verified, err := LoadRelease(archivePath, publicPath)
	if err != nil || verified.Version != "0.6.0-stage4" || len(verified.Agent) != len(binary) || len(verified.Xray) != len(binary) {
		t.Fatalf("signed agent/Xray bundle rejected: %#v %v", verified.Version, err)
	}
	otherPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publicPath, []byte(hex.EncodeToString(otherPublic)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRelease(archivePath, publicPath); err == nil {
		t.Fatal("bundle accepted a different trust anchor")
	}
}

func TestReadOnlyPreflightShellSyntax(t *testing.T) {
	cmd := exec.Command("sh", "-n")
	cmd.Stdin = strings.NewReader(readOnlyPreflight)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("remote preflight shell is invalid: %v: %s", err, output)
	}
	for _, needle := range []string{"managed_paths_present", "managed_accounts_present", "managed_unit_present", "vpn_port_busy"} {
		if !strings.Contains(readOnlyPreflight, needle) {
			t.Fatalf("preflight does not check %q", needle)
		}
	}
}

func TestRemovalScriptRequiresOriginalInstallID(t *testing.T) {
	valid := "061b8595-c790-4a4a-83ad-8aee0ef4c305"
	for _, mode := range []UninstallMode{UninstallDetach, UninstallPurge} {
		script, err := RemovalScript(mode, valid)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(script, "__OPERATION__") || strings.Contains(script, "__MODE__") {
			t.Fatal("unresolved replacement")
		}
		cmd := exec.Command("sh", "-n")
		cmd.Stdin = strings.NewReader(script)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("invalid removal shell: %v, %s", err, output)
		}
		if !strings.Contains(script, "[ \"$installed\" = \"$expected\" ]") {
			t.Fatal("removal does not verify managed marker")
		}
		if strings.Contains(script, "rm -rf") || strings.Contains(script, "/etc/ssh") || strings.Contains(script, "ufw ") {
			t.Fatal("destructive script touches unowned resources")
		}
	}
	for _, input := range []struct {
		mode UninstallMode
		id   string
	}{
		{UninstallPurge, "../../etc"}, {UninstallMode("shell"), valid}, {UninstallDetach, ""}, {UninstallPurge, "';rm -rf /;#"},
	} {
		if _, err := RemovalScript(input.mode, input.id); err == nil {
			t.Fatalf("accepted unsafe removal request: %+v", input)
		}
	}
	if !strings.Contains(removalScript, `if [ "$mode" = detach ]`) || !strings.Contains(removalScript, "exit 0") {
		t.Fatal("detach not isolated from purge branch")
	}
}

func TestInstallationMarkerBindsSignedAssetsToOperation(t *testing.T) {
	id := "af1b8595-c790-4a4a-83ad-8aee0ef4c305"
	script := installBody("/tmp/arcvpn-stage4-test", id, "0.6.0-stage4", []byte("agent-test"), []byte("xray-test"))
	if strings.Contains(script, "__OPERATION__") || strings.Contains(script, "__STAGE__") || strings.Contains(script, "__VERSION__") {
		t.Fatal("unbound installer marker")
	}
	if !strings.Contains(script, "printf '%s\\n' '"+id+"' > /etc/arcvpn-node/.arcvpn-managed") {
		t.Fatal("installation identity not persisted")
	}
	cmd := exec.Command("sh", "-n")
	cmd.Stdin = strings.NewReader(script)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("invalid install shell: %v %s", err, output)
	}
}

func TestSignedUpgradeVersionAndScript(t *testing.T) {
	for _, tc := range []struct {
		installed, candidate string
		success              bool
	}{
		{"0.6.0-stage4", "0.6.1-stage4", true},
		{"0.6.8", "0.6.9", true},
		{"0.6.9", "0.6.8", false},
		{"0.6.0-stage4", "0.6.0", false},
		{"0.6.5", "0.7.0", false},
		{"0.6.5", "bad;id", false},
	} {
		err := UpgradeAvailable(tc.installed, tc.candidate)
		if (err == nil) != tc.success {
			t.Fatalf("unexpected version compatibility %+v: %v", tc, err)
		}
	}
	id := "af1b8595-c790-4a4a-83ad-8aee0ef4c305"
	body, err := upgradeBody("/tmp/arcvpn-upgrade-test", id, "0.6.1-stage4", []byte("agent-test"), []byte("xray-test"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-n")
	cmd.Stdin = strings.NewReader(body)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("invalid upgrade script: %v %s", err, out)
	}
	for _, want := range []string{"rollback_upgrade", "run -test -config", "systemctl restart arcvpn-node-xray.service", "signed_upgrade_applied"} {
		if !strings.Contains(body, want) {
			t.Fatalf("upgrade missing %q", want)
		}
	}
	if strings.Contains(body, "/etc/ssh/") || strings.Contains(body, "ufw ") {
		t.Fatal("signed upgrade modifies foreign resources")
	}
}
