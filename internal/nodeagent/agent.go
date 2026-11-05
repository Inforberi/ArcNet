package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/arcvpn/arcvpn/internal/metrics"
	"github.com/arcvpn/arcvpn/internal/nodeprotocol"
	"github.com/arcvpn/arcvpn/internal/noderuntime"
	"github.com/arcvpn/arcvpn/internal/store"
)

// Version can be stamped by the reproducible release build at link time.
var Version = "0.6.5-stage4"

type Configuration struct {
	Controller     string `json:"controller"`
	NodeID         string `json:"node_id"`
	Secret         string `json:"secret"`
	PreviousSecret string `json:"previous_secret,omitempty"`
	RotatedAt      int64  `json:"rotated_at,omitempty"`
}

func ValidateOrigin(raw string) (string, error) {
	return nodeprotocol.ValidateHTTPSOrigin(raw)
}

func Client() *http.Client {
	return &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}
func Register(ctx context.Context, controller, token, configPath string) error {
	return register(ctx, Client(), controller, token, configPath)
}
func registerIdentity(ctx context.Context, client *http.Client, controller, token string) (Configuration, error) {
	origin, err := ValidateOrigin(controller)
	if err != nil {
		return Configuration{}, err
	}
	if len(token) < 40 || len(token) > 128 || strings.ContainsAny(token, " \r\n\t\x00") {
		return Configuration{}, errors.New("invalid enrollment token")
	}
	payload, _ := json.Marshal(map[string]string{"token": token})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+"/api/nodes/register", bytes.NewReader(payload))
	if err != nil {
		return Configuration{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return Configuration{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return Configuration{}, fmt.Errorf("registration rejected: HTTP %d", res.StatusCode)
	}
	var out struct {
		NodeID string `json:"node_id"`
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2048)).Decode(&out); err != nil {
		return Configuration{}, err
	}
	if len(out.Secret) < 40 || len(out.Secret) > 128 || out.NodeID == "" || len(out.NodeID) > 128 {
		return Configuration{}, errors.New("invalid controller registration response")
	}
	return Configuration{Controller: origin, NodeID: out.NodeID, Secret: out.Secret}, nil
}

func register(ctx context.Context, client *http.Client, controller, token, configPath string) error {
	// Check nonexistence BEFORE consuming the one-time token. O_EXCL closes
	// the remaining race with a concurrent registration.
	if _, err := os.Lstat(configPath); err == nil {
		return errors.New("node already registered; refusing to overwrite credentials")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	identity, err := registerIdentity(ctx, client, controller, token)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		os.Remove(configPath)
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		os.Remove(configPath)
		return err
	}
	if err = f.Close(); err != nil {
		os.Remove(configPath)
		return err
	}
	return nil
}

// Rebind changes agent authority ONLY after explicit administrator revocation
// of the old identity and after the old service was stopped by the CLI.
// The existing Xray configuration is neither replaced nor restarted.
func Rebind(ctx context.Context, controller, token, configPath string) error {
	return rebind(ctx, Client(), controller, token, configPath)
}
func rebind(ctx context.Context, client *http.Client, controller, token, configPath string) error {
	old, err := ReadConfig(configPath)
	if err != nil {
		return err
	}
	if old.PreviousSecret != "" {
		return errors.New("incomplete credential rotation; recover before rebind")
	}
	origin, err := ValidateOrigin(controller)
	if err != nil {
		return err
	}
	if origin == old.Controller {
		return errors.New("new controller must be different from the current controller")
	}
	// Make no changes to the old credential unless the new enrollment succeeds.
	next, err := registerIdentity(ctx, client, origin, token)
	if err != nil {
		return err
	}
	if err = saveConfiguration(configPath, next); err != nil {
		return errors.New("new registration accepted but local credential replacement failed; old identity retained")
	}
	return nil
}

const maxCredentialFileSize = 16 << 10

// ReadConfig opens the credential atomically with O_NOFOLLOW. Checking with
// Lstat and then calling os.ReadFile would allow a malicious replacement of
// node.json with a symlink between the permission check and the actual read.
// Limit the payload before allocating to avoid exhausting a long-running agent.
func ReadConfig(path string) (Configuration, error) {
	var c Configuration
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return c, errors.New("cannot securely open node credential file")
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0077 != 0 || fi.Size() > maxCredentialFileSize {
		return c, errors.New("node configuration must be a private regular file within size limit")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxCredentialFileSize+1))
	if err != nil || len(data) > maxCredentialFileSize {
		return c, errors.New("cannot read bounded node credential file")
	}
	if err = json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	if _, err = ValidateOrigin(c.Controller); err != nil {
		return c, err
	}
	if c.NodeID == "" || len(c.Secret) < 40 || len(c.Secret) > 128 || (c.PreviousSecret != "" && (len(c.PreviousSecret) < 40 || len(c.PreviousSecret) > 128)) {
		return c, errors.New("invalid node credentials")
	}
	return c, nil
}

