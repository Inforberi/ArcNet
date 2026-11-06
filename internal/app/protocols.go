package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/engines"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

// applyXray includes every configured XHTTP listener in the same validated
// Xray configuration as the original TCP Reality inbound.
func (a *App) applyXray(ctx context.Context, reality store.Reality, users []store.User) error {
	if err := a.Store.EnsureWireGuardDevices(ctx); err != nil {
		return err
	}
	var deviceErr error
	users, deviceErr = a.xrayUsers(ctx, users)
	if deviceErr != nil {
		return deviceErr
	}
	previousEpoch := ""
	previousHysteriaEpoch, _ := a.collectHysteriaUsage(ctx, users, "")
	previousWireGuardEpoch, _ := a.collectWireGuardUsage(ctx, users, "")
	if counters, at, err := a.Core.Statistics(ctx, users, true); err == nil {
		if err := a.Store.RecordUsage(ctx, counters, at); err != nil {
			return err
		}
		if len(counters) > 0 {
			previousEpoch = counters[0].Epoch
		}
	}
	var accessErr error
	users, accessErr = a.effectiveUsers(ctx, users)
	if accessErr != nil {
		return accessErr
	}
	profiles, err := a.Store.ListProtocols(ctx)
	if err != nil {
		return err
	}
	// Refuse an unmanageable auxiliary listener before modifying Xray.
	if a.Engines == nil {
		for _, p := range profiles {
			if p.Kind != protocols.XHTTP && p.Enabled {
				return errors.New("auxiliary engine controller not configured")
			}
		}
	}
	// Read all state before touching a running VPN service.
	var credentials []store.ProtocolCredential
	if a.Engines != nil {
		credentials, err = a.Store.ListProtocolCredentials(ctx)
		if err != nil {
			return err
		}
	}
	// Snapshot the active Xray configuration for cross-engine rollback.
	// A failure in Hysteria/WG must not leave a new Xray listener active
	// while the database transaction is being rolled back by the caller.
	before, readErr := a.Core.Snapshot()
	if readErr != nil {
		return readErr
	}
	xrayNeeded := reality.Enabled
	for _, p := range profiles {
		if p.Kind == protocols.XHTTP && p.Enabled {
			xrayNeeded = true
		}
	}
	// Auxiliary-only hosts do not need an Xray process, key or stats socket.
	// An existing config still needs clearing when removing its last listener.
	if xrayNeeded || len(before) > 0 {
		if err := a.Core.ApplyWithProfiles(ctx, reality, users, profiles); err != nil {
			return err
		}
	}
	if a.Engines == nil {
		a.recordAppliedUsage(ctx, users, previousEpoch)
		return nil
	}
	if err := a.Engines.Apply(ctx, profiles, users, credentials); err != nil {
		undoCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if xrayNeeded || len(before) > 0 {
			if undoErr := a.Core.Restore(undoCtx, before); undoErr != nil {
				return fmt.Errorf("auxiliary engine apply failed (%v); Xray rollback also failed: %w", err, undoErr)
			}
		}
		return fmt.Errorf("auxiliary engine apply failed; prior Xray configuration restored: %w", err)
	}
	if xrayNeeded {
		a.recordAppliedUsage(ctx, users, previousEpoch)
	}
	for _, p := range profiles {
		if p.Kind == protocols.WireGuard && p.Enabled {
			_, _ = a.collectWireGuardUsage(ctx, users, previousWireGuardEpoch)
			break
		}
	}
	for _, p := range profiles {
		if p.Kind == protocols.Hysteria2 && p.Enabled {
			observeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			// Type=simple can become active before its HTTP listener is ready.
			// Bound this observation wait; uncertainty never becomes a fake ACK.
			for attempt := 0; attempt < 10 && observeCtx.Err() == nil; attempt++ {
				if _, err := a.collectHysteriaUsage(observeCtx, users, previousHysteriaEpoch); err == nil {
					break
				}
				if a.Engines.SkipRestart {
					break
				}
				timer := time.NewTimer(100 * time.Millisecond)
				select {
				case <-observeCtx.Done():
					timer.Stop()
				case <-timer.C:
				}
			}
			cancel()
			break
		}
	}
	return nil
}

func (a *App) applyAll(ctx context.Context, reality store.Reality, users []store.User) error {
	if a.RuntimeCheck != nil {
		if err := a.RuntimeCheck(); err != nil {
			return err
		}
	}
	a.usageApplied = ""
	return a.applyXray(ctx, reality, users)
}

// ListProtocolProfiles returns independent protocol listeners. Key material is
// not present in the JSON representation of store.Protocol.
func (a *App) ListProtocolProfiles(ctx context.Context) ([]store.Protocol, error) {
	return a.Store.ListProtocols(ctx)
}

