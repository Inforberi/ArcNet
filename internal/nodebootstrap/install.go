package nodebootstrap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/arcvpn/arcvpn/internal/updater"
)

type Release struct {
	Version string
	Agent   []byte
	Xray    []byte
}

// LoadRelease accepts only an independently Ed25519-signed archive. Digest
// data downloaded from the controller alone is not a trust anchor.
func LoadRelease(archivePath, publicKeyPath string) (Release, error) {
	if archivePath == "" || publicKeyPath == "" {
		return Release{}, errors.New("signed node release and pinned public key must be configured")
	}
	public, err := updater.ReadPublicKey(publicKeyPath)
	if err != nil {
		return Release{}, errors.New("pinned node release public key missing or invalid")
	}
	// One descriptor enforces the same trust boundary as the root updater.
	// A separate Lstat followed by ReadFile admits a symlink-swap race and
	// can allocate arbitrary memory if the staged archive changes meanwhile.
	fd, err := syscall.Open(archivePath, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return Release{}, errors.New("node release archive unavailable")
	}
	f := os.NewFile(uintptr(fd), archivePath)
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() == 0 || stat.Size() > updater.MaxArchive {
		return Release{}, errors.New("node release must be a bounded regular file")
	}
	blob, err := io.ReadAll(io.LimitReader(f, updater.MaxArchive+1))
	if err != nil || len(blob) == 0 || len(blob) > updater.MaxArchive {
		return Release{}, errors.New("node release archive exceeds size limit")
	}
	verified, err := updater.Verify(blob, public)
	if err != nil {
		return Release{}, errors.New("node release signature or components invalid")
	}
	if _, err := versionPatch(verified.Manifest.Version); err != nil {
		return Release{}, err
	}
	xray := verified.Components["xray"]
	if len(xray) == 0 {
		return Release{}, errors.New("node release must include an Xray binary")
	}
	for _, binary := range [][]byte{verified.Binary, xray} {
		file, err := elf.NewFile(bytes.NewReader(binary))
		if err != nil {
			return Release{}, errors.New("node binaries must be Linux ELF")
		}
		if file.Machine != elf.EM_X86_64 {
			return Release{}, errors.New("node release supports Linux amd64 only")
		}
	}
	return Release{Version: verified.Manifest.Version, Agent: verified.Binary, Xray: xray}, nil
}