func Heartbeat(ctx context.Context, c Configuration) error {
	return heartbeat(ctx, Client(), c)
}
func heartbeat(ctx context.Context, client *http.Client, c Configuration) error {
	m := metrics.Read()
	state := "unknown"
	svcCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	output, _ := exec.CommandContext(svcCtx, "systemctl", "is-active", "arcvpn-node-xray.service").Output()
	switch strings.TrimSpace(string(output)) {
	case "active":
		state = "active"
	case "inactive", "failed":
		state = "inactive"
	}
	cancel()
	b, err := json.Marshal(store.NodeHeartbeat{Version: Version, ProtocolVersion: nodeprotocol.Current, XrayState: state, MemoryTotal: m.MemoryTotal, MemoryAvailable: m.MemoryAvailable, Load1: m.Load1, UptimeSeconds: m.UptimeSeconds})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Controller+"/api/nodes/heartbeat", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Secret)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 204 {
		return fmt.Errorf("heartbeat rejected: HTTP %d", res.StatusCode)
	}
	return nil
}

// Send a bounded authenticated request. Client follows no redirects, preventing
// credential forwarding to attacker-controlled origins.
func request(ctx context.Context, client *http.Client, c Configuration, method, path string, body any, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Controller+path, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode == 200 {
		if err = json.NewDecoder(io.LimitReader(resp.Body, noderuntime.MaxRequest+4096)).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func PollAndApply(ctx context.Context, client *http.Client, c Configuration, socket string) error {
	var cmd *store.NodeCommand
	status, err := request(ctx, client, c, http.MethodGet, "/api/nodes/poll?node_id="+url.QueryEscape(c.NodeID), nil, &cmd)
	if err != nil {
		return err
	}
	if status == 204 {
		return nil
	}
	if status != 200 {
		return fmt.Errorf("node poll refused: HTTP %d", status)
	}
	if cmd == nil {
		return errors.New("invalid controller command")
	}
	request := noderuntime.ApplyRequest{ProtocolVersion: cmd.ProtocolVersion, Kind: cmd.Kind, OperationID: cmd.OperationID, Revision: cmd.Revision, SHA256: cmd.SHA256, Config: cmd.Config}
	if err = noderuntime.ValidateRequest(request); err != nil {
		return err
	}
	response, applyErr := noderuntime.CallHelper(ctx, socket, request)
	result := "failed"
	if applyErr == nil {
		result = noderuntime.SafeStatus(response.Status)
	}
	status, err = requestNodeACK(ctx, client, c, *cmd, result)
	if err != nil {
		return err
	}
	if status != 204 {
		return fmt.Errorf("node acknowledgment refused: HTTP %d", status)
	}
	if applyErr != nil {
		return errors.New("local root-helper rejected apply operation")
	}
	if result != "applied" {
		return errors.New("Xray apply failed or was rolled back")
	}
	return nil
}
func requestNodeACK(ctx context.Context, client *http.Client, c Configuration, command store.NodeCommand, result string) (int, error) {
	return request(ctx, client, c, http.MethodPost, "/api/nodes/ack", map[string]any{"node_id": c.NodeID, "operation_id": command.OperationID, "revision": command.Revision, "sha256": command.SHA256, "result": result}, nil)
}

func saveConfiguration(path string, c Configuration) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".node-rotate-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
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
	// Rebind is a root-only CLI operation. Preserve the existing service
	// account ownership; otherwise root would leave node.json unreadable to
	// the unprivileged daemon after atomic replacement.
	if current, statErr := os.Stat(path); statErr == nil {
		if owner, ok := current.Sys().(*syscall.Stat_t); ok {
			if tempStat, err := os.Stat(f.Name()); err == nil {
				if tempOwner, ok := tempStat.Sys().(*syscall.Stat_t); ok && (owner.Uid != tempOwner.Uid || owner.Gid != tempOwner.Gid) {
					if err := os.Chown(f.Name(), int(owner.Uid), int(owner.Gid)); err != nil {
						return err
					}
				}
			}
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	if dir, e := os.Open(filepath.Dir(path)); e == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
func RotateCredential(ctx context.Context, client *http.Client, c *Configuration, path string) error {
	var rotation struct {
		Secret string `json:"secret"`
	}
	status, err := request(ctx, client, *c, http.MethodPost, "/api/nodes/rotate/begin", map[string]string{"node_id": c.NodeID}, &rotation)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("credential rotation HTTP %d", status)
	}
	if len(rotation.Secret) < 40 || len(rotation.Secret) > 128 {
		return errors.New("invalid rotated credential")
	}
	next := *c
	next.Secret = rotation.Secret
	next.PreviousSecret = c.Secret
	next.RotatedAt = time.Now().Unix()
	if err = saveConfiguration(path, next); err != nil {
		return err
	}
	*c = next
	if err = ConfirmCredential(ctx, client, *c); err != nil {
		// New and previous credentials are persisted until the controller
		// confirms. The agent can recover after a lost ACK or restart.
		return err
	}
	c.PreviousSecret = ""
	return saveConfiguration(path, *c)
}

// RecoverCredential reconciles a crash between writing new credentials and
// committing them at the controller. Never discard both possible valid keys.
func RecoverCredential(ctx context.Context, client *http.Client, c *Configuration, path string) error {
	if c.PreviousSecret == "" {
		return nil
	}
	if err := ConfirmCredential(ctx, client, *c); err == nil {
		c.PreviousSecret = ""
		return saveConfiguration(path, *c)
	}
	if err := heartbeat(ctx, client, *c); err == nil {
		// Confirm was committed; the response was lost.
		c.PreviousSecret = ""
		return saveConfiguration(path, *c)
	}
	previous := *c
	previous.Secret = c.PreviousSecret
	previous.PreviousSecret = ""
	if err := heartbeat(ctx, client, previous); err != nil {
		return errors.New("neither node credential could be confirmed")
	}
	*c = previous
	return saveConfiguration(path, *c)
}
func ConfirmCredential(ctx context.Context, client *http.Client, c Configuration) error {
	status, err := request(ctx, client, c, http.MethodPost, "/api/nodes/rotate/confirm", map[string]string{"node_id": c.NodeID}, nil)
	if err != nil {
		return err
	}
	if status != 204 {
		return fmt.Errorf("credential confirm HTTP %d", status)
	}
	return nil
}

// contactDelay uses bounded exponential backoff with up to 20% jitter. A
// failed control-plane request must not turn a controller outage into a
// synchronized retry storm across many nodes. VPN traffic remains independent.
func contactDelay(consecutiveFailures int) time.Duration {
	const normal = 20 * time.Second
	const maximum = 5 * time.Minute
	if consecutiveFailures <= 0 {
		return normal
	}
	base := normal
	for attempt := 1; attempt < consecutiveFailures; attempt++ {
		if base >= maximum/2 {
			base = maximum
			break
		}
		base *= 2
	}
	if base >= maximum {
		return maximum
	}
	return base + time.Duration(rand.Int63n(int64(base/5)+1))
}

// Run maintains outbound-only HTTPS contact. Errors never terminate the VPN
// process. Rotation is crash-recoverable and bounded to one attempt per day.
// Each cycle is bounded; retries are serialized and scheduled only after the
// preceding cycle completes, so a slow controller cannot pile up goroutines.
func Run(ctx context.Context, c Configuration, configPath string, report func(error)) {
	client := Client()
	telemetryCtx, stopTelemetry := context.WithCancel(ctx)
	telemetryDone := make(chan struct{})
	go func() { defer close(telemetryDone); runTelemetry(telemetryCtx, client, configPath, report) }()
	defer func() { stopTelemetry(); <-telemetryDone }()
	rotateAfter := time.Now().Add(24 * time.Hour)
	if c.RotatedAt > 0 {
		deadline := time.Unix(c.RotatedAt, 0).Add(24 * time.Hour)
		if deadline.After(time.Now()) {
			rotateAfter = deadline
		}
	}
	cycle := func() bool {
		requestCtx, cancel := context.WithTimeout(ctx, 115*time.Second)
		defer cancel()
		if c.PreviousSecret != "" {
			if err := RecoverCredential(requestCtx, client, &c, configPath); err != nil {
				report(errors.New("agent credential recovery failed"))
				return false
			}
		}
		if err := heartbeat(requestCtx, client, c); err != nil {
			report(errors.New("agent heartbeat unavailable"))
			return false
		}
		if err := PollAndApply(requestCtx, client, c, ""); err != nil {
			report(err)
			return false
		}
		if err := PollTargetChecks(requestCtx, client, c); err != nil {
			report(errors.New("agent target check unavailable"))
		}
		if !time.Now().Before(rotateAfter) && c.PreviousSecret == "" {
			// Limit further attempts even after failure, preventing repeated
			// registration requests against a transiently degraded controller.
			rotateAfter = time.Now().Add(24 * time.Hour)
			if err := RotateCredential(requestCtx, client, &c, configPath); err != nil {
				report(errors.New("agent credential rotation incomplete"))
				return false
			}
		}
		return true
	}
	failures := 0
	for ctx.Err() == nil {
		if cycle() {
			failures = 0
		} else if failures < 32 {
			failures++
		}
		timer := time.NewTimer(contactDelay(failures))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}
	}
}
