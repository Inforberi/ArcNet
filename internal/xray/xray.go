package xray

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
)

var sniPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]{1,250}[a-zA-Z0-9]$`)
var hexID = regexp.MustCompile(`^[a-fA-F0-9]{16}$`)
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{32,64}$`)

func Validate(r store.Reality) error {
	if _, err := netip.ParseAddr(r.Address); err != nil {
		return errors.New("public address must be a valid IP")
	}
	if r.Port < 1 || r.Port > 65535 {
		return errors.New("invalid VPN port")
	}
	if !sniPattern.MatchString(r.SNI) || strings.Contains(r.SNI, "..") {
		return errors.New("invalid Reality SNI")
	}
	if !sniPattern.MatchString(r.Target) || strings.Contains(r.Target, "..") {
		return errors.New("invalid Reality target")
	}
	if !keyPattern.MatchString(r.Private) || !keyPattern.MatchString(r.Public) {
		return errors.New("invalid X25519 keypair")
	}
	if !hexID.MatchString(r.ShortID) {
		return errors.New("invalid Reality short ID")
	}
	return nil
}
func GenerateKeys(ctx context.Context, binary string) (private, public, shortID string, err error) {
	c, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, e := exec.CommandContext(c, binary, "x25519").CombinedOutput()
	if e != nil {
		return "", "", "", fmt.Errorf("xray x25519 failed: %w", e)
	}
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.TrimSpace(parts[1])
		switch name {
		case "privatekey", "private key":
			private = v
		case "password", "publickey", "public key":
			public = v
		}
	}
	if !keyPattern.MatchString(private) || !keyPattern.MatchString(public) {
		return "", "", "", errors.New("unexpected output from xray x25519")
	}
	random := make([]byte, 8)
	if _, err = rand.Read(random); err != nil {
		return "", "", "", err
	}
	return private, public, hex.EncodeToString(random), nil
}
func Generate(r store.Reality, users []store.User) ([]byte, error) {
	if err := Validate(r); err != nil {
		return nil, err
	}
	clients := make([]map[string]any, 0, len(users))
	for _, u := range users {
		if !u.Enabled {
			continue
		}
		clients = append(clients, map[string]any{"id": u.UUID, "flow": "xtls-rprx-vision", "email": fmt.Sprintf("user-%d@local", u.ID)})
	}
	root := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []any{map[string]any{
			"listen": "0.0.0.0", "port": r.Port, "protocol": "vless", "tag": "vless-reality",
			"settings":       map[string]any{"clients": clients, "decryption": "none"},
			"streamSettings": map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{"show": false, "target": net.JoinHostPort(r.Target, "443"), "serverNames": []string{r.SNI}, "privateKey": r.Private, "shortIds": []string{r.ShortID}}},
			"sniffing":       map[string]any{"enabled": true, "destOverride": []string{"http", "tls"}, "routeOnly": true},
		}},
		"outbounds": []any{map[string]any{"protocol": "freedom", "tag": "direct"}, map[string]any{"protocol": "blackhole", "tag": "block"}},
	}
	return json.MarshalIndent(root, "", "  ")
}
func Link(r store.Reality, u store.User) string {
	q := url.Values{"encryption": {"none"}, "security": {"reality"}, "sni": {r.SNI}, "fp": {"chrome"}, "pbk": {r.Public}, "sid": {r.ShortID}, "type": {"tcp"}, "flow": {"xtls-rprx-vision"}}
	return fmt.Sprintf("vless://%s@%s?%s#%s", u.UUID, net.JoinHostPort(r.Address, strconv.Itoa(r.Port)), q.Encode(), url.QueryEscape(u.Name))
}

type Controller struct {
	Binary        string
	Path          string
	Service       string
	SkipRestart   bool
	ControlSocket string
}

// GenerateWithProfiles keeps the existing Reality TCP listener and adds
// independent XHTTP listeners to the same Xray process.
func GenerateWithProfiles(r store.Reality, users []store.User, profiles []store.Protocol) ([]byte, error) {
	base, err := Generate(r, users)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(base, &root); err != nil {
		return nil, err
	}
	inbounds := root["inbounds"].([]any)
	for _, p := range profiles {
		if p.Kind != "vless-xhttp" || !p.Enabled {
			continue
		}
		if p.Port <= 0 || p.Port > 65535 || !sniPattern.MatchString(p.SNI) || !sniPattern.MatchString(p.Target) || !keyPattern.MatchString(p.PrivateKey) || !hexID.MatchString(p.ShortID) || !strings.HasPrefix(p.Path, "/") {
			return nil, errors.New("invalid XHTTP profile")
		}
		clients := make([]map[string]any, 0, len(users))
		for _, u := range users {
			if u.Enabled {
				clients = append(clients, map[string]any{"id": u.UUID, "email": fmt.Sprintf("xhttp-user-%d@local", u.ID)})
			}
		}
		inbounds = append(inbounds, map[string]any{
			"listen": "0.0.0.0", "port": p.Port, "protocol": "vless", "tag": "vless-xhttp-reality",
			"settings": map[string]any{"clients": clients, "decryption": "none"},
			"streamSettings": map[string]any{"network": "xhttp", "security": "reality",
				"xhttpSettings":   map[string]any{"path": p.Path, "mode": "auto"},
				"realitySettings": map[string]any{"show": false, "target": net.JoinHostPort(p.Target, "443"), "serverNames": []string{p.SNI}, "privateKey": p.PrivateKey, "shortIds": []string{p.ShortID}}},
		})
	}
	root["inbounds"] = inbounds
	return json.MarshalIndent(root, "", "  ")
}

