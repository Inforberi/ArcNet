package nodebootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var operationIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// UninstallMode is deliberately closed: detach preserves the existing Xray
// process and configuration; purge stops and removes only ArcVPN-owned assets.
type UninstallMode string

const (
	UninstallDetach UninstallMode = "detach"
	UninstallPurge  UninstallMode = "purge"
)

func validRemoval(mode UninstallMode, operationID string) error {
	if mode != UninstallDetach && mode != UninstallPurge {
		return errors.New("invalid removal mode")
	}
	if !operationIDPattern.MatchString(operationID) {
		return errors.New("invalid managed installation identifier")
	}
	return nil
}

// The script has no parameterized filesystem paths. It first compares the
// root-owned installation marker to the original persistent bootstrap ID.
// Never delete arbitrary directories or unrelated Xray/SSH/firewall resources.
const removalScript = `set -eu
umask 077
[ "$(id -u)" -eq 0 ] || exit 41
expected=__OPERATION__
mode=__MODE__
marker=/etc/arcvpn-node/.arcvpn-managed
[ -f "$marker" ] && [ ! -L "$marker" ] || exit 42
installed=$(cat "$marker")
[ "$installed" = "$expected" ] || [ "$installed" = "detached:$expected" ] || exit 43
# A detach is intentionally one-way: no controller can re-activate it without
# a new explicitly approved registration.
if [ "$mode" = detach ]; then
  [ "$installed" = "$expected" ] || exit 44
  systemctl stop arcvpn-node.service arcvpn-node-helper.socket arcvpn-node-helper.service >/dev/null 2>&1 || true
  systemctl disable arcvpn-node.service arcvpn-node-helper.socket >/dev/null 2>&1 || true
  rm -f /etc/systemd/system/arcvpn-node.service /etc/systemd/system/arcvpn-node-helper.service /etc/systemd/system/arcvpn-node-helper.socket
  rm -f /var/lib/arcvpn-node/node.json /opt/arcvpn-node/bin/agent /var/lib/arcvpn-node-root/last-apply.json
  printf 'detached:%s\n' "$expected" > "$marker"
  chmod 0600 "$marker"
  systemctl daemon-reload
  printf 'detached_vpn_remains_active\n'
  exit 0
fi
[ "$mode" = purge ] || exit 45
systemctl stop arcvpn-node.service arcvpn-node-helper.socket arcvpn-node-helper.service arcvpn-node-xray.service >/dev/null 2>&1 || true
systemctl disable arcvpn-node.service arcvpn-node-helper.socket arcvpn-node-xray.service >/dev/null 2>&1 || true
rm -f /etc/systemd/system/arcvpn-node.service /etc/systemd/system/arcvpn-node-helper.socket /etc/systemd/system/arcvpn-node-helper.service /etc/systemd/system/arcvpn-node-xray.service
rm -f /var/lib/arcvpn-node/node.json /var/lib/arcvpn-node-root/last-apply.json
rm -f /etc/arcvpn-node/xray.json /etc/arcvpn-node/release-version /opt/arcvpn-node/bin/agent /opt/arcvpn-node/bin/xray /etc/arcvpn-node/.arcvpn-managed
# These rmdir calls will fail rather than recurse through unexpected data.
rmdir /var/lib/arcvpn-node /var/lib/arcvpn-node-root /etc/arcvpn-node /opt/arcvpn-node/bin /opt/arcvpn-node
if id arcvpn-node >/dev/null 2>&1; then userdel arcvpn-node; fi
if id arcvpn-xray >/dev/null 2>&1; then userdel arcvpn-xray; fi
systemctl daemon-reload
printf 'managed_resources_removed\n'
`

func RemovalScript(mode UninstallMode, operationID string) (string, error) {
	if err := validRemoval(mode, operationID); err != nil {
		return "", err
	}
	body := strings.ReplaceAll(removalScript, "__OPERATION__", ShellString(operationID))
	return strings.ReplaceAll(body, "__MODE__", ShellString(string(mode))), nil
}

