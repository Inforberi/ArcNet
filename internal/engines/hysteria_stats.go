package engines

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
)

const HysteriaStatsPort = 19823
const hysteriaStatsAddress = "127.0.0.1:19823"

var statsSecretPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
var invocationPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type hysteriaConfig struct {
	Traffic struct {
		Listen string `json:"listen"`
		Secret string `json:"secret"`
	} `json:"trafficStats"`
	Auth struct {
		Type  string            `json:"type"`
		Users map[string]string `json:"userpass"`
	} `json:"auth"`
}

func (m Manager) withHysteriaStatistics(data []byte) ([]byte, error) {
	name := filepath.Join(m.Dir, "hysteria-stats.key")
	info, err := os.Lstat(name)
	if os.IsNotExist(err) {
		secret, e := auth.Secret(32)
		if e != nil {
			return nil, e
		}
		f, e := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return nil, e
		}
		_, e = f.WriteString(secret)
		closeErr := f.Close()
		if e != nil {
			return nil, e
		}
		if closeErr != nil {
			return nil, closeErr
		}
		info, err = os.Lstat(name)
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("Hysteria statistics key is not private")
	}
	secret, err := os.ReadFile(name)
	if err != nil || !statsSecretPattern.Match(secret) {
		return nil, errors.New("Hysteria statistics key unavailable")
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	var config hysteriaConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	// Hysteria rejects an empty userpass map. Keep the listener healthy with
	// a private, unissued 256-bit credential when no real access is allowed.
	// Domain separation keeps this credential distinct from the Stats API key;
	// determinism avoids restarting the daemon on every accounting tick.
	if config.Auth.Type == "userpass" && len(config.Auth.Users) == 0 {
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write([]byte("arcnet:hysteria:empty-roster:v1"))
		root["auth"] = map[string]any{"type": "userpass", "userpass": map[string]string{
			"_arcnet_no_users": base64.RawURLEncoding.EncodeToString(mac.Sum(nil)),
		}}
	}
	root["trafficStats"] = map[string]string{"listen": hysteriaStatsAddress, "secret": string(secret)}
	return json.MarshalIndent(root, "", "  ")
}

type hysteriaProcess struct {
	PID       int
	Epoch     string
	StartedAt int64
}

func parseHysteriaProcess(blob []byte) (hysteriaProcess, error) {
	var p hysteriaProcess
	for _, line := range strings.Split(string(blob), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "MainPID":
			p.PID, _ = strconv.Atoi(value)
		case "InvocationID":
			p.Epoch = value
		case "ExecMainStartTimestamp":
			if at, err := time.Parse("Mon 2006-01-02 15:04:05.000000 MST", value); err == nil {
				p.StartedAt = at.UnixNano()
			}
		}
	}
	if p.PID < 1 || !invocationPattern.MatchString(p.Epoch) || p.StartedAt < 1 {
		return p, errors.New("Hysteria process identity unavailable")
	}
	return p, nil
}
func hysteriaServiceProcess(ctx context.Context) (hysteriaProcess, error) {
	call, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(call, "/usr/bin/systemctl", "show", "arcvpn-hysteria.service", "--property=MainPID,InvocationID,ExecMainStartTimestamp", "--timestamp=us+utc")
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	blob, err := cmd.Output()
	if err != nil || len(blob) > 4096 {
		return hysteriaProcess{}, errors.New("Hysteria service identity unavailable")
	}
	return parseHysteriaProcess(blob)
}

// Authenticate the listener BEFORE sending its HTTP secret. Unlike a bare
// loopback URL, an inode held by systemd's actual MainPID cannot be supplied by
// an unrelated local service that happens to occupy the reserved port.
func verifyHysteriaListener(p hysteriaProcess) error {
	status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", p.PID))
	if err != nil {
		return errors.New("Hysteria process unavailable")
	}
	uidOK := false
	for _, line := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(line, "Uid:") {
			f := strings.Fields(line)
			uidOK = len(f) == 5 && f[2] == strconv.Itoa(os.Geteuid())
		}
	}
	if !uidOK {
		return errors.New("Hysteria process owner mismatch")
	}
	f, err := os.Open(fmt.Sprintf("/proc/%d/net/tcp", p.PID))
	if err != nil {
		return errors.New("Hysteria listener unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	f.Close()
	if err != nil || len(data) > 16<<20 {
		return errors.New("Hysteria listener table unavailable")
	}
	inode := ""
	address := fmt.Sprintf("0100007F:%04X", HysteriaStatsPort)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 10 && fields[1] == address && fields[3] == "0A" {
			inode = fields[9]
			break
		}
	}
	if inode == "" {
		return errors.New("Hysteria private listener not found")
	}
	dir, err := os.Open(fmt.Sprintf("/proc/%d/fd", p.PID))
	if err != nil {
		return errors.New("Hysteria descriptors unavailable")
	}
	defer dir.Close()
	for n := 0; n < 65536; n += 256 {
		entries, err := dir.ReadDir(256)
		for _, entry := range entries {
			target, e := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", p.PID, entry.Name()))
			if e == nil && target == "socket:["+inode+"]" {
				return nil
			}
		}
		if err != nil {
			break
		}
	}
	return errors.New("statistics listener does not belong to Hysteria")
}

