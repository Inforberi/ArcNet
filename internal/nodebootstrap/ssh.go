// Package nodebootstrap implements a restricted, one-shot SSH bootstrap for
// machines explicitly selected by an administrator. The permanent agent has
// NO SSH or shell execution endpoint.
package nodebootstrap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/nodeprotocol"
)

var userRegex = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
var keyRegex = regexp.MustCompile(`^(ssh-ed25519|ecdsa-sha2-nistp256|ssh-rsa)$`)

// Reject local/private/metadata/control-plane addresses, including IPv4-mapped
// and special-use ranges that global-unicast alone does not protect.
func PublicIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() || !ip.IsGlobalUnicast() {
		return false
	}
	if ip.Is4In6() {
		return false
	}
	if ip.Is4() {
		for _, cidr := range []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4"} {
			p := netip.MustParsePrefix(cidr)
			if p.Contains(ip) {
				return false
			}
		}
	} else {
		for _, cidr := range []string{"fc00::/7", "fe80::/10", "::/128", "::1/128", "2001::/23", "2001:db8::/32", "2002::/16"} {
			if netip.MustParsePrefix(cidr).Contains(ip) {
				return false
			}
		}
	}
	return true
}
func ResolvePublic(ctx context.Context, host string) (string, error) {
	// Parse IP literals before DNS validation; a valid IPv6 address contains
	// colons, which are forbidden only in DNS names (SSH port is separate).
	if addr, e := netip.ParseAddr(host); e == nil {
		if !PublicIP(addr) {
			return "", errors.New("private or special-use SSH address blocked")
		}
		return addr.String(), nil
	}
	if len(host) < 3 || len(host) > 253 || strings.ContainsAny(host, " /\\\x00\r\n\t@#%:") {
		return "", errors.New("invalid SSH host")
	}
	// Go's resolver, not remote ssh, makes the selection; the chosen IP is then
	// passed to ssh by literal address, avoiding a second DNS resolution.
	candidates, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if e != nil || len(candidates) == 0 {
		return "", errors.New("cannot resolve SSH host")
	}
	for _, address := range candidates {
		if !PublicIP(address) {
			return "", errors.New("SSH DNS returned a private or reserved address")
		}
	}
	return candidates[0].String(), nil
}
func ValidateTarget(host, user string, sshPort, vpnPort int) error {
	if !userRegex.MatchString(user) || sshPort < 1 || sshPort > 65535 || vpnPort < 1 || vpnPort > 65535 || sshPort == vpnPort {
		return errors.New("invalid SSH target or conflicting VPN port")
	}
	if strings.TrimSpace(host) == "" {
		return errors.New("SSH host required")
	}
	return nil
}

type HostKey struct {
	Host        string `json:"host"`
	ResolvedIP  string `json:"resolved_ip"`
	Port        int    `json:"port"`
	Type        string `json:"type"`
	Fingerprint string `json:"fingerprint"`
	keyLine     string
}

func Inspect(ctx context.Context, host string, port int) (HostKey, error) {
	if port < 1 || port > 65535 {
		return HostKey{}, errors.New("invalid SSH port")
	}
	c, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()
	ip, e := ResolvePublic(c, host)
	if e != nil {
		return HostKey{}, e
	}
	command := exec.CommandContext(c, "ssh-keyscan", "-T", "5", "-p", strconv.Itoa(port), "-t", "ed25519", ip)
	command.Stderr = io.Discard
	output, e := command.Output()
	if e != nil {
		return HostKey{}, errors.New("SSH server did not expose an Ed25519 host key")
	}
	if len(output) > 32<<10 {
		return HostKey{}, errors.New("SSH host key output too large")
	}
	for _, line := range strings.Split(string(output), "\n") {
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[1] != "ssh-ed25519" || !keyRegex.MatchString(fields[1]) {
			continue
		}
		raw, e := base64.StdEncoding.DecodeString(fields[2])
		if e != nil || len(raw) < 32 || len(raw) > 2048 {
			continue
		}
		sum := sha256.Sum256(raw)
		fingerprint := "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
		return HostKey{Host: host, ResolvedIP: ip, Port: port, Type: fields[1], Fingerprint: fingerprint, keyLine: "arcvpn-node-target " + fields[1] + " " + fields[2] + "\n"}, nil
	}
	return HostKey{}, errors.New("no valid Ed25519 host key in SSH response")
}

