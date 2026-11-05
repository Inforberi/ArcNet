// Package nodeprobe performs an external, authenticated VLESS/REALITY data-plane
// check with an independent Xray client. A TCP port scan is not a successful
// probe: success requires an HTTPS 204 response through the VLESS outbound.
package nodeprobe

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/nodebootstrap"
)

const (
	ProbeURL    = "https://www.gstatic.com/generate_204"
	MaxDuration = 35 * time.Second
)

var (
	uuidPattern   = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	domainPattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)
	shortPattern  = regexp.MustCompile(`(?i)^[0-9a-f]{16}$`)
)

type Link struct {
	Address   string
	Port      int
	UUID      string
	SNI       string
	PublicKey string
	ShortID   string
}

// ParseLink accepts only the exact TCP+REALITY+Vision direct-link profile used
// by ArcVPN. Restricting the target to public IP literals prevents the probe
// runner from being repurposed as an internal-network connection tool.
func ParseLink(raw string) (Link, error) {
	if len(raw) > 2048 {
		return Link{}, errors.New("VLESS link too long")
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "vless" || u.User == nil || u.User.String() == "" || u.Opaque != "" || u.Host == "" || u.Path != "" {
		return Link{}, errors.New("invalid VLESS link")
	}
	if _, password := u.User.Password(); password {
		return Link{}, errors.New("VLESS URI must not contain a password")
	}
	id := u.User.Username()
	if !uuidPattern.MatchString(id) {
		return Link{}, errors.New("invalid VLESS user ID")
	}
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || !nodebootstrap.PublicIP(ip) {
		return Link{}, errors.New("VLESS target must be a public IP")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return Link{}, errors.New("invalid VLESS port")
	}
	q := u.Query()
	allowed := map[string]bool{"encryption": true, "security": true, "sni": true, "fp": true, "pbk": true, "sid": true, "type": true, "flow": true}
	for key, values := range q {
		if !allowed[key] || len(values) != 1 {
			return Link{}, errors.New("unsupported or repeated VLESS parameter")
		}
	}
	for key, value := range map[string]string{"encryption": "none", "security": "reality", "fp": "chrome", "type": "tcp", "flow": "xtls-rprx-vision"} {
		if q.Get(key) != value {
			return Link{}, fmt.Errorf("unsupported VLESS %s", key)
		}
	}
	sni, key, short := q.Get("sni"), q.Get("pbk"), q.Get("sid")
	if !domainPattern.MatchString(sni) || strings.Contains(sni, "..") || !shortPattern.MatchString(short) {
		return Link{}, errors.New("invalid REALITY hostname or short ID")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil || len(decoded) != 32 {
		return Link{}, errors.New("invalid REALITY public key")
	}
	return Link{Address: ip.String(), Port: port, UUID: id, SNI: sni, PublicKey: key, ShortID: short}, nil
}

// Config produces an outbound with NO direct/freedom fallback. If the VPN
// fails, a probe HTTPS request cannot silently take the local network route.
func Config(link Link, localPort int, username, password string) ([]byte, error) {
	if localPort < 1 || localPort > 65535 || username == "" || password == "" {
		return nil, errors.New("invalid probe listener")
	}
	return json.Marshal(map[string]any{
		"log": map[string]string{"loglevel": "error"},
		"inbounds": []any{
			map[string]any{
				"listen": "127.0.0.1", "port": localPort, "protocol": "socks", "tag": "probe-in",
				"settings": map[string]any{"auth": "password", "users": []any{map[string]string{"user": username, "pass": password}}, "udp": false},
			},
		},
		"outbounds": []any{
			map[string]any{
				"tag": "vpn", "protocol": "vless",
				"settings": map[string]any{
					"address": link.Address, "port": link.Port, "id": link.UUID, "encryption": "none", "flow": "xtls-rprx-vision",
				},
				"streamSettings": map[string]any{
					"network": "tcp", "security": "reality",
					"realitySettings": map[string]any{"serverName": link.SNI, "fingerprint": "chrome", "password": link.PublicKey, "shortId": link.ShortID},
				},
			},
		},
		"routing": map[string]any{"domainStrategy": "AsIs", "rules": []any{map[string]any{"type": "field", "inboundTag": []string{"probe-in"}, "outboundTag": "vpn"}}},
	})
}

// Result never includes the client UUID, REALITY key, or full VLESS link.
type Result struct {
	Result     string `json:"result"`
	DurationMS int64  `json:"duration_ms"`
	CheckedAt  int64  `json:"checked_at"`
	Evidence   string `json:"evidence"`
}

func randomCredential() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func Run(ctx context.Context, binary, rawLink string) (Result, error) {
	link, err := ParseLink(rawLink)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(binary) == "" {
		return Result{}, errors.New("xray binary path required")
	}
	username, err := randomCredential()
	if err != nil {
		return Result{}, err
	}
	password, err := randomCredential()
	if err != nil {
		return Result{}, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Result{}, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	data, err := Config(link, port, username, password)
	if err != nil {
		return Result{}, err
	}
	dir, err := os.MkdirTemp("", "arcvpn-vpn-probe-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir)
	configPath := filepath.Join(dir, "client.json")
	if err = os.WriteFile(configPath, data, 0600); err != nil {
		return Result{}, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, MaxDuration)
	defer cancel()
	validate := exec.CommandContext(probeCtx, binary, "run", "-test", "-config", configPath)
	validate.Stdout = io.Discard
	validate.Stderr = io.Discard
	if err = validate.Run(); err != nil {
		return Result{}, errors.New("Xray rejected probe configuration")
	}
	command := exec.CommandContext(probeCtx, binary, "run", "-config", configPath)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err = command.Start(); err != nil {
		return Result{}, errors.New("unable to start Xray probe client")
	}
	done := make(chan struct{})
	go func() { _ = command.Wait(); close(done) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = command.Process.Kill()
			<-done
		}
	}()
	localAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	ready := false
	for !ready {
		select {
		case <-probeCtx.Done():
			return Result{}, errors.New("Xray probe client startup timeout")
		case <-done:
			return Result{}, errors.New("Xray probe client exited before listening")
		default:
		}
		c, e := net.DialTimeout("tcp", localAddr, 150*time.Millisecond)
		if e == nil {
			c.Close()
			ready = true
			break
		}
		select {
		case <-probeCtx.Done():
			return Result{}, errors.New("Xray probe client startup timeout")
		case <-time.After(80 * time.Millisecond):
		}
	}
	start := time.Now()
	dialer := &socksDialer{address: localAddr, username: username, password: password}
	transport := &http.Transport{Proxy: nil, DialContext: dialer.DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 12 * time.Second, DisableKeepAlives: true, MaxResponseHeaderBytes: 8 << 10}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, ProbeURL, nil)
	if err != nil {
		return Result{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, errors.New("HTTPS request through VLESS/REALITY failed")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode != http.StatusNoContent {
		return Result{}, fmt.Errorf("VPN HTTPS target returned HTTP %d (expected 204)", resp.StatusCode)
	}
	return Result{Result: "pass", DurationMS: time.Since(start).Milliseconds(), CheckedAt: time.Now().Unix(), Evidence: "HTTPS 204 through authenticated local SOCKS5 -> Xray VLESS/REALITY; TLS verified"}, nil
}