const installScript = `set -eu
umask 077
stage=__STAGE__
[ -d "$stage" ] && [ -f "$stage/agent" ] && [ -f "$stage/xray" ] || exit 12
[ -d /run/systemd/system ] || exit 13
[ ! -e /etc/arcvpn-node/.arcvpn-managed ] || exit 14
[ ! -e /etc/systemd/system/arcvpn-node.service ] || exit 15
[ ! -e /etc/systemd/system/arcvpn-node-helper.socket ] || exit 16
[ ! -e /etc/systemd/system/arcvpn-node-xray.service ] || exit 17
[ ! -e /opt/arcvpn-node ] || exit 18
[ ! -e /etc/arcvpn-node ] || exit 19
[ ! -e /var/lib/arcvpn-node ] || exit 22
[ ! -e /var/lib/arcvpn-node-root ] || exit 23
[ ! -e /etc/systemd/system/arcvpn-node-helper.service ] || exit 24
[ ! -e /etc/systemd/system/arcvpn-node-xray.service ] || exit 25
# Refuse to re-use unmanaged accounts; no existing resources are overwritten.
! id arcvpn-node >/dev/null 2>&1 || exit 26
! id arcvpn-xray >/dev/null 2>&1 || exit 27
. /etc/os-release
case "$ID" in debian|ubuntu) ;; *) exit 20;; esac
[ "$(uname -m)" = 'x86_64' ] || exit 21
# Compensation is limited to resources we proved were absent above. Apt
# cache/packages are not reverted; this is explicitly reported to operators.
created_node=0
created_xray=0
rollback_install() {
  set +e
  systemctl disable --now arcvpn-node.service arcvpn-node-helper.socket arcvpn-node-xray.service >/dev/null 2>&1
  rm -f /etc/systemd/system/arcvpn-node.service /etc/systemd/system/arcvpn-node-helper.socket /etc/systemd/system/arcvpn-node-helper.service /etc/systemd/system/arcvpn-node-xray.service
  rm -f /opt/arcvpn-node/bin/agent /opt/arcvpn-node/bin/xray /etc/arcvpn-node/.arcvpn-managed /etc/arcvpn-node/release-version
  rmdir /opt/arcvpn-node/bin /opt/arcvpn-node /etc/arcvpn-node /var/lib/arcvpn-node-root /var/lib/arcvpn-node >/dev/null 2>&1
  # Never delete a foreign account if a concurrent process won the race
  # after preflight but before our useradd could complete.
  if [ "$created_node" -eq 1 ]; then userdel arcvpn-node >/dev/null 2>&1; fi
  if [ "$created_xray" -eq 1 ]; then userdel arcvpn-xray >/dev/null 2>&1; fi
  systemctl daemon-reload >/dev/null 2>&1
}
trap rollback_install EXIT
useradd --system --home-dir /var/lib/arcvpn-node --shell /usr/sbin/nologin --user-group arcvpn-node
created_node=1
useradd --system --home-dir /nonexistent --shell /usr/sbin/nologin --user-group arcvpn-xray
created_xray=1
# Dependencies for the signed ArcVPN Go binary (CGO SQLite / QR library).
DEBIAN_FRONTEND=noninteractive apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ca-certificates libsqlite3-0 libqrencode4
install -d -m 0755 /opt/arcvpn-node /opt/arcvpn-node/bin
install -d -o root -g arcvpn-xray -m 0750 /etc/arcvpn-node
install -d -o arcvpn-node -g arcvpn-node -m 0700 /var/lib/arcvpn-node
# The helper receipt is privileged state: the untrusted agent cannot forge it.
install -d -o root -g root -m 0700 /var/lib/arcvpn-node-root
install -m 0755 "$stage/agent" /opt/arcvpn-node/bin/agent
install -m 0755 "$stage/xray" /opt/arcvpn-node/bin/xray
# The SSH user owns /tmp staging. Only execute files after copying into a
# root-owned directory and comparing their final bytes to the signed release.
# Verifying staged files only would leave a time-of-check/time-of-use race.
printf '%s  %s\n' __AGENT_SHA__ /opt/arcvpn-node/bin/agent | sha256sum -c --status || exit 28
printf '%s  %s\n' __XRAY_SHA__ /opt/arcvpn-node/bin/xray | sha256sum -c --status || exit 29
# Own files ONLY: this installer does not touch sshd, UFW, /usr/local/bin/xray
# or existing 3x-ui and Xray services.
cat >/etc/systemd/system/arcvpn-node-helper.socket <<'UNIT'
[Unit]
Description=ArcVPN remote node controlled Xray apply socket
[Socket]
ListenStream=/run/arcvpn-node/helper.sock
SocketMode=0600
SocketUser=arcvpn-node
SocketGroup=arcvpn-node
DirectoryMode=0755
RemoveOnStop=true
[Install]
WantedBy=sockets.target
UNIT
cat >/etc/systemd/system/arcvpn-node-helper.service <<'UNIT'
[Unit]
Description=ArcVPN node minimal root apply helper
Requires=arcvpn-node-helper.socket
[Service]
Type=simple
ExecStart=/opt/arcvpn-node/bin/agent node-helper
User=root
Group=root
NoNewPrivileges=true
ProtectHome=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
MemoryMax=128M
UNIT
cat >/etc/systemd/system/arcvpn-node-xray.service <<'UNIT'
[Unit]
Description=ArcVPN managed remote Xray
Wants=network-online.target
After=network-online.target
ConditionPathExists=/etc/arcvpn-node/xray.json
[Service]
Type=simple
User=arcvpn-xray
Group=arcvpn-xray
ExecStart=/opt/arcvpn-node/bin/xray run -config /etc/arcvpn-node/xray.json
# Xray runs unprivileged, but must be allowed to listen on TCP 443.
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_BIND_SERVICE
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadOnlyPaths=/etc/arcvpn-node
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
UMask=0077
[Install]
WantedBy=multi-user.target
UNIT
cat >/etc/systemd/system/arcvpn-node.service <<'UNIT'
[Unit]
Description=ArcVPN managed remote node agent
Wants=network-online.target
After=network-online.target
[Service]
Type=simple
User=arcvpn-node
Group=arcvpn-node
ExecStart=/opt/arcvpn-node/bin/agent node-agent --config /var/lib/arcvpn-node/node.json
Restart=on-failure
RestartSec=8
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/arcvpn-node
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
UMask=0077
[Install]
WantedBy=multi-user.target
UNIT
printf '%s\n' __OPERATION__ > /etc/arcvpn-node/.arcvpn-managed
chmod 0600 /etc/arcvpn-node/.arcvpn-managed
printf '%s\n' __VERSION__ > /etc/arcvpn-node/release-version
chmod 0600 /etc/arcvpn-node/release-version
systemctl daemon-reload
systemctl enable arcvpn-node-xray.service
systemctl enable --now arcvpn-node-helper.socket
trap - EXIT
printf 'managed_files_installed\n'
`