type Credentials struct {
	Host        string `json:"host"`
	User        string `json:"user"`
	SSHPort     int    `json:"ssh_port"`
	VPNPort     int    `json:"vpn_port"`
	Fingerprint string `json:"fingerprint"`
	PrivateKey  string `json:"private_key"`
}

func (c Credentials) Validate() error {
	if err := ValidateTarget(c.Host, c.User, c.SSHPort, c.VPNPort); err != nil {
		return err
	}
	if len(c.PrivateKey) < 100 || len(c.PrivateKey) > 24<<10 || !strings.Contains(c.PrivateKey, "PRIVATE KEY-----") {
		return errors.New("SSH private key is required")
	}
	if !strings.HasPrefix(c.Fingerprint, "SHA256:") || len(c.Fingerprint) > 100 {
		return errors.New("confirm SSH host fingerprint")
	}
	return nil
}

// Connection keeps ephemeral private SSH data inside a 0700 temp directory.
// Nothing is stored in SQLite, a command line, env or any application log.
type Connection struct {
	Target     HostKey
	User       string
	Port       int
	VPNPort    int
	keyFile    string
	knownHosts string
	tempDir    string
}

func Connect(ctx context.Context, c Credentials) (*Connection, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	target, err := Inspect(ctx, c.Host, c.SSHPort)
	if err != nil {
		return nil, err
	}
	if target.Fingerprint != c.Fingerprint {
		return nil, errors.New("SSH host key mismatch; installation stopped")
	}
	tempDir, err := os.MkdirTemp("", "arcvpn-ssh-*")
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(tempDir, 0700); err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}
	conn := &Connection{Target: target, User: c.User, Port: c.SSHPort, VPNPort: c.VPNPort, tempDir: tempDir, keyFile: filepath.Join(tempDir, "identity"), knownHosts: filepath.Join(tempDir, "known_hosts")}
	if err = os.WriteFile(conn.keyFile, []byte(c.PrivateKey), 0600); err != nil {
		conn.Close()
		return nil, err
	}
	if err = os.WriteFile(conn.knownHosts, []byte(target.keyLine), 0600); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}
func (c *Connection) Close() {
	if c != nil && c.tempDir != "" {
		_ = os.RemoveAll(c.tempDir)
	}
}
func (c *Connection) sshOptions() []string {
	return []string{"-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + c.knownHosts, "-o", "GlobalKnownHostsFile=/dev/null", "-o", "HostKeyAlias=arcvpn-node-target", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=2", "-i", c.keyFile, "-p", strconv.Itoa(c.Port)}
}
func (c *Connection) exec(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	return c.execWithTimeout(ctx, 55*time.Second, stdin, args...)
}
func (c *Connection) execWithTimeout(ctx context.Context, timeout time.Duration, stdin io.Reader, args ...string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	full := append(c.sshOptions(), "-T", c.User+"@"+c.Target.ResolvedIP)
	// OpenSSH concatenates post-target arguments into a remote shell command.
	// Quote each argument to avoid execution of shell metacharacters in
	// controller URLs or future caller-supplied values.
	full = append(full, remoteCommand(args...))
	command := exec.CommandContext(cctx, "ssh", full...)
	command.Stdin = stdin
	var stdout bytes.Buffer
	command.Stdout = &bounded{buffer: &stdout, limit: 16384}
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return nil, classifySSHError(err, cctx)
	}
	return stdout.Bytes(), nil
}