func (a *App) AddProtocol(ctx context.Context, kind, address string, port int, sni, target string) (store.Protocol, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if kind != protocols.XHTTP && kind != protocols.Hysteria2 && kind != protocols.WireGuard {
		limited, err := a.Store.HasRestrictedPackages(ctx, true)
		if err != nil {
			return store.Protocol{}, err
		}
		if limited {
			return store.Protocol{}, store.ErrUsageUnsupported
		}
	}
	if !protocols.IsSupported(kind) {
		return store.Protocol{}, protocols.ErrInvalid
	}
	if _, err := a.Store.GetProtocol(ctx, kind); err == nil {
		return store.Protocol{}, store.ErrConflict
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.Protocol{}, err
	}
	reality, err := a.Store.OptionalReality(ctx)
	if err != nil {
		return store.Protocol{}, err
	}
	if kind == protocols.AmneziaWG && reality.Address == "" {
		return store.Protocol{}, errors.New("legacy AmneziaWG requires the existing Reality configuration")
	}
	server, err := a.Store.LocalServer(ctx)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.Protocol{}, err
	}
	if err == nil && strings.TrimSpace(address) != server.Address {
		return store.Protocol{}, errors.New("protocol address must match local VPN address")
	}
	// Reuse the already selected, explicitly configured Reality target.
	if kind == protocols.XHTTP {
		if strings.TrimSpace(target) == "" {
			target = reality.Target
		}
		if strings.TrimSpace(sni) == "" {
			sni = reality.SNI
		}
	}
	// The bundled certificate is issued for the public IP. An arbitrary DNS
	// SNI without a matching certificate would silently break TLS clients.
	if kind == protocols.Hysteria2 && strings.TrimSpace(sni) != "" && strings.TrimSpace(sni) != strings.TrimSpace(address) {
		return store.Protocol{}, errors.New("Hysteria2 requires an SNI matching the installed IP certificate")
	}
	profiles, err := a.Store.ListProtocols(ctx)
	if err != nil {
		return store.Protocol{}, err
	}
	// Only one listener per protocol currently. Reject port collisions across
	// engines of the same transport and against the existing Xray TCP inbound.
	if port == 0 {
		port = protocols.DefaultPort(kind)
	}
	if a.Engines == nil && kind != protocols.XHTTP {
		return store.Protocol{}, errors.New("auxiliary engine controller not configured")
	}
	if kind == protocols.XHTTP && ((reality.Enabled && port == reality.Port) || port == 80 || port == a.ReservedTCPPort) {
		return store.Protocol{}, errors.New("TCP port already reserved by Reality, HTTPS panel or ACME")
	}
	if kind == protocols.Hysteria2 {
		if reality.Port == engines.HysteriaStatsPort || a.ReservedTCPPort == engines.HysteriaStatsPort {
			return store.Protocol{}, errors.New("TCP port reserved for private Hysteria statistics is occupied")
		}
		for _, p := range profiles {
			if p.Enabled && protocols.Transport(p.Kind) == "tcp" && p.Port == engines.HysteriaStatsPort {
				return store.Protocol{}, errors.New("TCP port reserved for private Hysteria statistics is occupied")
			}
		}
	}
	if kind == protocols.XHTTP && port == engines.HysteriaStatsPort {
		for _, p := range profiles {
			if p.Kind == protocols.Hysteria2 && p.Enabled {
				return store.Protocol{}, errors.New("TCP port reserved for private Hysteria statistics")
			}
		}
	}
	for _, p := range profiles {
		if p.Enabled && p.Port == port && protocols.Transport(p.Kind) == protocols.Transport(kind) {
			return store.Protocol{}, errors.New("protocol port already in use")
		}
	}
	p, err := protocols.New(ctx, kind, address, port, sni, target, a.Core.Binary)
	if err != nil {
		return store.Protocol{}, err
	}
	users, err := a.Store.ListUsers(ctx)
	if err != nil {
		return store.Protocol{}, err
	}
	creds := make([]store.ProtocolCredential, 0, len(users))
	for _, u := range users {
		c, e := protocols.NewCredential(kind, u)
		if e != nil {
			return store.Protocol{}, e
		}
		creds = append(creds, c)
	}
	if err := a.Store.UpsertProtocol(ctx, p); err != nil {
		return store.Protocol{}, err
	}
	if err := a.Store.SaveCredentials(ctx, creds); err != nil {
		_ = a.Store.DeleteProtocol(context.Background(), kind)
		return store.Protocol{}, err
	}
	// This stage can validate and activate XHTTP with the existing Xray engine.
	// Separate daemon activation has an explicit externally verified gate.
	if err := a.applyAll(ctx, reality, users); err != nil {
		_ = a.Store.DeleteProtocol(context.Background(), kind)
		_ = a.applyAll(context.Background(), reality, users)
		return store.Protocol{}, fmt.Errorf("protocol activation failed: %w", err)
	}
	a.Store.Event(ctx, "protocol.created", kind)
	return p, nil
}

