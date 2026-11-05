package nodebootstrap

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/nodeprotocol"
)

// LocalPreflight performs exactly the same read-only checks as SSH preflight.
// Running this does not install packages, modify firewall or access sshd.
func LocalPreflight(ctx context.Context, vpnPort int) (Preflight, error) {
	if vpnPort < 1 || vpnPort > 65535 {
		return Preflight{}, errors.New("invalid VPN port")
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	command := exec.CommandContext(cctx, "sh", "-s", "--", strconv.Itoa(vpnPort))
	command.Stdin = strings.NewReader(readOnlyPreflight)
	command.Stderr = io.Discard
	output, err := command.Output()
	if err != nil {
		return Preflight{}, errors.New("local read-only preflight failed")
	}
	if len(output) > 16384 {
		return Preflight{}, errors.New("local preflight output too large")
	}
	return parsePreflight(HostKey{Host: "localhost"}, output), nil
}

// LocalPreflightForController performs an additional HTTPS check against the
// configured controller, with no TLS bypass and no token in the command line.
func LocalPreflightForController(ctx context.Context, controller string, vpnPort int) (Preflight, error) {
	if err := validateProbeOrigin(controller); err != nil {
		return Preflight{}, err
	}
	plan, err := LocalPreflight(ctx, vpnPort)
	if err != nil {
		return Preflight{}, err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	command := exec.CommandContext(checkCtx, "sh", "-s", "--", controller)
	command.Stdin = strings.NewReader(controllerHTTPSPreflight)
	command.Stderr = io.Discard
	output, err := command.Output()
	if err != nil {
		return Preflight{}, errors.New("local controller HTTPS reachability check failed")
	}
	parts := strings.SplitN(strings.TrimSpace(string(output)), "=", 2)
	if len(parts) != 2 || parts[0] != "controller_https" {
		return Preflight{}, errors.New("invalid local controller HTTPS probe result")
	}
	addControllerReachability(&plan, parts[1])
	return plan, nil
}

// InstallLocal requires a trusted ArcVPN verifier already present on the VPS,
// an independently signed node release and public key, and a single-use token
// read from stdin. No unsigned executable or network script is fetched.
// Success proves the node service started, never a VPN client handshake.
func InstallLocal(ctx context.Context, release Release, controller, token string, vpnPort int) (string, error) {
	if os.Geteuid() != 0 {
		return "", errors.New("local node installer requires root")
	}
	if len(token) < 40 || len(token) > 128 || strings.ContainsAny(token, " \r\n\t\x00") {
		return "", errors.New("invalid enrollment token")
	}
	if _, err := versionPatch(release.Version); err != nil {
		return "", err
	}
	if len(release.Agent) == 0 || len(release.Xray) == 0 {
		return "", errors.New("signed node bundle must be verified before installation")
	}
	// Only HTTPS origins without extra path/query may receive enrollment.
	if _, err := nodeControllerOrigin(controller); err != nil {
		return "", err
	}
	check, err := LocalPreflightForController(ctx, controller, vpnPort)
	if err != nil {
		return "", err
	}
	if len(check.Blockers) != 0 {
		return "", errors.New("node preflight blockers prevent installation")
	}
	operationID, err := auth.UUID()
	if err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp("", "arcvpn-local-stage-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	for name, payload := range map[string][]byte{"agent": release.Agent, "xray": release.Xray} {
		file, err := os.OpenFile(filepath.Join(stage, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
		if err != nil {
			return "", err
		}
		_, writeErr := file.Write(payload)
		if writeErr == nil {
			writeErr = file.Sync()
		}
		closeErr := file.Close()
		if writeErr != nil {
			return "", writeErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	setupCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	command := exec.CommandContext(setupCtx, "sh", "-s")
	command.Stdin = strings.NewReader(installBody(stage, operationID, release.Version, release.Agent, release.Xray))
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	err = command.Run()
	cancel()
	if err != nil {
		return "", errors.New("managed installation failed; inspect host before retrying")
	}
	// The secret is sent through fd0 of the unprivileged agent, never argv,
	// process environment or a shell script. The remote controller consumes it.
	registerCtx, stop := context.WithTimeout(ctx, 20*time.Second)
	reg := exec.CommandContext(registerCtx, "runuser", "-u", "arcvpn-node", "--", "/opt/arcvpn-node/bin/agent", "node-register", "--controller", controller, "--token-stdin", "--config", "/var/lib/arcvpn-node/node.json")
	reg.Stdin = strings.NewReader(token + "\n")
	reg.Stdout = io.Discard
	reg.Stderr = io.Discard
	err = reg.Run()
	stop()
	if err != nil {
		// If registration succeeded remotely but its response was lost, the
		// persisted identity remains. Never delete it speculatively.
		if _, statErr := os.Stat("/var/lib/arcvpn-node/node.json"); errors.Is(statErr, os.ErrNotExist) {
			rollbackCtx, done := context.WithTimeout(context.Background(), 90*time.Second)
			rollbackErr := RemoveLocal(rollbackCtx, operationID, UninstallPurge)
			done()
			if rollbackErr != nil {
				return operationID, errors.New("registration failed; automatic managed rollback also failed; inspect node")
			}
			return operationID, errors.New("registration failed; managed installation rolled back")
		}
		return operationID, errors.New("registration acknowledgment uncertain; inspect node identity before retrying")
	}
	startCtx, done := context.WithTimeout(ctx, 25*time.Second)
	defer done()
	start := exec.CommandContext(startCtx, "systemctl", "enable", "--now", "arcvpn-node.service")
	start.Stderr = io.Discard
	start.Stdout = io.Discard
	if err := start.Run(); err != nil {
		return operationID, errors.New("node identity registered but agent service failed to start; managed installation retained for repair")
	}
	return operationID, nil
}

// Used by the local installer without importing the entire agent package.
func nodeControllerOrigin(value string) (string, error) {
	return validateControllerOrigin(value)
}

func validateControllerOrigin(value string) (string, error) {
	return nodeprotocol.ValidateHTTPSOrigin(value)
}
