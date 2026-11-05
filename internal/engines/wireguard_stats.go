package engines

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
)

type WireGuardReceipt struct {
	Epoch     string `json:"epoch"`
	StartedAt int64  `json:"started_at"`
	Index     int    `json:"index"`
	Boot      string `json:"boot"`
	ConfigSHA string `json:"config_sha"`
}
type WireGuardTransfer struct {
	Public   string `json:"public_key"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
}
type WireGuardReport struct {
	WireGuardReceipt
	Public string              `json:"public_key"`
	Peers  []WireGuardTransfer `json:"peers"`
}

func wireGuardDigest(blob []byte) string {
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}

func rootWireGuardFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > limit {
		return nil, errors.New("private WireGuard file unavailable")
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 {
		return nil, errors.New("WireGuard file is not root-owned")
	}
	return os.ReadFile(path)
}
func stampWireGuardEpoch(dir, target string) error {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
		return errors.New("untrusted WireGuard runtime directory")
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 {
		return errors.New("WireGuard runtime directory is not root-owned")
	}
	iface, err := net.InterfaceByName("arcwg")
	if err != nil {
		return err
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return err
	}
	config, err := rootWireGuardFile(target, 1<<20)
	if err != nil {
		return err
	}
	nonce, err := auth.Secret(18)
	if err != nil {
		return err
	}
	receipt := WireGuardReceipt{Epoch: strings.TrimSpace(string(boot)) + "/" + strconv.Itoa(iface.Index) + "/" + nonce, StartedAt: time.Now().UnixNano(), Index: iface.Index, Boot: strings.TrimSpace(string(boot)), ConfigSHA: wireGuardDigest(config)}
	blob, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".wireguard-epoch-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err = file.Write(blob); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(dir, "wireguard-epoch.json"))
}

func ParseWireGuardTransfers(blob []byte) ([]WireGuardTransfer, error) {
	if len(blob) > 8<<20 {
		return nil, usage.ErrInvalid
	}
	seen := map[string]bool{}
	result := []WireGuardTransfer{}
	for _, line := range strings.Split(strings.TrimSpace(string(blob)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 || !quickKey(fields[0]) || seen[fields[0]] || len(result) >= 20000 {
			return nil, usage.ErrInvalid
		}
		up, e1 := strconv.ParseInt(fields[1], 10, 64)
		down, e2 := strconv.ParseInt(fields[2], 10, 64)
		if e1 != nil || e2 != nil || up < 0 || down < 0 || up > usage.MaxBytes || down > usage.MaxBytes-up {
			return nil, usage.ErrInvalid
		}
		seen[fields[0]] = true
		result = append(result, WireGuardTransfer{fields[0], up, down})
	}
	return result, nil
}
func checkedWireGuardReport(receipt WireGuardReceipt, config, public, transfer []byte) (WireGuardReport, error) {
	report := WireGuardReport{WireGuardReceipt: receipt, Public: strings.TrimSpace(string(public))}
	if len(receipt.Epoch) < 1 || len(receipt.Epoch) > 128 || receipt.StartedAt < 1 || receipt.Index < 1 || wireGuardDigest(config) != receipt.ConfigSHA || ValidateQuickServer("wireguard", config) != nil || !quickKey(report.Public) {
		return report, usage.ErrInvalid
	}
	expected := map[string]bool{}
	private := ""
	section := ""
	for _, line := range strings.Split(string(config), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if section == "[Interface]" && key == "PrivateKey" {
			private = value
		}
		if section == "[Peer]" && key == "PublicKey" {
			if expected[value] {
				return report, usage.ErrInvalid
			}
			expected[value] = true
		}
	}
	raw, err := base64.StdEncoding.DecodeString(private)
	if err != nil {
		return report, usage.ErrInvalid
	}
	key, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil || base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()) != report.Public {
		return report, usage.ErrInvalid
	}
	report.Peers, err = ParseWireGuardTransfers(transfer)
	if err != nil || len(report.Peers) != len(expected) {
		return report, usage.ErrInvalid
	}
	for _, peer := range report.Peers {
		if !expected[peer.Public] {
			return report, usage.ErrInvalid
		}
	}
	return report, nil
}

type wireGuardOutput struct{ bytes.Buffer }

func (b *wireGuardOutput) Write(blob []byte) (int, error) {
	if b.Len()+len(blob) > 8<<20 {
		return 0, errors.New("WireGuard output limit")
	}
	return b.Buffer.Write(blob)
}
func readWireGuardAttribute(ctx context.Context, attribute string) ([]byte, error) {
	if attribute != "public-key" && attribute != "transfer" {
		return nil, usage.ErrInvalid
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/wg", "show", "arcwg", attribute)
	var output wireGuardOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return nil, errors.New("WireGuard observation unavailable")
	}
	return output.Bytes(), nil
}

// Called only by the authenticated root helper. Neither argv nor output can
// contain private/preshared keys; interface and attributes are fixed allowlists.
func ReadWireGuardStats(ctx context.Context) (WireGuardReport, error) {
	var empty WireGuardReport
	if os.Geteuid() != 0 {
		return empty, errors.New("root-only WireGuard statistics")
	}
	path := "/run/arcvpn/wireguard-epoch.json"
	blob, err := rootWireGuardFile(path, 4096)
	if err != nil {
		return empty, err
	}
	var receipt WireGuardReceipt
	if json.Unmarshal(blob, &receipt) != nil {
		return empty, usage.ErrInvalid
	}
	iface, err := net.InterfaceByName("arcwg")
	if err != nil || iface.Index != receipt.Index {
		return empty, errors.New("WireGuard interface epoch mismatch")
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil || strings.TrimSpace(string(boot)) != receipt.Boot {
		return empty, errors.New("WireGuard boot epoch mismatch")
	}
	config, err := rootWireGuardFile("/run/arcvpn/arcwg.conf", 1<<20)
	if err != nil {
		return empty, err
	}
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	public, err := readWireGuardAttribute(call, "public-key")
	if err != nil {
		return empty, err
	}
	transfer, err := readWireGuardAttribute(call, "transfer")
	if err != nil {
		return empty, err
	}
	report, err := checkedWireGuardReport(receipt, config, public, transfer)
	if err != nil {
		return empty, err
	}
	after, err := rootWireGuardFile(path, 4096)
	if err != nil || !bytes.Equal(blob, after) {
		return empty, errors.New("WireGuard changed during observation")
	}
	iface, err = net.InterfaceByName("arcwg")
	if err != nil || iface.Index != receipt.Index {
		return empty, errors.New("WireGuard restarted during observation")
	}
	return report, nil
}

func (m Manager) WireGuardStatistics(ctx context.Context, users []store.User, credentials []store.ProtocolCredential) ([]usage.Counter, int64, error) {
	socket, err := os.Lstat(m.HelperSocket)
	if err != nil || socket.Mode()&os.ModeSocket == 0 {
		return nil, 0, errors.New("WireGuard helper unavailable")
	}
	owner, ok := socket.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 {
		return nil, 0, errors.New("WireGuard helper is not root-owned")
	}
	dir, err := os.Lstat(filepath.Dir(m.HelperSocket))
	if err != nil || !dir.IsDir() || dir.Mode().Perm()&0022 != 0 {
		return nil, 0, errors.New("untrusted WireGuard helper directory")
	}
	owner, ok = dir.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 {
		return nil, 0, errors.New("untrusted WireGuard helper owner")
	}
	call, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(call, "unix", m.HelperSocket)
	if err != nil {
		return nil, 0, errors.New("WireGuard helper unavailable")
	}
	defer conn.Close()
	deadline, _ := call.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, 0, err
	}
	if _, err := io.WriteString(conn, "stats wireguard\n"); err != nil {
		return nil, 0, err
	}
	blob, err := io.ReadAll(io.LimitReader(conn, (8<<20)+1))
	if err != nil || len(blob) > 8<<20 {
		return nil, 0, errors.New("WireGuard report unavailable")
	}
	var report WireGuardReport
	decoder := json.NewDecoder(bytes.NewReader(blob))
	if decoder.Decode(&report) != nil {
		return nil, 0, usage.ErrInvalid
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, 0, usage.ErrInvalid
	}
	path := filepath.Join(m.Dir, "arcwg.conf")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
		return nil, 0, errors.New("WireGuard configuration unavailable")
	}
	config, err := os.ReadFile(path)
	if err != nil || wireGuardDigest(config) != report.ConfigSHA {
		return nil, 0, errors.New("WireGuard configuration revision mismatch")
	}
	now := time.Now().Unix()
	result, err := wireGuardCounters(report, users, credentials, now)
	return result, now, err
}
func wireGuardCounters(report WireGuardReport, users []store.User, credentials []store.ProtocolCredential, at int64) ([]usage.Counter, error) {
	if !quickKey(report.Public) || len(report.Peers) > 20000 {
		return nil, usage.ErrInvalid
	}
	index := map[int64]int{}
	aliases := map[string]int64{}
	result := []usage.Counter{}
	for _, u := range users {
		index[u.ID] = len(result)
		result = append(result, usage.Counter{UserID: u.ID, Identity: u.UUID, Stream: protocols.WireGuard, Epoch: report.Epoch, StartedAt: report.StartedAt, ObservedAt: at})
	}
	for _, peer := range protocols.WireGuardPeers(users, credentials) {
		if previous, ok := aliases[peer.Public]; ok && previous != peer.UserID {
			return nil, usage.ErrInvalid
		}
		aliases[peer.Public] = peer.UserID
	}
	seen := map[string]bool{}
	for _, peer := range report.Peers {
		if !quickKey(peer.Public) || seen[peer.Public] || peer.Upload < 0 || peer.Download < 0 {
			return nil, usage.ErrInvalid
		}
		seen[peer.Public] = true
		id, ok := aliases[peer.Public]
		if !ok {
			continue
		}
		c := &result[index[id]]
		if peer.Upload > usage.MaxBytes-c.Upload || peer.Download > usage.MaxBytes-c.Download {
			return nil, usage.ErrInvalid
		}
		c.Upload += peer.Upload
		c.Download += peer.Download
	}
	for _, c := range result {
		if c.Validate() != nil {
			return nil, usage.ErrInvalid
		}
	}
	return result, nil
}