// classifySSHError exposes only a process status, never SSH stderr or remote
// stdout (both may contain credentials or other sensitive diagnostics).
func classifySSHError(err error, ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("SSH operation timed out; remote state must be reconciled")
	}
	if ctx.Err() != nil {
		return errors.New("SSH operation canceled; remote state may be uncertain")
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if exit.ExitCode() == 255 {
			return errors.New("SSH transport or authentication failure (exit 255)")
		}
		return fmt.Errorf("remote SSH command failed (exit %d)", exit.ExitCode())
	}
	return errors.New("unable to start SSH command")
}

type bounded struct {
	buffer *bytes.Buffer
	limit  int
}

func (b *bounded) Write(p []byte) (int, error) {
	n := len(p)
	remain := b.limit - b.buffer.Len()
	if remain > 0 {
		if len(p) < remain {
			remain = len(p)
		}
		_, _ = b.buffer.Write(p[:remain])
	}
	return n, nil
}

const readOnlyPreflight = `set -eu
printf 'uid=%s\n' "$(id -u)"
if [ "$(id -u)" -ne 0 ]; then
  sudo -n /usr/bin/true >/dev/null 2>&1 || { printf 'sudo=unavailable\n'; exit 20; }
fi
printf 'sudo=available\n'
[ -r /etc/os-release ] || exit 21
. /etc/os-release
printf 'os_id=%s\n' "$ID"
printf 'os_version=%s\n' "$VERSION_ID"
printf 'arch=%s\n' "$(uname -m)"
printf 'systemd=%s\n' "$(test -d /run/systemd/system && echo yes || echo no)"
printf 'cpu_count=%s\n' "$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo unknown)"
printf 'memory_kb=%s\n' "$(awk '/MemTotal:/{print $2;exit}' /proc/meminfo)"
printf 'swap_kb=%s\n' "$(awk '/SwapTotal:/{print $2;exit}' /proc/meminfo)"
printf 'disk_kb=%s\n' "$(df -Pk / | awk 'NR==2{print $4}')"
printf 'xray_installed=%s\n' "$(command -v xray >/dev/null 2>&1 && echo yes || echo no)"
printf 'apt=%s\n' "$(command -v apt-get >/dev/null 2>&1 && echo yes || echo no)"
printf 'node_installed=%s\n' "$(test -e /etc/arcvpn-node/.arcvpn-managed && echo yes || echo no)"
if test -e /opt/arcvpn-node || test -e /etc/arcvpn-node || test -e /var/lib/arcvpn-node || test -e /var/lib/arcvpn-node-root; then
  printf 'managed_paths_present=yes\n'
else
  printf 'managed_paths_present=no\n'
fi
if id arcvpn-node >/dev/null 2>&1 || id arcvpn-xray >/dev/null 2>&1; then
  printf 'managed_accounts_present=yes\n'
else
  printf 'managed_accounts_present=no\n'
fi
if test -e /etc/systemd/system/arcvpn-node.service || test -e /etc/systemd/system/arcvpn-node-helper.service || test -e /etc/systemd/system/arcvpn-node-helper.socket || test -e /etc/systemd/system/arcvpn-node-xray.service; then
  printf 'managed_unit_present=yes\n'
else
  printf 'managed_unit_present=no\n'
fi
printf 'ufw=%s\n' "$(command -v ufw >/dev/null 2>&1 && echo available || echo absent)"
# Read-only status only. Neither host nor cloud firewall can be inferred
# reliably from local rules; active rules require an independent VPS check.
if command -v ufw >/dev/null 2>&1; then
  if [ "$(id -u)" -eq 0 ]; then
    ufw_line=$(ufw status 2>/dev/null | head -n 1) || ufw_line=''
  else
    ufw_line=$(sudo -n ufw status 2>/dev/null | head -n 1) || ufw_line=''
  fi
  case "$ufw_line" in
    'Status: active'*) printf 'ufw_status=active\n' ;;
    'Status: inactive'*) printf 'ufw_status=inactive\n' ;;
    *) printf 'ufw_status=unknown\n' ;;
  esac
else
  printf 'ufw_status=absent\n'
fi
printf 'nft=%s\n' "$(command -v nft >/dev/null 2>&1 && echo available || echo absent)"
if command -v ss >/dev/null 2>&1; then
  if ss -H -lnt | awk -v target=":$1" 'length($4)>=length(target) && substr($4,length($4)-length(target)+1)==target {found=1;exit} END{exit !found}'; then
    printf 'vpn_port_busy=yes\n'
  else
    printf 'vpn_port_busy=no\n'
  fi
else
  printf 'vpn_port_busy=unknown\n'
fi
`

