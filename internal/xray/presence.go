package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/arcvpn/arcvpn/internal/releaseversion"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Online APIs in older Xray releases used activity expiration instead of live
// reference counts. Do not interpret their empty lists as confirmed offline.
func QueryOnlineUsers(ctx context.Context, binary, socket string) ([]string, error) {
	call, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var version statsOutput
	cmd := exec.CommandContext(call, binary, "version")
	cmd.Stdout = &version
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return nil, errors.New("Xray presence capability unavailable")
	}
	fields := strings.Fields(version.String())
	if len(fields) < 2 || fields[0] != "Xray" {
		return nil, usage.ErrInvalid
	}
	order, valid := releaseversion.Compare(fields[1], "26.10.31")
	if !valid || order < 0 {
		return nil, errors.New("Xray live presence unsupported")
	}
	var output statsOutput
	cmd = exec.CommandContext(call, binary, "api", "statsgetallonlineusers", "--server=unix://"+socket, "-timeout=3")
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return nil, errors.New("Xray online query unavailable")
	}
	return ParseOnlineUsers(output.Bytes())
}
func ParseOnlineUsers(data []byte) ([]string, error) {
	if len(data) > 8<<20 {
		return nil, usage.ErrInvalid
	}
	var response struct {
		Users []string `json:"users"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&response) != nil || d.Decode(new(any)) != io.EOF || len(response.Users) > 40000 {
		return nil, usage.ErrInvalid
	}
	seen := map[string]bool{}
	for i, name := range response.Users {
		if strings.Contains(name, ">>>") {
			parts := strings.Split(name, ">>>")
			if len(parts) != 3 || parts[0] != "user" || parts[2] != "online" {
				return nil, usage.ErrInvalid
			}
			name = parts[1]
			response.Users[i] = name
		}
		if len(name) > 160 || name == "" || seen[name] {
			return nil, usage.ErrInvalid
		}
		seen[name] = true
	}
	return response.Users, nil
}
func (c Controller) Presence(ctx context.Context, users []store.User) ([]usage.PresenceObservation, error) {
	epoch, _, err := socketEpoch(c.StatisticsSocket())
	if err != nil {
		return nil, err
	}
	config, err := c.Snapshot()
	if err != nil {
		return nil, err
	}
	var active struct {
		Policy struct {
			Levels map[string]struct {
				Online bool `json:"statsUserOnline"`
			} `json:"levels"`
		} `json:"policy"`
		Inbounds []struct {
			Settings struct {
				Clients []struct {
					ID    string `json:"id"`
					Email string `json:"email"`
				} `json:"clients"`
			} `json:"settings"`
		} `json:"inbounds"`
	}
	if json.Unmarshal(config, &active) != nil || !active.Policy.Levels["0"].Online {
		return nil, errors.New("live presence is not configured")
	}
	type owner struct {
		user       store.User
		credential string
	}
	roster := map[string]owner{}
	for _, user := range users {
		for _, prefix := range []string{"user-", "xhttp-user-"} {
			roster[prefix+strconv.FormatInt(user.ID, 10)+"@local"] = owner{user, user.UUID}
			if user.DeviceAccess != nil {
				for _, device := range user.DeviceAccess.Devices {
					roster[prefix+"device-"+device.ID+"@local"] = owner{user, device.UUID}
				}
			}
		}
	}
	configured := map[string]owner{}
	for _, inbound := range active.Inbounds {
		for _, client := range inbound.Settings.Clients {
			parent, ok := roster[client.Email]
			if !ok || parent.credential != client.ID {
				return nil, errors.New("Xray presence credential revision mismatch")
			}
			configured[client.Email] = parent
		}
	}
	online, err := QueryOnlineUsers(ctx, c.Binary, c.StatisticsSocket())
	if err != nil {
		return nil, err
	}
	next, _, err := socketEpoch(c.StatisticsSocket())
	after, readErr := c.Snapshot()
	if err != nil || readErr != nil || epoch != next || !bytes.Equal(config, after) {
		return nil, errors.New("Xray changed during presence query")
	}
	observed := map[int64]bool{}
	for _, name := range online {
		owner, ok := configured[name]
		if !ok {
			return nil, usage.ErrInvalid
		}
		observed[owner.user.ID] = true
	}
	at := time.Now().Unix()
	result := make([]usage.PresenceObservation, 0, len(users))
	for _, user := range users {
		value := observed[user.ID]
		result = append(result, usage.PresenceObservation{UserID: user.ID, Identity: user.UUID, PresenceSource: usage.PresenceSource{Source: "xray", State: "fresh", ObservedAt: at, Epoch: epoch, Online: &value}})
	}
	return result, nil
}
