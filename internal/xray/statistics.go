package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
)

func (c Controller) StatisticsSocket() string {
	return filepath.Join(filepath.Dir(c.Path), "xray-stats.sock")
}

// Only StatsService is exposed on a private filesystem socket, never a TCP
// port. Handler/config mutation APIs remain disabled.
func (c Controller) withStatistics(data []byte) ([]byte, error) {
	if c.Path == "" || !filepath.IsAbs(c.Path) {
		return nil, errors.New("statistics requires an absolute configuration path")
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	root["stats"] = map[string]any{}
	root["api"] = map[string]any{"tag": "arcvpn-stats", "listen": c.StatisticsSocket(), "services": []string{"StatsService"}}
	root["policy"] = map[string]any{"levels": map[string]any{"0": map[string]bool{"statsUserUplink": true, "statsUserDownlink": true}}}
	return json.MarshalIndent(root, "", "  ")
}

type statsOutput struct{ bytes.Buffer }

func (w *statsOutput) Write(p []byte) (int, error) {
	if w.Len()+len(p) > 8<<20 {
		return 0, errors.New("statistics output too large")
	}
	return w.Buffer.Write(p)
}

func socketEpoch(path string) (string, int64, error) {
	dir, err := os.Lstat(filepath.Dir(path))
	if err != nil || !dir.IsDir() || dir.Mode().Perm()&0077 != 0 {
		return "", 0, errors.New("statistics directory is not private")
	}
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSocket == 0 || fi.Mode().Perm()&0077 != 0 {
		return "", 0, errors.New("statistics socket is not private")
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Geteuid() {
		return "", 0, errors.New("statistics socket ownership mismatch")
	}
	started := fi.ModTime().UnixNano()
	return fmt.Sprintf("%d:%d:%d", st.Dev, st.Ino, started), started, nil
}

func ParseStatistics(blob []byte, users []store.User, xhttp bool, epoch string, started, observed int64) ([]usage.Counter, error) {
	var response struct {
		Stat []struct {
			Name  string          `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"stat"`
	}
	decoder := json.NewDecoder(bytes.NewReader(blob))
	if len(blob) > 8<<20 || decoder.Decode(&response) != nil || len(response.Stat) > 40000 {
		return nil, usage.ErrInvalid
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, usage.ErrInvalid
	}
	// Empty counters are zero only after a successful request to our configured
	// StatsService. Errors never become zero observations.
	index := map[string]int{}
	result := make([]usage.Counter, 0, len(users)*2)
	for _, u := range users {
		streams := []string{"vless-reality"}
		if xhttp {
			streams = append(streams, "vless-xhttp")
		}
		for _, stream := range streams {
			prefix := "user-"
			if stream == "vless-xhttp" {
				prefix = "xhttp-user-"
			}
			index[prefix+strconv.FormatInt(u.ID, 10)+"@local"] = len(result)
			if u.DeviceAccess != nil {
				for _, d := range u.DeviceAccess.Devices {
					index[prefix+"device-"+d.ID+"@local"] = len(result)
				}
			}
			result = append(result, usage.Counter{UserID: u.ID, Identity: u.UUID, Stream: stream, Epoch: epoch, StartedAt: started, ObservedAt: observed})
		}
	}
	seen := map[string]bool{}
	for _, stat := range response.Stat {
		parts := strings.Split(stat.Name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
			return nil, usage.ErrInvalid
		}
		i, ok := index[parts[1]]
		if !ok {
			continue
		} // Retired users cannot create charges.
		if seen[stat.Name] {
			return nil, usage.ErrInvalid
		}
		seen[stat.Name] = true
		raw := string(stat.Value)
		if len(raw) > 0 && raw[0] == '"' {
			var value string
			if json.Unmarshal(stat.Value, &value) != nil {
				return nil, usage.ErrInvalid
			}
			raw = value
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			return nil, usage.ErrInvalid
		}
		switch parts[3] {
		case "uplink":
			if n > usage.MaxBytes-result[i].Upload {
				return nil, usage.ErrInvalid
			}
			result[i].Upload += n
		case "downlink":
			if n > usage.MaxBytes-result[i].Download {
				return nil, usage.ErrInvalid
			}
			result[i].Download += n
		default:
			return nil, usage.ErrInvalid
		}
	}
	for _, c := range result {
		if c.Validate() != nil {
			return nil, usage.ErrInvalid
		}
	}
	return result, nil
}

func (c Controller) Statistics(ctx context.Context, users []store.User, xhttp bool) ([]usage.Counter, int64, error) {
	path := c.StatisticsSocket()
	epoch, started, err := socketEpoch(path)
	if err != nil {
		return nil, 0, err
	}
	config, err := c.Snapshot()
	if err != nil || len(config) > 8<<20 {
		return nil, 0, errors.New("statistics configuration unavailable")
	}
	var active struct {
		Inbounds []struct {
			Settings struct {
				Clients []struct {
					ID    string `json:"id"`
					Email string `json:"email"`
				} `json:"clients"`
			} `json:"settings"`
		} `json:"inbounds"`
	}
	if json.Unmarshal(config, &active) != nil {
		return nil, 0, errors.New("statistics configuration invalid")
	}
	identities := map[string]string{}
	for _, in := range active.Inbounds {
		for _, client := range in.Settings.Clients {
			identities[client.Email] = client.ID
		}
	}
	for _, u := range users {
		for _, prefix := range []string{"user-", "xhttp-user-"} {
			if actual := identities[prefix+strconv.FormatInt(u.ID, 10)+"@local"]; actual != "" && actual != u.UUID {
				return nil, 0, errors.New("statistics credential revision mismatch")
			}
			if u.DeviceAccess != nil {
				for _, d := range u.DeviceAccess.Devices {
					if actual := identities[prefix+"device-"+d.ID+"@local"]; actual != "" && actual != d.UUID {
						return nil, 0, errors.New("statistics device revision mismatch")
					}
				}
			}
		}
	}
	call, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(call, c.Binary, "api", "statsquery", "--server=unix://"+path, "-timeout=3", "-pattern=user>>>", "-reset=false")
	var output statsOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return nil, 0, errors.New("Xray statistics unavailable")
	}
	after, _, err := socketEpoch(path)
	if err != nil || epoch != after {
		return nil, 0, errors.New("Xray restarted during statistics query")
	}
	afterConfig, err := c.Snapshot()
	if err != nil || !bytes.Equal(config, afterConfig) {
		return nil, 0, errors.New("configuration changed during statistics query")
	}
	now := time.Now().Unix()
	counters, err := ParseStatistics(output.Bytes(), users, xhttp, epoch, started, now)
	return counters, now, err
}
