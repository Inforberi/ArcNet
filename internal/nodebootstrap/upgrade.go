package nodebootstrap

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The signed candidate is staged over the pinned SSH channel. The script
// verifies the exact original root-owned installation marker and validates
// Xray before changing any active binary. Failed restarts restore both
// previous binaries and re-activate the old systemd units.
const upgradeScript = `set -eu
umask 077
[ "$(id -u)" -eq 0 ] || exit 41
expected=__OPERATION__
version=__VERSION__
stage=__STAGE__
marker=/etc/arcvpn-node/.arcvpn-managed
installed=/etc/arcvpn-node/release-version
[ -f "$marker" ] && [ ! -L "$marker" ] && [ "$(cat "$marker")" = "$expected" ] || exit 42
[ -f "$installed" ] && [ ! -L "$installed" ] || exit 43
[ -f /etc/arcvpn-node/xray.json ] && [ ! -L /etc/arcvpn-node/xray.json ] || exit 44
[ -f "$stage/agent" ] && [ -f "$stage/xray" ] && [ ! -L "$stage/agent" ] && [ ! -L "$stage/xray" ] || exit 45
[ -f /opt/arcvpn-node/bin/agent ] && [ ! -L /opt/arcvpn-node/bin/agent ] || exit 46
[ -f /opt/arcvpn-node/bin/xray ] && [ ! -L /opt/arcvpn-node/bin/xray ] || exit 47
[ ! -e /opt/arcvpn-node/bin/.agent-next ] && [ ! -e /opt/arcvpn-node/bin/.xray-next ] || exit 48
systemctl is-active --quiet arcvpn-node-xray.service || exit 49
systemctl is-active --quiet arcvpn-node.service || exit 50
# Only ArcVPN's own service files are touched. SSH, UFW and foreign Xray
# instances remain unchanged.
backup=$(mktemp -d /var/lib/arcvpn-node-root/.upgrade-XXXXXXXX)
chmod 0700 "$backup"
cp -p /opt/arcvpn-node/bin/agent "$backup/agent"
cp -p /opt/arcvpn-node/bin/xray "$backup/xray"
cp -p "$installed" "$backup/version"
# A corrupt or unsigned staged binary must not restart the existing VPN.
# Restore services only after the disruptive part of upgrade began.
switch_started=0
rollback_upgrade() {
  set +e
  if [ "$switch_started" -eq 1 ]; then
    systemctl stop arcvpn-node.service arcvpn-node-helper.socket arcvpn-node-helper.service >/dev/null 2>&1
    install -m 0755 "$backup/agent" /opt/arcvpn-node/bin/agent
    install -m 0755 "$backup/xray" /opt/arcvpn-node/bin/xray
    install -m 0600 "$backup/version" "$installed"
    systemctl restart arcvpn-node-xray.service >/dev/null 2>&1
    systemctl enable --now arcvpn-node-helper.socket arcvpn-node.service >/dev/null 2>&1
  fi
  rm -f /opt/arcvpn-node/bin/.agent-next /opt/arcvpn-node/bin/.xray-next
  rm -f "$backup/agent" "$backup/xray" "$backup/version"
  rmdir "$backup" >/dev/null 2>&1
}
trap rollback_upgrade EXIT
install -m 0755 "$stage/agent" /opt/arcvpn-node/bin/.agent-next
install -m 0755 "$stage/xray" /opt/arcvpn-node/bin/.xray-next
# Do not execute a staged Xray before verifying bytes at the root-owned path.
printf '%s  %s\n' __AGENT_SHA__ /opt/arcvpn-node/bin/.agent-next | sha256sum -c --status || exit 51
printf '%s  %s\n' __XRAY_SHA__ /opt/arcvpn-node/bin/.xray-next | sha256sum -c --status || exit 52
/opt/arcvpn-node/bin/.xray-next run -test -config /etc/arcvpn-node/xray.json >/dev/null 2>&1
switch_started=1
systemctl stop arcvpn-node.service arcvpn-node-helper.socket arcvpn-node-helper.service
mv -f /opt/arcvpn-node/bin/.agent-next /opt/arcvpn-node/bin/agent
mv -f /opt/arcvpn-node/bin/.xray-next /opt/arcvpn-node/bin/xray
systemctl restart arcvpn-node-xray.service
systemctl is-active --quiet arcvpn-node-xray.service
systemctl enable --now arcvpn-node-helper.socket arcvpn-node.service
systemctl is-active --quiet arcvpn-node.service
systemctl is-active --quiet arcvpn-node-helper.socket
printf '%s\n' "$version" > "$installed"
chmod 0600 "$installed"
trap - EXIT
rm -f "$backup/agent" "$backup/xray" "$backup/version"
rmdir "$backup"
printf 'signed_upgrade_applied\n'
`