// This probe performs only an HTTPS GET /healthz from the target VPS.
// No certificates are ignored, no secrets are passed, and no redirect follows.
// It is a read-only reachability check, not an external VPN/ingress probe.
const controllerHTTPSPreflight = `set -eu
controller="$1"
if ! command -v curl >/dev/null 2>&1; then
  printf 'controller_https=unverified_no_curl\n'
  exit 0
fi
status=$(curl --proto '=https' --connect-timeout 5 --max-time 9 --silent --output /dev/null --write-out '%{http_code}' "$controller/healthz" 2>/dev/null) || {
  printf 'controller_https=unreachable\n'
  exit 0
}
if [ "$status" = 204 ]; then
  printf 'controller_https=reachable\n'
else
  printf 'controller_https=unexpected_status\n'
fi
`

func validateProbeOrigin(controller string) error {
	_, err := nodeprotocol.ValidateHTTPSOrigin(controller)
	return err
}

// addControllerReachability does not claim that an external VPN handshake
// succeeded. DNS, CA and proxy failures are presented as distinct blockers.
func addControllerReachability(plan *Preflight, result string) {
	plan.Items["controller_https"] = result
	switch result {
	case "reachable":
	case "unverified_no_curl":
		plan.Warnings = append(plan.Warnings, "curl is missing on VPS; outbound HTTPS to the controller could not be checked")
	case "unreachable":
		plan.Blockers = append(plan.Blockers, "VPS cannot establish trusted HTTPS to controller /healthz; check DNS, certificates, egress and reverse proxy")
	case "unexpected_status":
		plan.Blockers = append(plan.Blockers, "Controller /healthz did not return HTTP 204 from VPS")
	default:
		plan.Blockers = append(plan.Blockers, "Controller HTTPS reachability result is invalid")
	}
}

type Preflight struct {
	Host     HostKey           `json:"host_key"`
	Items    map[string]string `json:"items"`
	Warnings []string          `json:"warnings"`
	Blockers []string          `json:"blockers"`
	Plan     []string          `json:"plan"`
}

func (c *Connection) Preflight(ctx context.Context) (Preflight, error) {
	output, err := c.exec(ctx, strings.NewReader(readOnlyPreflight), "sh", "-s", "--", strconv.Itoa(c.VPNPort))
	if err != nil {
		return Preflight{}, err
	}
	return parsePreflight(c.Target, output), nil
}

// PreflightForController adds a remote outbound HTTPS probe, without
// downloading/executing scripts or modifying the machine.
func (c *Connection) PreflightForController(ctx context.Context, controller string) (Preflight, error) {
	if err := validateProbeOrigin(controller); err != nil {
		return Preflight{}, err
	}
	plan, err := c.Preflight(ctx)
	if err != nil {
		return Preflight{}, err
	}
	output, err := c.execWithTimeout(ctx, 20*time.Second, strings.NewReader(controllerHTTPSPreflight), "sh", "-s", "--", controller)
	if err != nil {
		return Preflight{}, errors.New("read-only controller HTTPS probe failed")
	}
	parts := strings.SplitN(strings.TrimSpace(string(output)), "=", 2)
	if len(parts) != 2 || parts[0] != "controller_https" {
		return Preflight{}, errors.New("invalid controller HTTPS probe result")
	}
	addControllerReachability(&plan, parts[1])
	return plan, nil
}