func (a *App) SetProtocolEnabled(ctx context.Context, kind string, enabled bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	p, err := a.Store.GetProtocol(ctx, kind)
	if err != nil {
		return err
	}
	if p.Enabled == enabled {
		return nil
	}
	// Snapshot dependencies before mutation; a failed read must not leave
	// the persisted enabled flag out of sync with running services.
	r, err := a.Store.OptionalReality(ctx)
	if err != nil {
		return err
	}
	users, err := a.Store.ListUsers(ctx)
	if err != nil {
		return err
	}
	if enabled && kind != protocols.XHTTP && kind != protocols.Hysteria2 && kind != protocols.WireGuard {
		limited, err := a.Store.HasRestrictedPackages(ctx, true)
		if err != nil {
			return err
		}
		if limited {
			return store.ErrUsageUnsupported
		}
	}
	old := p
	p.Enabled = enabled
	if err := a.Store.UpsertProtocol(ctx, p); err != nil {
		return err
	}
	if err := a.applyAll(ctx, r, users); err != nil {
		_ = a.Store.UpsertProtocol(context.Background(), old)
		_ = a.applyAll(context.Background(), r, users)
		return err
	}
	a.Store.Event(ctx, "protocol.status", fmt.Sprintf("%s enabled=%t", kind, enabled))
	return nil
}

func (a *App) DeleteProtocol(ctx context.Context, kind string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	p, err := a.Store.GetProtocol(ctx, kind)
	if err != nil {
		return err
	}
	if p.Enabled {
		return errors.New("disable protocol before deletion")
	}
	reality, err := a.Store.OptionalReality(ctx)
	if err != nil {
		return err
	}
	if reality.Address == "" {
		profiles, err := a.Store.ListProtocols(ctx)
		if err != nil {
			return err
		}
		total, _, err := a.Store.UserCounts(ctx)
		if err != nil {
			return err
		}
		if len(profiles) == 1 && total > 0 {
			return store.ErrLocalServerInUse
		}
	}
	if err := a.Store.DeleteProtocol(ctx, kind); err != nil {
		return err
	}
	if a.Engines != nil {
		if err := a.Engines.RemoveDisabled(kind); err != nil {
			return fmt.Errorf("protocol deleted but stale config cleanup failed: %w", err)
		}
	}
	a.Store.Event(ctx, "protocol.deleted", kind)
	return nil
}

// ensureCredentials creates secrets for all non-TCP enabled profiles when a
// user is added. Existing users and credentials are never regenerated.
func (a *App) ensureCredentials(ctx context.Context, u store.User) error {
	profiles, err := a.Store.ListProtocols(ctx)
	if err != nil {
		return err
	}
	items := make([]store.ProtocolCredential, 0, len(profiles))
	for _, p := range profiles {
		c, err := protocols.NewCredential(p.Kind, u)
		if err != nil {
			return err
		}
		items = append(items, c)
	}
	return a.Store.SaveCredentials(ctx, items)
}

func (a *App) Connections(ctx context.Context, id int64) ([]protocols.Connection, error) {
	if err := a.UserAllowed(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return []protocols.Connection{}, nil
		}
		return nil, err
	}
	u, err := a.Store.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	profiles, err := a.Store.ListProtocols(ctx)
	if err != nil {
		return nil, err
	}
	creds, err := a.Store.CredentialsForUser(ctx, id)
	if err != nil {
		return nil, err
	}
	return protocols.Connections(u, profiles, creds)
}

func (a *App) SubscriptionLinks(ctx context.Context, u store.User) ([]string, error) {
	if err := a.UserAllowed(ctx, u.ID); err != nil {
		return nil, err
	}
	r, err := a.Store.OptionalReality(ctx)
	if err != nil {
		return nil, err
	}
	if !u.Enabled {
		return nil, store.ErrNotFound
	}
	links := []string{}
	// Existing VLESS URI remains stable for backwards compatibility.
	if r.Enabled {
		links = append(links, xray.Link(r, u))
	}
	extra, err := a.Connections(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	for _, connection := range extra {
		if connection.Format == "uri" {
			links = append(links, connection.Link)
		}
	}
	nodes, err := a.Store.PublishedCascadeEntries(ctx)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		profile, err := a.Store.NodeReality(ctx, node)
		if err != nil {
			return nil, err
		}
		links = append(links, xray.Link(profile, u))
	}
	return links, nil
}