func upgradeBody(stage, installationID, version string, agent, xray []byte) (string, error) {
	if !operationIDPattern.MatchString(installationID) {
		return "", errors.New("invalid installation marker")
	}
	if _, err := versionPatch(version); err != nil {
		return "", err
	}
	body := strings.ReplaceAll(upgradeScript, "__STAGE__", ShellString(stage))
	body = strings.ReplaceAll(body, "__OPERATION__", ShellString(installationID))
	body = strings.ReplaceAll(body, "__VERSION__", ShellString(version))
	body = strings.ReplaceAll(body, "__AGENT_SHA__", ShellString(fmt.Sprintf("%x", sha256.Sum256(agent))))
	return strings.ReplaceAll(body, "__XRAY_SHA__", ShellString(fmt.Sprintf("%x", sha256.Sum256(xray)))), nil
}

func (c *Connection) ReadVersion(ctx context.Context) (string, error) {
	out, err := c.privileged(ctx, `set -eu
file=/etc/arcvpn-node/release-version
[ -f "$file" ] && [ ! -L "$file" ] || exit 45
cat "$file"
`)
	if err != nil {
		return "", errors.New("cannot read root-owned ArcVPN release version")
	}
	version := strings.TrimSpace(string(out))
	if _, err := versionPatch(version); err != nil {
		return "", err
	}
	return version, nil
}

// UpgradeVerified never auto-upgrades: an authenticated administrator must
// supply a new independently signed bundle and reconfirm the SSH host key.
func (c *Connection) UpgradeVerified(ctx context.Context, release Release, installationID, upgradeID string) error {
	if !operationIDPattern.MatchString(upgradeID) || !operationIDPattern.MatchString(installationID) {
		return errors.New("invalid upgrade identifiers")
	}
	marker, err := c.ReadManaged(ctx)
	if err != nil {
		return err
	}
	if marker != installationID {
		return errors.New("cannot upgrade detached or foreign installation")
	}
	current, err := c.ReadVersion(ctx)
	if err != nil {
		return err
	}
	if err = UpgradeAvailable(current, release.Version); err != nil {
		return err
	}
	if len(release.Agent) == 0 || len(release.Xray) == 0 {
		return errors.New("signed upgrade binary missing")
	}
	stage := "/tmp/arcvpn-upgrade-" + upgradeID
	if _, err = c.exec(ctx, strings.NewReader("set -eu\numask 077\n[ ! -e "+ShellString(stage)+" ] || exit 25\nmkdir -m 0700 "+ShellString(stage)+"\n"), "sh", "-s"); err != nil {
		return err
	}
	defer func() {
		cleanCtx, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		_, _ = c.exec(cleanCtx, strings.NewReader("rm -rf -- "+ShellString(stage)+"\n"), "sh", "-s")
	}()
	for _, item := range []struct {
		name string
		data []byte
	}{{"agent", release.Agent}, {"xray", release.Xray}} {
		path := filepath.Join(c.tempDir, "upgrade-"+item.name)
		if err = os.WriteFile(path, item.data, 0600); err != nil {
			return err
		}
		if err = c.copy(ctx, path, stage+"/"+item.name); err != nil {
			return err
		}
	}
	body, err := upgradeBody(stage, installationID, release.Version, release.Agent, release.Xray)
	if err != nil {
		return err
	}
	uid, err := c.exec(ctx, nil, "id", "-u")
	if err != nil {
		return err
	}
	command := []string{"sh", "-s"}
	if strings.TrimSpace(string(uid)) != "0" {
		command = []string{"sudo", "-n", "sh", "-s"}
	}
	out, err := c.execWithTimeout(ctx, 150*time.Second, strings.NewReader(body), command...)
	if err != nil {
		// Only a complete, running installation with byte-for-byte signed
		// binaries can reconcile an ambiguous SSH failure as success.
		checkCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if verified, checkErr := c.InstalledReceipt(checkCtx, installationID, release, true); checkErr == nil && verified {
			return nil
		}
		return errors.New("signed upgrade incomplete or SSH receipt lost; verify Xray and rollback on VPS")
	}
	if strings.TrimSpace(string(out)) != "signed_upgrade_applied" {
		return errors.New("unexpected signed upgrade receipt")
	}
	return nil
}