func parsePreflight(target HostKey, output []byte) Preflight {
	info := Preflight{Host: target, Items: map[string]string{}, Warnings: []string{}, Blockers: []string{}, Plan: []string{
		"Verify signed release package and SSH host key before any changes",
		"Create /opt/arcvpn-node/bin, /etc/arcvpn-node, /var/lib/arcvpn-node and /var/lib/arcvpn-node-root; install signed agent and Xray",
		"Create dedicated arcvpn-node and arcvpn-xray accounts; install arcvpn-node.service, arcvpn-node-helper.socket/service and arcvpn-node-xray.service",
		"Register outbound-only HTTPS agent using a one-time secret on stdin",
		"Do not modify existing SSH, firewall or foreign Xray services",
		"Require confirmed agent/Xray state before treating node as configured",
	}}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			info.Items[parts[0]] = parts[1]
		}
	}
	if info.Items["os_id"] != "ubuntu" && info.Items["os_id"] != "debian" {
		info.Blockers = append(info.Blockers, "Supported OS: Debian / Ubuntu only")
	}
	if info.Items["arch"] != "x86_64" {
		info.Blockers = append(info.Blockers, "Only Linux amd64 signed bundles are supported")
	}
	if info.Items["systemd"] != "yes" || info.Items["apt"] != "yes" {
		info.Blockers = append(info.Blockers, "systemd and apt are required")
	}
	if info.Items["sudo"] != "available" {
		info.Blockers = append(info.Blockers, "Passwordless sudo or root access is required")
	}
	if info.Items["ufw_status"] == "active" {
		info.Warnings = append(info.Warnings, "UFW is active: confirm inbound VPN port and SSH are allowed using provider console; rules are NOT changed")
	} else if info.Items["ufw_status"] == "unknown" {
		info.Warnings = append(info.Warnings, "UFW status is unknown: inspect the firewall with VPS console before continuing")
	}
	if info.Items["nft"] == "available" {
		info.Warnings = append(info.Warnings, "nftables is present; its effective policy and provider firewall must be independently checked")
	}
	info.Warnings = append(info.Warnings, "Cloud provider inbound firewall and external VPN handshake cannot be inferred from SSH preflight")
	if info.Items["xray_installed"] == "yes" {
		info.Warnings = append(info.Warnings, "Foreign Xray installation exists; managed service uses separate paths and a new port")
	}
	if info.Items["node_installed"] == "yes" || info.Items["managed_unit_present"] == "yes" || info.Items["managed_paths_present"] == "yes" || info.Items["managed_accounts_present"] == "yes" {
		info.Blockers = append(info.Blockers, "ArcVPN-managed components already present; installer refuses overwrite")
	}
	if info.Items["vpn_port_busy"] == "yes" {
		info.Blockers = append(info.Blockers, "Requested VPN listener port is already in use")
	} else if info.Items["vpn_port_busy"] != "no" {
		info.Blockers = append(info.Blockers, "Cannot verify the VPN listener port is free (ss unavailable)")
	}
	mem, memErr := strconv.ParseInt(info.Items["memory_kb"], 10, 64)
	disk, diskErr := strconv.ParseInt(info.Items["disk_kb"], 10, 64)
	if memErr != nil || mem < 256*1024 {
		info.Blockers = append(info.Blockers, "Less than 256 MiB of total memory or unreadable memory information")
	}
	if diskErr != nil || disk < 512*1024 {
		info.Blockers = append(info.Blockers, "Less than 512 MiB of free root disk or unreadable disk information")
	}
	return info
}

// remoteCommand quotes each argv element before OpenSSH sends it to the
// remote login shell. Empty arguments are preserved and no shell expansion
// takes place. Only the fixed command vocabulary is passed by callers.
func remoteCommand(args ...string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, ShellString(arg))
	}
	return strings.Join(quoted, " ")
}

// ShellString emits a single POSIX-shell literal for trusted fixed scripts.
func ShellString(v string) string                     { return "'" + strings.ReplaceAll(v, "'", "'\\''") + "'" }
func EncodePreflight(value Preflight) ([]byte, error) { return json.Marshal(value) }
func CopyOutput(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, io.LimitReader(src, 128<<20))
	return err
}
func Diagnostic(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("ssh/bootstrap: %v", err)
}