// InstallVerified stages ONLY files independently verified by LoadRelease.
// Requires a host fingerprint confirmed against a separate trusted source.
func (c *Connection) InstallVerified(ctx context.Context, release Release, operationID string, step func(string)) error {
	if len(operationID) < 16 || len(operationID) > 64 || strings.ContainsAny(operationID, "/\\\x00\n ") {
		return errors.New("invalid bootstrap operation ID")
	}
	stage := "/tmp/arcvpn-stage4-" + operationID
	if step != nil {
		step("stage")
	}
	_, err := c.exec(ctx, strings.NewReader("set -eu\numask 077\n[ ! -e "+ShellString(stage)+" ] || exit 25\nmkdir -m 0700 "+ShellString(stage)+"\n"), "sh", "-s")
	if err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = c.exec(cleanupCtx, strings.NewReader("rm -rf -- "+ShellString(stage)+"\n"), "sh", "-s")
	}()
	for name, data := range map[string][]byte{"agent": release.Agent, "xray": release.Xray} {
		localFile := filepath.Join(c.tempDir, name)
		if err = os.WriteFile(localFile, data, 0700); err != nil {
			return err
		}
		if err = c.copy(ctx, localFile, stage+"/"+name); err != nil {
			return err
		}
	}
	if step != nil {
		step("install")
	}
	body := installBody(stage, operationID, release.Version, release.Agent, release.Xray)
	remote := []string{"sh", "-s"}
	// remote sudo credentials are intentionally neither sent nor stored.
	check, err := c.Preflight(ctx)
	if err != nil {
		return err
	}
	if len(check.Blockers) > 0 {
		return errors.New("preflight blockers prevent installation")
	}
	if check.Items["uid"] != "0" {
		remote = []string{"sudo", "-n", "sh", "-s"}
	}
	_, err = c.execWithTimeout(ctx, 180*time.Second, strings.NewReader(body), remote...)
	if err != nil {
		// If the SSH response was lost after completion, read back the exact
		// root-owned marker, signed bytes and systemd socket on the pinned VPS.
		// A partial install is never considered successful.
		checkCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		verified, verifyErr := c.InstalledReceipt(checkCtx, operationID, release, false)
		if verifyErr == nil && verified {
			return nil
		}
		return errors.New("remote install incomplete or SSH acknowledgment lost; inspect VPS console before retrying")
	}
	return nil
}

// installedReceiptScript observes only fixed ArcVPN-owned paths. The expected
// SHA-256 values come from the already verified release archive, never from
// data downloaded from the VPS. No secrets or private configuration is read.
func installedReceiptScript(installationID string, release Release, running bool) (string, error) {
	if !operationIDPattern.MatchString(installationID) {
		return "", errors.New("invalid installation marker")
	}
	if _, err := versionPatch(release.Version); err != nil {
		return "", err
	}
	if len(release.Agent) == 0 || len(release.Xray) == 0 {
		return "", errors.New("empty signed release")
	}
	body := `set -eu
expected=__OPERATION__
version=__VERSION__
if [ -f /etc/arcvpn-node/.arcvpn-managed ] && [ ! -L /etc/arcvpn-node/.arcvpn-managed ] &&
   [ -f /etc/arcvpn-node/release-version ] && [ ! -L /etc/arcvpn-node/release-version ] &&
   [ "$(cat /etc/arcvpn-node/.arcvpn-managed)" = "$expected" ] &&
   [ "$(cat /etc/arcvpn-node/release-version)" = "$version" ] &&
   [ -f /etc/systemd/system/arcvpn-node.service ] &&
   [ -f /etc/systemd/system/arcvpn-node-helper.socket ] &&
   [ -f /etc/systemd/system/arcvpn-node-xray.service ] &&
   printf '%s  %s\n' __AGENT_SHA__ /opt/arcvpn-node/bin/agent | sha256sum -c --status &&
   printf '%s  %s\n' __XRAY_SHA__ /opt/arcvpn-node/bin/xray | sha256sum -c --status &&
   systemctl is-active --quiet arcvpn-node-helper.socket; then
  printf 'verified_installed\n'
else
  printf 'incomplete\n'
fi
`
	if running {
		body = strings.Replace(body, "  printf 'verified_installed\\n'", `  systemctl is-active --quiet arcvpn-node.service &&
  systemctl is-active --quiet arcvpn-node-xray.service &&
  printf 'verified_installed\n'`, 1)
	}
	body = strings.ReplaceAll(body, "__OPERATION__", ShellString(installationID))
	body = strings.ReplaceAll(body, "__VERSION__", ShellString(release.Version))
	body = strings.ReplaceAll(body, "__AGENT_SHA__", ShellString(fmt.Sprintf("%x", sha256.Sum256(release.Agent))))
	return strings.ReplaceAll(body, "__XRAY_SHA__", ShellString(fmt.Sprintf("%x", sha256.Sum256(release.Xray)))), nil
}