// RemovalPlan is read-only and never outputs any secrets.
type RemovalPlan struct {
	InstallationID string        `json:"installation_id"`
	State          string        `json:"state"`
	Mode           UninstallMode `json:"mode"`
	Changes        []string      `json:"changes"`
}

func NewRemovalPlan(installationID string, mode UninstallMode) (RemovalPlan, error) {
	if err := validRemoval(mode, installationID); err != nil {
		return RemovalPlan{}, err
	}
	p := RemovalPlan{InstallationID: installationID, State: "managed", Mode: mode}
	if mode == UninstallDetach {
		p.Changes = []string{"Stop and disable only ArcVPN agent and helper services", "Delete agent identity, agent binary and helper receipt", "Keep ArcVPN Xray service, configuration and listener running", "Controller identity must be revoked separately"}
	} else {
		p.Changes = []string{"Stop ArcVPN agent/helper/Xray services", "Remove only recognized ArcVPN systemd units, files and accounts", "Preserve all foreign services, SSH and firewall rules", "Remove controller identity only after remote cleanup succeeds"}
	}
	return p, nil
}

func (c *Connection) privileged(ctx context.Context, body string) ([]byte, error) {
	uid, err := c.exec(ctx, nil, "id", "-u")
	if err != nil {
		return nil, err
	}
	command := []string{"sh", "-s"}
	if strings.TrimSpace(string(uid)) != "0" {
		command = []string{"sudo", "-n", "sh", "-s"}
	}
	return c.execWithTimeout(ctx, 90*time.Second, strings.NewReader(body), command...)
}

func (c *Connection) ReadManaged(ctx context.Context) (string, error) {
	out, err := c.privileged(ctx, `set -eu
marker=/etc/arcvpn-node/.arcvpn-managed
[ -f "$marker" ] && [ ! -L "$marker" ] || exit 42
cat "$marker"
`)
	if err != nil {
		return "", errors.New("cannot verify root-owned ArcVPN installation marker")
	}
	marker := strings.TrimSpace(string(out))
	if operationIDPattern.MatchString(marker) || strings.HasPrefix(marker, "detached:") && operationIDPattern.MatchString(strings.TrimPrefix(marker, "detached:")) {
		return marker, nil
	}
	return "", errors.New("unexpected ArcVPN installation marker")
}

func (c *Connection) RegistrationExists(ctx context.Context) (bool, error) {
	out, err := c.privileged(ctx, `set -eu
if [ -f /var/lib/arcvpn-node/node.json ]; then printf 'yes\n'; else printf 'no\n'; fi
`)
	if err != nil {
		return false, err
	}
	switch strings.TrimSpace(string(out)) {
	case "yes":
		return true, nil
	case "no":
		return false, nil
	}
	return false, errors.New("ambiguous registration state")
}