func (c Controller) ApplyWithProfiles(ctx context.Context, r store.Reality, users []store.User, profiles []store.Protocol) error {
	data, err := GenerateWithProfiles(r, users, profiles)
	if err != nil {
		return err
	}
	return c.apply(ctx, data)
}

func (c Controller) Apply(ctx context.Context, r store.Reality, users []store.User) error {
	data, err := Generate(r, users)
	if err != nil {
		return err
	}
	return c.apply(ctx, data)
}

// Snapshot captures the current Xray configuration. Callers use it to roll
// back changes if an auxiliary VPN service fails after Xray is applied.
func (c Controller) Snapshot() ([]byte, error) {
	data, err := os.ReadFile(c.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// Restore reverts to a previously captured, already validated Xray config.
// It does not accept an arbitrary file path or untrusted input from an API.
func (c Controller) Restore(ctx context.Context, previous []byte) error {
	if len(previous) != 0 {
		return c.apply(ctx, previous)
	}
	// The prior state had no Xray config, typically a failed first-time setup.
	// Remove the newly applied config and stop the newly started service.
	if err := os.Remove(c.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if c.SkipRestart {
		return nil
	}
	_, err := c.control(ctx, "stop")
	return err
}

func (c Controller) apply(ctx context.Context, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(c.Path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.Path), ".xray-*.json")
	if err != nil {
		return err
	}
	tempPath := tmp.Name()
	defer os.Remove(tempPath)
	if err = tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	// Verify BEFORE modifying the active configuration.
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(testCtx, c.Binary, "run", "-test", "-config", tempPath)
	_, e := cmd.CombinedOutput()
	if e != nil {
		// Xray errors may include the offending JSON value or private keys.
		return fmt.Errorf("Xray config validation failed: %w", e)
	}
	previous, readErr := os.ReadFile(c.Path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	if readErr == nil && bytes.Equal(previous, data) {
		return nil
	}
	if err = os.Rename(tempPath, c.Path); err != nil {
		return err
	}
	if dir, err := os.Open(filepath.Dir(c.Path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	if c.SkipRestart {
		return nil
	}
	restartCtx, stop := context.WithTimeout(ctx, 15*time.Second)
	defer stop()
	res, err := c.restart(restartCtx)
	if err == nil {
		return nil
	}
	// Roll back atomically. Writing directly to the active file can leave a
	// truncated config if the filesystem fails mid-write.
	var fileErr error
	if readErr == nil {
		fileErr = writeAtomicConfig(c.Path, previous)
	} else {
		fileErr = os.Remove(c.Path)
		if errors.Is(fileErr, os.ErrNotExist) {
			fileErr = nil
		}
	}
	if fileErr != nil {
		return fmt.Errorf("Xray restart failed (%v) and previous config could not be restored: %w", err, fileErr)
	}
	fallbackCtx, fallbackCancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer fallbackCancel()
	var recovery []byte
	var recErr error
	if readErr == nil {
		recovery, recErr = c.restart(fallbackCtx)
	} else {
		recovery, recErr = c.control(fallbackCtx, "stop")
	}
	if recErr != nil {
		return fmt.Errorf("Xray restart failed (%v); rollback service operation also failed (%s): %w", err, sanitize(recovery), recErr)
	}
	return fmt.Errorf("Xray restart failed; previous state restored: %s: %w", sanitize(res), err)
}

// writeAtomicConfig replaces a secret-bearing configuration with mode 0600.
func writeAtomicConfig(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".xray-rollback-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func sanitize(b []byte) string {
	if len(b) > 600 {
		b = b[:600]
	}
	return strings.ReplaceAll(strings.TrimSpace(string(b)), "\x00", "")
}

func (c Controller) restart(ctx context.Context) ([]byte, error) {
	return c.control(ctx, "restart")
}

func (c Controller) control(ctx context.Context, action string) ([]byte, error) {
	if action != "restart" && action != "stop" {
		return nil, errors.New("invalid Xray control action")
	}
	if c.ControlSocket != "" {
		d := net.Dialer{Timeout: 3 * time.Second}
		conn, err := d.DialContext(ctx, "unix", c.ControlSocket)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		deadline := time.Now().Add(12 * time.Second)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		if err := conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
		if _, err = conn.Write([]byte(action + "\n")); err != nil {
			return nil, err
		}
		reply, err := bufio.NewReader(io.LimitReader(conn, 128)).ReadString('\n')
		if err != nil {
			return nil, err
		}
		if reply != "ok\n" {
			return []byte(reply), errors.New("privileged restart helper failed")
		}
		return []byte(reply), nil
	}
	return exec.CommandContext(ctx, "systemctl", action, c.Service).CombinedOutput()
}
