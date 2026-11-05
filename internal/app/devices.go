package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/devices"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func (a *App) xrayUsers(ctx context.Context, users []store.User) ([]store.User, error) {
	state, err := a.Store.DeviceState(ctx)
	if err != nil {
		return nil, err
	}
	records := map[int64]*devices.Record{}
	for i := range state {
		records[state[i].UserID] = &state[i]
	}
	result := append([]store.User{}, users...)
	for i := range result {
		result[i].DeviceAccess = records[result[i].ID]
	}
	return result, nil
}
func (a *App) ChangeUserDevices(ctx context.Context, id, version int64, name, revoke string, limit *int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	name = strings.TrimSpace(name)
	if (limit != nil && (*limit < 0 || *limit > devices.MaxActive)) || (revoke == "" && limit == nil && !devices.ValidName(name)) {
		return devices.ErrInvalid
	}
	var created devices.Device
	if revoke == "" && limit == nil {
		var err error
		created.ID, err = auth.UUID()
		if err != nil {
			return err
		}
		created.UUID, err = auth.UUID()
		if err != nil {
			return err
		}
		created.Token, err = auth.Secret(32)
		if err != nil {
			return err
		}
		created.Name = name
		created.CreatedAt = time.Now().Unix()
	}
	err := a.Store.ChangeDevices(ctx, id, version, func(r *devices.Record) error {
		if limit != nil {
			if *limit > 0 && r.Active() > *limit {
				return devices.ErrLimit
			}
			r.Limit = *limit
			return nil
		}
		if revoke != "" {
			for i, d := range r.Devices {
				if d.ID == revoke {
					if d.RevokedAt == 0 {
						r.Devices[i].RevokedAt = time.Now().Unix()
					}
					return nil
				}
			}
			return store.ErrNotFound
		}
		if len(r.Devices) >= devices.MaxHistory || r.Active() >= devices.MaxActive || (r.Limit > 0 && r.Active() >= r.Limit) {
			return devices.ErrLimit
		}
		r.Devices = append(r.Devices, created)
		return nil
	})
	if err != nil {
		return err
	}
	a.Store.Event(ctx, "user.devices", fmt.Sprintf("ID %d", id))
	// Desired policy stays persisted on failure. The worker retries it; the API
	// never claims that an old live connection has already been terminated.
	_ = a.reconcilePackagesLocked(ctx, true)
	return nil
}

func (a *App) DeviceLinks(ctx context.Context, id int64, deviceID string) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	selected, err := a.authorizedDevice(ctx, id, deviceID)
	if err != nil {
		return nil, err
	}
	reality, err := a.Store.ReadReality(ctx)
	if err != nil {
		return nil, err
	}
	if !reality.Enabled {
		return nil, store.ErrNotFound
	}
	user := store.User{ID: id, Name: selected.Name, UUID: selected.UUID, Enabled: true}
	links := []string{xray.Link(reality, user)}
	profiles, err := a.Store.ListProtocols(ctx)
	if err != nil {
		return nil, err
	}
	extra, err := protocols.Connections(user, profiles, []store.ProtocolCredential{{UserID: id, Kind: protocols.XHTTP, Secret: selected.UUID}})
	if err != nil {
		return nil, err
	}
	for _, c := range extra {
		if c.Format == "uri" {
			links = append(links, c.Link)
		}
	}
	for _, p := range profiles {
		if p.Kind == protocols.Hysteria2 && p.Enabled {
			links = append(links, protocols.HysteriaLink(p, selected.Name, protocols.HysteriaDeviceName(selected.ID), selected.UUID))
		}
	}
	return links, nil
}

func (a *App) authorizedDevice(ctx context.Context, id int64, deviceID string) (devices.Device, error) {
	allowed, _, err := a.Store.UserAccess(ctx, id)
	if err != nil {
		return devices.Device{}, err
	}
	if !allowed {
		return devices.Device{}, store.ErrNotFound
	}
	r, err := a.Store.UserDevices(ctx, id)
	if err != nil {
		return devices.Device{}, err
	}
	var selected *devices.Device
	for i, d := range r.Devices {
		if d.ID == deviceID && d.RevokedAt == 0 {
			selected = &r.Devices[i]
			break
		}
	}
	if selected == nil {
		return devices.Device{}, store.ErrNotFound
	}
	// Fail closed if restored/configured state has an unsupported credential path.
	if err := a.Store.CheckPackageCoverage(ctx, 1, 0); err != nil {
		return devices.Device{}, err
	}
	return *selected, nil
}
func (a *App) DeviceWireGuardConfig(ctx context.Context, id int64, deviceID string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	d, err := a.authorizedDevice(ctx, id, deviceID)
	if err != nil {
		return "", err
	}
	p, err := a.Store.GetProtocol(ctx, protocols.WireGuard)
	if err != nil {
		return "", err
	}
	if !p.Enabled || d.WGPrivate == "" {
		return "", store.ErrNotFound
	}
	return protocols.ClientTunnel(p, store.ProtocolCredential{UserID: id, Kind: protocols.WireGuard, PrivateKey: d.WGPrivate, PublicKey: d.WGPublic, Address: d.WGAddress})
}
