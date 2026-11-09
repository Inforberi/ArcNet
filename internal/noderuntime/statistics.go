package noderuntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/arcvpn/arcvpn/internal/usage"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func ValidateStatisticsConfig(data []byte) error {
	if len(data) > MaxRequest-4096 || uniqueJSON(data) != nil {
		return usage.ErrInvalid
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil {
		return usage.ErrInvalid
	}
	base := map[string]json.RawMessage{}
	for k, v := range root {
		if k != "stats" && k != "api" && k != "policy" {
			base[k] = v
		}
	}
	plain, _ := json.Marshal(base)
	if e := ValidateConfig(plain); e != nil {
		return e
	}
	var policy struct {
		Levels map[string]struct {
			Online bool `json:"statsUserOnline"`
		} `json:"levels"`
	}
	if json.Unmarshal(root["policy"], &policy) != nil {
		return usage.ErrInvalid
	}
	expected, e := xray.WithNodeStatistics(plain, nil, policy.Levels["0"].Online)
	if e != nil {
		return e
	}
	var actualValue, expectedValue any
	if json.Unmarshal(data, &actualValue) != nil || json.Unmarshal(expected, &expectedValue) != nil || !reflect.DeepEqual(actualValue, expectedValue) {
		return usage.ErrInvalid
	}
	return nil
}

