// Package noderuntime exposes only a single privileged operation: install a
// validated ArcVPN Xray config with rollback. It never accepts shell or paths.
package noderuntime

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/arcvpn/arcvpn/internal/usage"
)

const Socket = "/run/arcvpn-node/helper.sock"
const MaxRequest = 256 << 10
const Service = "arcvpn-node-xray.service"

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{16,64}$`)
var hexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ApplyRequest struct {
	ProtocolVersion int             `json:"protocol_version"`
	Kind            string          `json:"kind"`
	OperationID     string          `json:"operation_id"`
	Revision        int64           `json:"revision"`
	SHA256          string          `json:"sha256"`
	Config          json.RawMessage `json:"config,omitempty"`
}
type ApplyResponse struct {
	UsageProtocol   int               `json:"usage_protocol,omitempty"`
	Usage           *usage.NodeReport `json:"usage,omitempty"`
	CascadeProtocol int               `json:"cascade_protocol,omitempty"`
	DeviceProtocol  int               `json:"device_protocol,omitempty"`
	Status          string            `json:"status"`
	Error           string            `json:"error,omitempty"`
}

type Runner interface {
	Run(context.Context, string, ...string) error
}
type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...)
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	return c.Run()
}

type Installer struct {
	XrayBinary  string
	ConfigPath  string
	ReceiptPath string
	Runner      Runner
}

func (i Installer) defaults() Installer {
	if i.XrayBinary == "" {
		i.XrayBinary = "/opt/arcvpn-node/bin/xray"
	}
	if i.ConfigPath == "" {
		i.ConfigPath = "/etc/arcvpn-node/xray.json"
	}
	if i.ReceiptPath == "" {
		i.ReceiptPath = "/var/lib/arcvpn-node-root/last-apply.json"
	}
	if i.Runner == nil {
		i.Runner = OSRunner{}
	}
	return i
}

// ValidateConfig uses allowlisted keys. Agent compromise cannot pass arbitrary
// Xray runtime fields (e.g. API, reverse, transport file paths, socket marks).
func ValidateConfig(raw []byte) error {
	if len(raw) == 0 || len(raw) > MaxRequest-4096 {
		return errors.New("config exceeds limit")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return errors.New("invalid JSON")
	}
	if err := allow(root, "log", "inbounds", "outbounds"); err != nil {
		return err
	}
	var logLevel map[string]json.RawMessage
	if json.Unmarshal(root["log"], &logLevel) != nil {
		return errors.New("missing log")
	}
	if err := allow(logLevel, "loglevel"); err != nil {
		return err
	}
	var inbounds []map[string]json.RawMessage
	if json.Unmarshal(root["inbounds"], &inbounds) != nil || len(inbounds) != 1 {
		return errors.New("exactly one VLESS listener required")
	}
	for _, in := range inbounds {
		if err := allow(in, "listen", "port", "protocol", "tag", "settings", "streamSettings", "sniffing"); err != nil {
			return err
		}
		if stringValue(in["listen"]) != "0.0.0.0" || stringValue(in["protocol"]) != "vless" || stringValue(in["tag"]) != "vless-reality" {
			return errors.New("unsupported listener")
		}
		var port int
		if json.Unmarshal(in["port"], &port) != nil || port < 1 || port > 65535 {
			return errors.New("invalid listener port")
		}
		var settings map[string]json.RawMessage
		if json.Unmarshal(in["settings"], &settings) != nil {
			return errors.New("invalid VLESS settings")
		}
		if err := allow(settings, "clients", "decryption"); err != nil {
			return err
		}
		if stringValue(settings["decryption"]) != "none" {
			return errors.New("unsupported decryption")
		}
		var clients []map[string]json.RawMessage
		if json.Unmarshal(settings["clients"], &clients) != nil || len(clients) > 5000 {
			return errors.New("invalid client list")
		}
		for _, cl := range clients {
			if err := allow(cl, "id", "flow", "email"); err != nil {
				return err
			}
			if len(stringValue(cl["id"])) != 36 || stringValue(cl["flow"]) != "xtls-rprx-vision" {
				return errors.New("invalid VLESS client")
			}
		}
		var stream map[string]json.RawMessage
		if json.Unmarshal(in["streamSettings"], &stream) != nil {
			return errors.New("invalid stream settings")
		}
		if err := allow(stream, "network", "security", "realitySettings"); err != nil {
			return err
		}
		if stringValue(stream["network"]) != "tcp" || stringValue(stream["security"]) != "reality" {
			return errors.New("unsupported transport")
		}
		var reality map[string]json.RawMessage
		if json.Unmarshal(stream["realitySettings"], &reality) != nil {
			return errors.New("invalid Reality settings")
		}
		if err := allow(reality, "show", "target", "serverNames", "privateKey", "shortIds"); err != nil {
			return err
		}
		if len(stringValue(reality["privateKey"])) < 32 {
			return errors.New("invalid Reality private key")
		}
		if stringValue(reality["target"]) == "" {
			return errors.New("missing Reality target")
		}
		if v, ok := in["sniffing"]; ok {
			var sn map[string]json.RawMessage
			if json.Unmarshal(v, &sn) != nil {
				return errors.New("invalid sniffing")
			}
			if err := allow(sn, "enabled", "destOverride", "routeOnly"); err != nil {
				return err
			}
		}
	}
	var outbounds []map[string]json.RawMessage
	if json.Unmarshal(root["outbounds"], &outbounds) != nil || len(outbounds) != 2 {
		return errors.New("expected direct/block outbounds")
	}
	seen := map[string]bool{}
	for _, o := range outbounds {
		if err := allow(o, "protocol", "tag"); err != nil {
			return err
		}
		kind := stringValue(o["protocol"])
		tag := stringValue(o["tag"])
		if kind == "freedom" && tag == "direct" {
			seen["direct"] = true
		} else if kind == "blackhole" && tag == "block" {
			seen["block"] = true
		} else {
			return errors.New("unsupported outbound")
		}
	}
	if !seen["direct"] || !seen["block"] {
		return errors.New("missing required outbound")
	}
	return nil
}
func allow(obj map[string]json.RawMessage, keys ...string) error {
	good := map[string]bool{}
	for _, key := range keys {
		good[key] = true
	}
	for key := range obj {
		if !good[key] {
			return fmt.Errorf("field %s not allowed", key)
		}
	}
	return nil
}
func stringValue(raw json.RawMessage) string {
	var str string
	_ = json.Unmarshal(raw, &str)
	return str
}

func ValidateRequest(req ApplyRequest) error {
	if req.Kind == "capabilities_v1" || req.Kind == "statistics_v1" {
		if req.ProtocolVersion == 1 && req.OperationID == "" && req.Revision == 0 && req.SHA256 == "" && len(req.Config) == 0 {
			return nil
		}
		return errors.New("invalid capability query")
	}
	if req.ProtocolVersion != 1 || (req.Kind != "apply_xray_v1" && req.Kind != "apply_cascade_xray_v1" && req.Kind != "apply_xray_devices_v1" && req.Kind != "apply_xray_usage_v1") || !idPattern.MatchString(req.OperationID) || req.Revision < 1 || !hexPattern.MatchString(req.SHA256) {
		return errors.New("unsupported or malformed apply operation")
	}
	sum := sha256.Sum256(req.Config)
	if hex.EncodeToString(sum[:]) != req.SHA256 {
		return errors.New("payload digest mismatch")
	}
	if req.Kind == "apply_cascade_xray_v1" {
		return ValidateCascadeConfig(req.Config)
	}
	if req.Kind == "apply_xray_usage_v1" {
		return ValidateStatisticsConfig(req.Config)
	}
	return ValidateConfig(req.Config)
}

// Write temp+fsync+rename, refusing symlinks and special existing files.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if st, err := os.Lstat(path); err == nil {
		if !st.Mode().IsRegular() {
			return errors.New("refusing nonregular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".arcvpn-stage-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if filepath.Clean(path) == "/etc/arcvpn-node/xray.json" && os.Geteuid() == 0 {
		serviceUser, lookupErr := user.Lookup("arcvpn-xray")
		if lookupErr != nil {
			return lookupErr
		}
		groupID, parseErr := strconv.Atoi(serviceUser.Gid)
		if parseErr != nil {
			return parseErr
		}
		if chownErr := os.Chown(f.Name(), 0, groupID); chownErr != nil {
			return chownErr
		}
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

// readPrivilegedState rejects symlinks and special files before loading
// last-good configurations and root-owned receipts. The descriptor remains
// unchanged between the security check and the bounded read.
func readPrivilegedState(path string, max int64) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > max {
		return nil, errors.New("invalid or oversized privileged state file")
	}
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || int64(len(data)) > max {
		return nil, errors.New("cannot read bounded privileged state")
	}
	return data, nil
}

func (raw Installer) Apply(ctx context.Context, request ApplyRequest) (ApplyResponse, error) {
	i := raw.defaults()
	if err := ValidateRequest(request); err != nil {
		return ApplyResponse{Status: "failed"}, err
	}
	if request.Kind == "capabilities_v1" {
		return ApplyResponse{Status: "capabilities", CascadeProtocol: 1, DeviceProtocol: 1, UsageProtocol: 1}, nil
	}
	if request.Kind == "statistics_v1" {
		report, err := i.Statistics(ctx)
		return ApplyResponse{Status: "statistics", Usage: report}, err
	}
	if receipt, err := readPrivilegedState(i.ReceiptPath, 4096); err == nil {
		var prior struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
			SHA256   string `json:"sha256"`
		}
		if json.Unmarshal(receipt, &prior) != nil || !idPattern.MatchString(prior.ID) || !hexPattern.MatchString(prior.SHA256) || prior.Revision < 1 {
			return ApplyResponse{Status: "failed"}, errors.New("corrupt previous apply receipt; manual recovery required")
		}
		if prior.Revision > request.Revision {
			return ApplyResponse{Status: "failed"}, errors.New("revision downgrade refused")
		}
		if prior.Revision == request.Revision {
			if prior.ID != request.OperationID || prior.SHA256 != request.SHA256 {
				return ApplyResponse{Status: "failed"}, errors.New("conflicting operation replay")
			}
			// A receipt alone cannot prove the Xray service still serves the
			// exact configuration after a crash, restart or manual repair.
			active, activeErr := readPrivilegedState(i.ConfigPath, MaxRequest)
			if activeErr != nil || sha256.Sum256(active) != sha256.Sum256(request.Config) {
				return ApplyResponse{Status: "failed"}, errors.New("applied revision differs from on-disk Xray configuration")
			}
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := i.Runner.Run(checkCtx, "/usr/bin/systemctl", "is-active", "--quiet", Service); err != nil {
				return ApplyResponse{Status: "failed"}, errors.New("previously applied Xray service is not active")
			}
			return ApplyResponse{Status: "applied"}, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ApplyResponse{Status: "failed"}, err
	}

	previous, readErr := readPrivilegedState(i.ConfigPath, MaxRequest)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return ApplyResponse{Status: "failed"}, readErr
	}
	temp, err := os.CreateTemp(filepath.Dir(i.ConfigPath), ".node-validate-*.json")
	if err != nil {
		return ApplyResponse{Status: "failed"}, err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0600); err != nil {
		temp.Close()
		return ApplyResponse{Status: "failed"}, err
	}
	if _, err = temp.Write(request.Config); err != nil {
		temp.Close()
		return ApplyResponse{Status: "failed"}, err
	}
	if err = temp.Close(); err != nil {
		return ApplyResponse{Status: "failed"}, err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = i.Runner.Run(checkCtx, i.XrayBinary, "run", "-test", "-config", temp.Name())
	cancel()
	if err != nil {
		return ApplyResponse{Status: "failed"}, errors.New("Xray rejected new config")
	}
	if err = writeAtomic(i.ConfigPath, request.Config, 0640); err != nil {
		return ApplyResponse{Status: "failed"}, err
	}
	restartCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	err = i.Runner.Run(restartCtx, "/usr/bin/systemctl", "restart", Service)
	if err == nil {
		err = i.Runner.Run(restartCtx, "/usr/bin/systemctl", "is-active", "--quiet", Service)
	}
	cancel()
	if err != nil {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer rollbackCancel()
		if readErr == nil {
			if restoreErr := writeAtomic(i.ConfigPath, previous, 0640); restoreErr != nil {
				return ApplyResponse{Status: "failed"}, errors.New("restart and rollback write both failed")
			}
			if restoreErr := i.Runner.Run(rollbackCtx, "/usr/bin/systemctl", "restart", Service); restoreErr != nil {
				return ApplyResponse{Status: "failed"}, errors.New("restart failed and previous service could not restart")
			}
		} else {
			if removeErr := os.Remove(i.ConfigPath); removeErr != nil {
				return ApplyResponse{Status: "failed"}, errors.New("restart failed and new config could not be removed")
			}
			if stopErr := i.Runner.Run(rollbackCtx, "/usr/bin/systemctl", "stop", Service); stopErr != nil {
				return ApplyResponse{Status: "failed"}, errors.New("restart failed and service could not be stopped")
			}
		}
		return ApplyResponse{Status: "rolled_back"}, nil
	}
	receipt, _ := json.Marshal(map[string]any{"id": request.OperationID, "revision": request.Revision, "sha256": request.SHA256})
	if err = writeAtomic(i.ReceiptPath, receipt, 0600); err != nil {
		return ApplyResponse{Status: "failed"}, errors.New("applied but receipt could not be persisted; operator intervention required")
	}
	return ApplyResponse{Status: "applied"}, nil
}

// CallHelper sends one bounded command over a private Unix socket. No argv,
// temporary config files or shell are involved in the unprivileged process.
func CallHelper(ctx context.Context, socket string, req ApplyRequest) (ApplyResponse, error) {
	if socket == "" {
		socket = Socket
	}
	if err := ValidateRequest(req); err != nil {
		return ApplyResponse{}, err
	}
	b, err := json.Marshal(req)
	if err != nil {
		return ApplyResponse{}, err
	}
	if len(b) > MaxRequest {
		return ApplyResponse{}, errors.New("oversized operation")
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	c, err := d.DialContext(ctx, "unix", socket)
	if err != nil {
		return ApplyResponse{}, err
	}
	defer c.Close()
	deadline := time.Now().Add(80 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = c.SetDeadline(deadline)
	if _, err = c.Write(append(b, '\n')); err != nil {
		return ApplyResponse{}, err
	}
	var response ApplyResponse
	limit := int64(2048)
	if req.Kind == "statistics_v1" {
		limit = MaxRequest
	}
	if err = json.NewDecoder(io.LimitReader(c, limit)).Decode(&response); err != nil {
		return response, err
	}
	if req.Kind == "statistics_v1" {
		if response.Status != "statistics" || response.Usage == nil {
			return ApplyResponse{}, errors.New("node statistics unavailable")
		}
		return response, nil
	}
	if req.Kind == "capabilities_v1" {
		if response.Status != "capabilities" || response.CascadeProtocol != 1 {
			return ApplyResponse{}, errors.New("helper does not support cascade protocol")
		}
		return response, nil
	}
	if response.Status != "applied" && response.Status != "rolled_back" && response.Status != "failed" {
		return response, errors.New("invalid helper reply")
	}
	return response, nil
}

// ServeConn is called ONLY by the root-owned systemd socket listener after
// SO_PEERCRED validates the exact dedicated arcvpn-node account.
func ServeConn(ctx context.Context, conn net.Conn, installer Installer) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(78 * time.Second))
	data, err := bufio.NewReader(io.LimitReader(conn, MaxRequest+1)).ReadBytes('\n')
	if err != nil || len(data) > MaxRequest {
		_ = json.NewEncoder(conn).Encode(ApplyResponse{Status: "failed", Error: "oversized request"})
		return
	}

	var req ApplyRequest
	if err = json.Unmarshal(data, &req); err != nil {
		_ = json.NewEncoder(conn).Encode(ApplyResponse{Status: "failed", Error: "malformed request"})
		return
	}
	response, err := installer.Apply(ctx, req)
	if err != nil {
		response = ApplyResponse{Status: "failed", Error: "node apply failed"}
	}
	_ = json.NewEncoder(conn).Encode(response)
}

// Ensure no unsafe values end up in operation logs.
func SafeStatus(status string) string {
	if status == "applied" || status == "rolled_back" || status == "failed" {
		return status
	}
	return "failed"
}