// InstalledReceipt reconciles a lost SSH response without trusting a bare
// marker or a claimed version. It does not prove external VPN connectivity.
func (c *Connection) InstalledReceipt(ctx context.Context, installationID string, release Release, running bool) (bool, error) {
	script, err := installedReceiptScript(installationID, release, running)
	if err != nil {
		return false, err
	}
	out, err := c.privileged(ctx, script)
	if err != nil {
		return false, err
	}
	switch strings.TrimSpace(string(out)) {
	case "verified_installed":
		return true, nil
	case "incomplete":
		return false, nil
	default:
		return false, errors.New("invalid remote installation readback")
	}
}

// OpenSSH scp parses the first colon as the host/path delimiter. IPv6
// literals therefore require brackets even though ssh -T accepts bare IPv6.
func scpDestination(user, ip, path string) string {
	if address, err := netip.ParseAddr(ip); err == nil && address.Is6() {
		return user + "@[" + ip + "]:" + path
	}
	return user + "@" + ip + ":" + path
}

func (c *Connection) copy(ctx context.Context, localFile, remotePath string) error {
	cctx, cancel := context.WithTimeout(ctx, 80*time.Second)
	defer cancel()
	args := []string{"-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + c.knownHosts, "-o", "GlobalKnownHostsFile=/dev/null", "-o", "HostKeyAlias=arcvpn-node-target", "-o", "ConnectTimeout=10", "-i", c.keyFile, "-P", fmt.Sprint(c.Port), localFile, scpDestination(c.User, c.Target.ResolvedIP, remotePath)}
	command := exec.CommandContext(cctx, "scp", args...)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return fmt.Errorf("SCP stage upload failed: %w", classifySSHError(err, cctx))
	}
	return nil
}
func (c *Connection) Register(ctx context.Context, controller, token string) error {
	// Do not pass the enrollment credential in the ssh command line or remote
	// shell. SSH encrypts its stdin; node-register reads from fd 0 only.
	preflight, err := c.Preflight(ctx)
	if err != nil {
		return err
	}
	remote := []string{"/opt/arcvpn-node/bin/agent", "node-register", "--controller", controller, "--token-stdin", "--config", "/var/lib/arcvpn-node/node.json"}
	if preflight.Items["uid"] == "0" {
		remote = append([]string{"runuser", "-u", "arcvpn-node", "--"}, remote...)
	} else {
		remote = append([]string{"sudo", "-n", "-u", "arcvpn-node", "--"}, remote...)
	}
	_, err = c.exec(ctx, strings.NewReader(token+"\n"), remote...)
	if err != nil {
		return errors.New("node enrollment failed; release a new one-time token")
	}
	serviceScript := "set -eu\nsystemctl enable --now arcvpn-node.service\n"
	if preflight.Items["uid"] == "0" {
		_, err = c.exec(ctx, strings.NewReader(serviceScript), "sh", "-s")
	} else {
		_, err = c.exec(ctx, strings.NewReader(serviceScript), "sudo", "-n", "sh", "-s")
	}
	return err
}

func installBody(stage, operationID, version string, agent, xray []byte) string {
	body := strings.ReplaceAll(installScript, "__STAGE__", ShellString(stage))
	body = strings.ReplaceAll(body, "__OPERATION__", ShellString(operationID))
	body = strings.ReplaceAll(body, "__VERSION__", ShellString(version))
	body = strings.ReplaceAll(body, "__AGENT_SHA__", ShellString(fmt.Sprintf("%x", sha256.Sum256(agent))))
	return strings.ReplaceAll(body, "__XRAY_SHA__", ShellString(fmt.Sprintf("%x", sha256.Sum256(xray))))
}
