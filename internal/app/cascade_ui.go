package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/arcvpn/arcvpn/internal/nodebootstrap"
	"github.com/arcvpn/arcvpn/internal/xray"
	"net/netip"

	"github.com/arcvpn/arcvpn/internal/store"
)

type CascadePreview struct {
	RouteID            string               `json:"route_id"`
	RouteVersion       int64                `json:"route_version"`
	Token              string               `json:"preview_token"`
	Binding            store.CascadeBinding `json:"binding"`
	EntryAddress       string               `json:"entry_address"`
	EntryPort          int                  `json:"entry_port"`
	ExitAddress        string               `json:"exit_address"`
	ChannelPort        int                  `json:"channel_port"`
	SNI                string               `json:"sni"`
	UserCount          int                  `json:"user_count"`
	EntryRevision      int64                `json:"entry_revision"`
	ExitRevision       int64                `json:"exit_revision"`
	PublicationEnabled bool                 `json:"publication_enabled"`
}

func (a *App) CascadePreview(ctx context.Context, id string, version int64, port int) (CascadePreview, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cascadePreviewLocked(ctx, id, version, port)
}
func (a *App) cascadePreviewLocked(ctx context.Context, id string, version int64, port int) (CascadePreview, error) {
	var out CascadePreview
	ready, e := a.Store.CascadePreflight(ctx, id, version)
	if e != nil {
		return out, e
	}
	entry, e := a.Store.NodeReality(ctx, ready.Entry.NodeID)
	if e != nil {
		return out, e
	}
	exit, e := a.Store.NodeReality(ctx, ready.Exit.NodeID)
	if e != nil {
		return out, e
	}
	if !entry.Enabled || !exit.Enabled || entry.Address != ready.Entry.Address || entry.Port != ready.Entry.Port || exit.Address != ready.Exit.Address || exit.Port != ready.Exit.Port {
		return out, store.ErrCascadeNotReady
	}
	if port < 1024 || port > 65535 || port == exit.Port || port == 80 || port == a.ReservedTCPPort {
		return out, store.ErrRouteInvalid
	}
	if xray.Validate(entry) != nil || xray.Validate(exit) != nil {
		return out, store.ErrCascadeNotReady
	}
	ipA, _ := netip.ParseAddr(entry.Address)
	ipB, _ := netip.ParseAddr(exit.Address)
	if !nodebootstrap.PublicIP(ipA) || !nodebootstrap.PublicIP(ipB) || ipA.Unmap() == ipB.Unmap() {
		return out, store.ErrRouteInvalid
	}
	base, e := a.Store.AcknowledgedNodeConfig(ctx, ready.Entry.NodeID)
	if e != nil {
		return out, e
	}
	if base.Digest != ready.Entry.Digest {
		return out, store.ErrConflict
	}
	out = CascadePreview{RouteID: id, RouteVersion: version, Binding: store.CascadeBinding{EndpointID: ready.EndpointID, NodeID: ready.Entry.NodeID, InboundTag: "vless-reality", Scope: "all_tcp_users"}, EntryAddress: entry.Address, EntryPort: entry.Port, ExitAddress: exit.Address, ChannelPort: port, SNI: exit.SNI, UserCount: cascadeClientCount(base.Config), EntryRevision: ready.Entry.Applied, ExitRevision: ready.Exit.Applied}
	// An ETag-style precondition binds exactly the reviewed intent and snapshots.
	// Private key hashes are internal; the browser receives only the final token.
	data, _ := json.Marshal(struct {
		Plan              CascadePreview
		Roster, A, B      string
		Entry, Exit       store.Reality
		EntryKey, ExitKey [32]byte
	}{out, ready.RosterDigest, ready.Entry.Digest, ready.Exit.Digest, entry, exit, sha256.Sum256([]byte(entry.Private)), sha256.Sum256([]byte(exit.Private))})
	sum := sha256.Sum256(data)
	out.Token = hex.EncodeToString(sum[:])
	return out, nil
}

// ApplyCascadeFromPreview is the only HTTP-facing preparation entry. No config,
// identity, node ID or user subset can be supplied by the browser.
func (a *App) ApplyCascadeFromPreview(ctx context.Context, id string, version int64, port int, token string) (store.CascadeDeployment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	plan, e := a.cascadePreviewLocked(ctx, id, version, port)
	if e != nil {
		return store.CascadeDeployment{}, e
	}
	if token == "" || plan.Token != token {
		return store.CascadeDeployment{}, store.ErrConflict
	}
	deployment, e := a.prepareCascadeLocked(ctx, id, version, port)
	if e != nil {
		return deployment, e
	}
	return a.Store.StartCascade(ctx, deployment.ID, deployment.Version)
}
func (a *App) StartPreparedCascade(ctx context.Context, id string, version int64) (store.CascadeDeployment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	items, e := a.Store.CascadeDeployments(ctx)
	if e != nil {
		return store.CascadeDeployment{}, e
	}
	for _, item := range items {
		if item.ID == id {
			if item.Binding == nil {
				return store.CascadeDeployment{}, errors.New("cascade has no reviewed endpoint binding")
			}
			return a.Store.StartCascade(ctx, id, version)
		}
	}
	return store.CascadeDeployment{}, store.ErrNotFound
}

func (a *App) RollbackCascade(ctx context.Context, id string, version int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Store.RollbackCascade(ctx, id, version)
}
func (a *App) DeleteCascade(ctx context.Context, id string, version int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Store.DeleteCascade(ctx, id, version)
}