// removalReceiptScript never alters state. A successful purge requires that
// ALL managed files, units and accounts have gone. A successful detach
// requires the detached marker and a still-running Xray service.
func removalReceiptScript(mode UninstallMode, installationID string) (string, error) {
	if err := validRemoval(mode, installationID); err != nil {
		return "", err
	}
	body := `set -eu
expected=__OPERATION__
mode=__MODE__
marker=/etc/arcvpn-node/.arcvpn-managed
if [ "$mode" = detach ]; then
  if [ -f "$marker" ] && [ ! -L "$marker" ] &&
    [ "$(cat "$marker")" = "detached:$expected" ] &&
    [ ! -e /etc/systemd/system/arcvpn-node.service ] &&
    [ ! -e /etc/systemd/system/arcvpn-node-helper.socket ] &&
    [ ! -e /var/lib/arcvpn-node/node.json ] &&
    [ ! -e /opt/arcvpn-node/bin/agent ] &&
    systemctl is-active --quiet arcvpn-node-xray.service; then
      printf 'detached_verified\n'
  else
      printf 'incomplete\n'
  fi
else
  if [ ! -e "$marker" ] && [ ! -e /etc/arcvpn-node ] &&
    [ ! -e /opt/arcvpn-node ] && [ ! -e /var/lib/arcvpn-node ] &&
    [ ! -e /var/lib/arcvpn-node-root ] &&
    [ ! -e /etc/systemd/system/arcvpn-node.service ] &&
    [ ! -e /etc/systemd/system/arcvpn-node-helper.socket ] &&
    [ ! -e /etc/systemd/system/arcvpn-node-helper.service ] &&
    [ ! -e /etc/systemd/system/arcvpn-node-xray.service ] &&
    ! id arcvpn-node >/dev/null 2>&1 && ! id arcvpn-xray >/dev/null 2>&1; then
      printf 'purged_verified\n'
  else
      printf 'incomplete\n'
  fi
fi
`
	body = strings.ReplaceAll(body, "__OPERATION__", ShellString(installationID))
	return strings.ReplaceAll(body, "__MODE__", ShellString(string(mode))), nil
}

func (c *Connection) RemovalReceipt(ctx context.Context, installationID string, mode UninstallMode) (bool, error) {
	script, err := removalReceiptScript(mode, installationID)
	if err != nil {
		return false, err
	}
	out, err := c.privileged(ctx, script)
	if err != nil {
		return false, err
	}
	want := "purged_verified"
	if mode == UninstallDetach {
		want = "detached_verified"
	}
	text := strings.TrimSpace(string(out))
	if text == want {
		return true, nil
	}
	if text == "incomplete" {
		return false, nil
	}
	return false, errors.New("invalid remote removal readback")
}

// RemoveVerified requires a pinned SSH host key and the exact installation ID
// from the controller's journal; no broad shell API is exposed to the agent.
func (c *Connection) RemoveVerified(ctx context.Context, installationID string, mode UninstallMode) error {
	script, err := RemovalScript(mode, installationID)
	if err != nil {
		return err
	}
	current, err := c.ReadManaged(ctx)
	if err != nil {
		return err
	}
	if current != installationID && !(mode == UninstallPurge && current == "detached:"+installationID) {
		return errors.New("remote installation marker differs; refusing destructive operation")
	}
	out, err := c.privileged(ctx, script)
	if err != nil {
		// Only success after the first marker was already verified, and the
		// exact desired terminal state is observed over the pinned SSH link.
		checkCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if verified, checkErr := c.RemovalReceipt(checkCtx, installationID, mode); checkErr == nil && verified {
			return nil
		}
		return errors.New("remote removal incomplete or SSH receipt lost; inspect VPS through provider console")
	}
	wanted := "managed_resources_removed"
	if mode == UninstallDetach {
		wanted = "detached_vpn_remains_active"
	}
	if strings.TrimSpace(string(out)) != wanted {
		return fmt.Errorf("unexpected remote removal receipt")
	}
	return nil
}

// RemoveLocal uses the same bounded script. It deliberately does not remove
// foreign services or attempt an unattended firewall/SSH change.
func RemoveLocal(ctx context.Context, installationID string, mode UninstallMode) error {
	if os.Geteuid() != 0 {
		return errors.New("node removal requires root")
	}
	script, err := RemovalScript(mode, installationID)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	command := exec.CommandContext(runCtx, "sh", "-s")
	command.Stdin = strings.NewReader(script)
	output, err := command.Output()
	if err != nil {
		return errors.New("local managed removal incomplete; inspect service and known files")
	}
	wanted := "managed_resources_removed"
	if mode == UninstallDetach {
		wanted = "detached_vpn_remains_active"
	}
	if strings.TrimSpace(string(output)) != wanted {
		return errors.New("unexpected local removal receipt")
	}
	return nil
}