func ParseNodeStatistics(data []byte, clients map[string]usage.NodeClient) ([]usage.NamedCounter, error) {
	if len(data) > 8<<20 || len(clients) > usage.MaxNodeCounters {
		return nil, usage.ErrInvalid
	}
	var reply struct {
		Stat []struct {
			Name  string          `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"stat"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if d.Decode(&reply) != nil || len(reply.Stat) > usage.MaxNodeCounters*2 {
		return nil, usage.ErrInvalid
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return nil, usage.ErrInvalid
	}
	values := make(map[string]usage.NamedCounter, len(clients))
	for name := range clients {
		values[name] = usage.NamedCounter{Name: name}
	}
	seen := map[string]bool{}
	for _, stat := range reply.Stat {
		parts := strings.Split(stat.Name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" || (parts[3] != "uplink" && parts[3] != "downlink") || seen[stat.Name] {
			return nil, usage.ErrInvalid
		}
		c, ok := values[parts[1]]
		if !ok {
			return nil, usage.ErrInvalid
		}
		seen[stat.Name] = true
		text := string(stat.Value)
		if strings.HasPrefix(text, "\"") {
			if json.Unmarshal(stat.Value, &text) != nil {
				return nil, usage.ErrInvalid
			}
		}
		n, e := strconv.ParseInt(text, 10, 64)
		if e != nil || n < 0 || n > usage.MaxBytes {
			return nil, usage.ErrInvalid
		}
		if parts[3] == "uplink" {
			c.Upload = n
		} else {
			c.Download = n
		}
		if c.Download > usage.MaxBytes-c.Upload {
			return nil, usage.ErrInvalid
		}
		values[c.Name] = c
	}
	out := make([]usage.NamedCounter, 0, len(values))
	for _, c := range values {
		out = append(out, c)
	}
	return out, nil
}

// Both request and response omit all private keys and arbitrary process/path
// arguments. Only the fixed socket of the root-managed Xray can be queried.
func (raw Installer) Statistics(ctx context.Context) (*usage.NodeReport, error) {
	report, _, err := raw.statistics(ctx, false)
	return report, err
}
func (raw Installer) Presence(ctx context.Context) (*usage.NodePresenceReport, error) {
	report, clients, err := raw.statistics(ctx, true)
	if err != nil {
		return nil, err
	}
	return &usage.NodePresenceReport{Version: 1, OperationID: report.OperationID, Revision: report.Revision, Digest: report.Digest, Epoch: report.Epoch, StartedAt: report.StartedAt, ObservedAt: report.ObservedAt, Clients: clients}, nil
}
func (raw Installer) statistics(ctx context.Context, online bool) (*usage.NodeReport, []usage.NamedPresence, error) {
	i := raw.defaults()
	config, e := readPrivilegedState(i.ConfigPath, MaxRequest)
	if e != nil || ValidateStatisticsConfig(config) != nil {
		return nil, nil, errors.New("node statistics not configured")
	}
	receipt, e := readPrivilegedState(i.ReceiptPath, 4096)
	if e != nil {
		return nil, nil, e
	}
	var report usage.NodeReport
	var applied struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
		SHA256   string `json:"sha256"`
	}
	if json.Unmarshal(receipt, &applied) != nil || !idPattern.MatchString(applied.ID) || applied.Revision < 1 {
		return nil, nil, usage.ErrInvalid
	}
	hash := sha256.Sum256(config)
	if hex.EncodeToString(hash[:]) != applied.SHA256 {
		return nil, nil, usage.ErrInvalid
	}
	owner, e := user.Lookup("arcvpn-xray")
	if e != nil {
		return nil, nil, e
	}
	uid, e := strconv.Atoi(owner.Uid)
	if e != nil {
		return nil, nil, e
	}
	identity := func() (string, int64, error) {
		dir, e := os.Lstat("/run/arcvpn-node-xray")
		if e != nil || !dir.IsDir() || dir.Mode().Perm() != 0700 {
			return "", 0, usage.ErrInvalid
		}
		fi, e := os.Lstat(xray.NodeStatisticsSocket)
		if e != nil || fi.Mode()&os.ModeSocket == 0 || fi.Mode().Perm()&0077 != 0 {
			return "", 0, usage.ErrInvalid
		}
		ds, ok := dir.Sys().(*syscall.Stat_t)
		if !ok || int(ds.Uid) != uid {
			return "", 0, usage.ErrInvalid
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok || int(st.Uid) != uid {
			return "", 0, usage.ErrInvalid
		}
		started := fi.ModTime().UnixNano()
		return fmt.Sprintf("%d:%d:%d", st.Dev, st.Ino, started), started, nil
	}
	epoch, started, e := identity()
	if e != nil {
		return nil, nil, e
	}
	var blob []byte
	var names []string
	if online {
		if !usage.NodePresenceEnabled(config) {
			return nil, nil, errors.New("node presence is not configured")
		}
		names, e = xray.QueryOnlineUsers(ctx, i.XrayBinary, xray.NodeStatisticsSocket)
	} else {
		blob, e = xray.QueryStatistics(ctx, i.XrayBinary, xray.NodeStatisticsSocket)
	}
	if e != nil {
		return nil, nil, e
	}
	after, _, e := identity()
	if e != nil || after != epoch {
		return nil, nil, usage.ErrInvalid
	}
	afterConfig, e := readPrivilegedState(i.ConfigPath, MaxRequest)
	if e != nil || !bytes.Equal(config, afterConfig) {
		return nil, nil, usage.ErrInvalid
	}
	afterReceipt, e := readPrivilegedState(i.ReceiptPath, 4096)
	if e != nil || !bytes.Equal(receipt, afterReceipt) {
		return nil, nil, usage.ErrInvalid
	}
	clients, e := usage.NodeClients(config)
	if e != nil {
		return nil, nil, e
	}
	var counters []usage.NamedCounter
	var presence []usage.NamedPresence
	if online {
		active := map[string]bool{}
		for _, name := range names {
			if _, ok := clients[name]; !ok {
				return nil, nil, usage.ErrInvalid
			}
			active[name] = true
		}
		for name := range clients {
			value := active[name]
			presence = append(presence, usage.NamedPresence{Name: name, Online: &value})
		}
	} else {
		counters, e = ParseNodeStatistics(blob, clients)
		if e != nil {
			return nil, nil, e
		}
	}
	report = usage.NodeReport{Version: 1, OperationID: applied.ID, Revision: applied.Revision, Digest: applied.SHA256, Epoch: epoch, StartedAt: started, ObservedAt: time.Now().Unix(), Counters: counters}
	return &report, presence, nil
}