func ParseHysteriaStatistics(blob []byte, users []store.User, credentials []store.ProtocolCredential, p hysteriaProcess, observed int64) ([]usage.Counter, error) {
	if len(blob) > 8<<20 {
		return nil, usage.ErrInvalid
	}
	var stats map[string]struct {
		Upload   int64 `json:"tx"`
		Download int64 `json:"rx"`
	}
	decoder := json.NewDecoder(bytes.NewReader(blob))
	if decoder.Decode(&stats) != nil || stats == nil || len(stats) > 20000 {
		return nil, usage.ErrInvalid
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, usage.ErrInvalid
	}
	index := map[int64]int{}
	result := make([]usage.Counter, 0, len(users))
	for _, u := range users {
		index[u.ID] = len(result)
		result = append(result, usage.Counter{UserID: u.ID, Identity: u.UUID, Stream: protocols.Hysteria2, Epoch: p.Epoch, StartedAt: p.StartedAt, ObservedAt: observed})
	}
	for _, id := range protocols.HysteriaIdentities(users, credentials) {
		stat, ok := stats[id.Name]
		if !ok {
			continue
		}
		i := index[id.UserID]
		if stat.Upload < 0 || stat.Download < 0 || stat.Upload > usage.MaxBytes-result[i].Upload || stat.Download > usage.MaxBytes-result[i].Download {
			return nil, usage.ErrInvalid
		}
		result[i].Upload += stat.Upload
		result[i].Download += stat.Download
	}
	for _, c := range result {
		if c.Validate() != nil {
			return nil, usage.ErrInvalid
		}
	}
	return result, nil
}

func (m Manager) HysteriaStatistics(ctx context.Context, users []store.User, credentials []store.ProtocolCredential) ([]usage.Counter, int64, error) {
	p, err := hysteriaServiceProcess(ctx)
	if err != nil {
		return nil, 0, err
	}
	counters, at, err := m.hysteriaStatistics(ctx, users, credentials, p)
	if err != nil {
		return nil, 0, err
	}
	after, err := hysteriaServiceProcess(ctx)
	if err != nil || p != after {
		return nil, 0, errors.New("Hysteria restarted during statistics query")
	}
	return counters, at, nil
}

func (m Manager) hysteriaStatistics(ctx context.Context, users []store.User, credentials []store.ProtocolCredential, p hysteriaProcess) ([]usage.Counter, int64, error) {
	data, at, err := m.hysteriaRead(ctx, users, credentials, p, "/traffic?clear=false")
	if err != nil {
		return nil, 0, err
	}
	counters, err := ParseHysteriaStatistics(data, users, credentials, p, at)
	return counters, at, err
}

func (m Manager) hysteriaRead(ctx context.Context, users []store.User, credentials []store.ProtocolCredential, p hysteriaProcess, endpoint string) ([]byte, int64, error) {
	if endpoint != "/traffic?clear=false" && endpoint != "/online" {
		return nil, 0, usage.ErrInvalid
	}
	name := filepath.Join(m.Dir, "hysteria.yaml")
	dir, err := os.Lstat(m.Dir)
	if err != nil || !dir.IsDir() || dir.Mode().Perm()&0077 != 0 {
		return nil, 0, errors.New("Hysteria directory is not private")
	}
	info, err := os.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 8<<20 {
		return nil, 0, errors.New("Hysteria configuration is not private")
	}
	blob, err := os.ReadFile(name)
	if err != nil {
		return nil, 0, errors.New("Hysteria configuration unavailable")
	}
	var config hysteriaConfig
	if json.Unmarshal(blob, &config) != nil || config.Traffic.Listen != hysteriaStatsAddress || !statsSecretPattern.MatchString(config.Traffic.Secret) || config.Auth.Type != "userpass" {
		return nil, 0, errors.New("Hysteria statistics not configured")
	}
	for _, id := range protocols.HysteriaIdentities(users, credentials) {
		if actual := config.Auth.Users[id.Name]; actual != "" && actual != id.Secret {
			return nil, 0, errors.New("Hysteria credential revision mismatch")
		}
	}
	if err := verifyHysteriaListener(p); err != nil {
		return nil, 0, err
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != hysteriaStatsAddress {
			return nil, errors.New("unexpected statistics address")
		}
		conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp4", address)
		if err != nil {
			return nil, err
		}
		if err := verifyHysteriaListener(p); err != nil {
			conn.Close()
			return nil, err
		}
		return conn, nil
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("statistics redirects forbidden") }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+hysteriaStatsAddress+endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Authorization", config.Traffic.Secret)
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, errors.New("Hysteria statistics unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, 0, errors.New("Hysteria statistics request rejected")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		return nil, 0, errors.New("Hysteria statistics response unavailable")
	}
	after, err := os.ReadFile(name)
	if err != nil || !bytes.Equal(blob, after) {
		return nil, 0, errors.New("Hysteria configuration changed during query")
	}
	return data, time.Now().Unix(), nil
}
