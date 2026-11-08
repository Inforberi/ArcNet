package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/engines"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

var ErrConnectionChanged = errors.New("connection changed since it was read")

// ConnectionRevision is an opaque optimistic lock; secrets are excluded by Protocol's JSON contract.
func ConnectionRevision(p store.Protocol) string {
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func RealityProfile(r store.Reality) store.Protocol {
	return store.Protocol{ServerOptions: r.ServerOptions, Fingerprint: r.Fingerprint, Kind: "vless-reality", Address: r.Address, Port: r.Port, Target: r.Target, SNI: r.SNI, PrivateKey: r.Private, PublicKey: r.Public, ShortID: r.ShortID, Enabled: r.Enabled}
}

type ConnectionSettings struct {
	RotateKeys    bool                 `json:"-"`
	ServerOptions *store.ServerOptions `json:"server_options,omitempty"`
	ClientOptions *store.ClientOptions `json:"client_options,omitempty"`
	Path          *string              `json:"path,omitempty"`
	Fingerprint   *string              `json:"fingerprint,omitempty"`
	Port          int                  `json:"port"`
	Target        string               `json:"target"`
	SNI           string               `json:"sni"`
	Enabled       bool                 `json:"enabled"`
}

// EditConnection preserves identity and credentials. Runtime and persistence recover together on failure.
func (a *App) EditConnection(ctx context.Context, kind, revision string, settings ConnectionSettings) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.publication.Add(1)
	defer a.publication.Add(1)
	return a.editConnectionLocked(ctx, kind, revision, settings)
}
func (a *App) editConnectionLocked(ctx context.Context, kind, revision string, settings ConnectionSettings) error {
	if store.ConnectionKind(kind) != "vless-reality" && store.ConnectionKind(kind) != protocols.XHTTP && kind != protocols.Hysteria2 && kind != protocols.WireGuard {
		return protocols.ErrInvalid
	}
	reality, err := a.Store.OptionalReality(ctx)
	if err != nil {
		return err
	}
	var old store.Protocol
	if kind == "vless-reality" {
		if reality.Address == "" {
			return store.ErrNotFound
		}
		old = RealityProfile(reality)
	} else {
		old, err = a.Store.GetProtocol(ctx, kind)
		if err != nil {
			return err
		}
	}
	if revision == "" || revision != ConnectionRevision(old) {
		return ErrConnectionChanged
	}
	next := old
	if settings.RotateKeys {
		if a.RuntimeCheck != nil {
			if err = a.RuntimeCheck(); err != nil {
				return err
			}
		}
		switch old.Kind {
		case "vless-reality", protocols.XHTTP:
			next.PrivateKey, next.PublicKey, next.ShortID, err = xray.GenerateKeys(ctx, a.Core.Binary)
		case protocols.WireGuard:
			next.PrivateKey, next.PublicKey, err = protocols.WGKeys()
		default:
			return protocols.ErrInvalid
		}
		if err != nil {
			return err
		}
		if next.PrivateKey == old.PrivateKey || next.PublicKey == old.PublicKey {
			return &xray.OperationError{Stage: "keys", Err: errors.New("generated keys did not change")}
		}
	}
	if settings.ServerOptions != nil && !old.ServerOptions.Equal(*settings.ServerOptions) {
		next.ServerOptions = *settings.ServerOptions
	}
	if store.ValidateServerOptions(old.Kind, next.ServerOptions) != nil {
		return protocols.ErrInvalid
	}
	if kind == protocols.Hysteria2 && next.ServerOptions.Salamander && next.ObfsKey == "" {
		next.ObfsKey, err = auth.Secret(32)
		if err != nil {
			return err
		}
	}
	if settings.ClientOptions != nil {
		next.ClientOptions = *settings.ClientOptions
	}
	if err = store.ValidateClientOptions(kind, next.ClientOptions); err != nil {
		return protocols.ErrInvalid
	}
	if settings.Path != nil {
		if old.Kind != protocols.XHTTP && *settings.Path != "" {
			return protocols.ErrInvalid
		}
		next.Path = *settings.Path
	}
	if settings.Fingerprint != nil {
		next.Fingerprint = *settings.Fingerprint
	}
	next.Port = settings.Port
	next.Target = strings.TrimSpace(settings.Target)
	next.SNI = strings.TrimSpace(settings.SNI)
	next.Enabled = settings.Enabled
	if err = a.validateConnectionEdit(ctx, next, reality); err != nil {
		return err
	}
	if next == old {
		return nil
	}
	if a.RuntimeCheck != nil {
		if err = a.RuntimeCheck(); err != nil {
			return err
		}
	}
	runtimeFields := next
	runtimeFields.Fingerprint = old.Fingerprint
	runtimeFields.ClientOptions = old.ClientOptions
	clientOnly := runtimeFields == old
	var users []store.User
	if !clientOnly {
		users, err = a.Store.ListUsers(ctx)
		if err != nil {
			return err
		}
	}
	candidate := reality
	save := func(ctx context.Context, p store.Protocol) error { return a.Store.UpsertProtocol(ctx, p) }
	if kind == "vless-reality" {
		candidate.Private, candidate.Public, candidate.ShortID = next.PrivateKey, next.PublicKey, next.ShortID
		candidate.ServerOptions = next.ServerOptions
		candidate.Fingerprint = next.Fingerprint
		candidate.Port = next.Port
		candidate.Target = next.Target
		candidate.SNI = next.SNI
		candidate.Enabled = next.Enabled
		save = func(ctx context.Context, p store.Protocol) error {
			r := reality
			r.Private, r.Public, r.ShortID = p.PrivateKey, p.PublicKey, p.ShortID
			r.ServerOptions = p.ServerOptions
			r.Fingerprint = p.Fingerprint
			r.Port = p.Port
			r.Target = p.Target
			r.SNI = p.SNI
			r.Enabled = p.Enabled
			return a.Store.SaveReality(ctx, r)
		}
	}
	if err = save(ctx, next); err != nil {
		return err
	}
	if clientOnly {
		a.Store.Event(ctx, "connection.client_options.updated", kind)
		return nil
	}
	if err = a.applyAll(ctx, candidate, users); err != nil {
		recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		restoreErr := save(recovery, old)
		runtimeErr := a.applyAll(recovery, reality, users)
		if restoreErr != nil || runtimeErr != nil {
			return &xray.OperationError{Stage: "rollback", Err: fmt.Errorf("connection apply failed (%v); recovery: %w", err, errors.Join(restoreErr, runtimeErr))}
		}
		return err
	}
	a.Store.Event(ctx, "connection.updated", kind)
	return nil
}

func (a *App) validateConnectionEdit(ctx context.Context, p store.Protocol, reality store.Reality) error {
	if p.Kind == "vless-reality" {
		r := reality
		r.Address = p.Address
		r.Private, r.Public, r.ShortID = p.PrivateKey, p.PublicKey, p.ShortID
		r.ServerOptions = p.ServerOptions
		r.Fingerprint = p.Fingerprint
		r.Port = p.Port
		r.Target = p.Target
		r.SNI = p.SNI
		r.Enabled = p.Enabled
		if err := xray.Validate(r); err != nil {
			return protocols.ErrInvalid
		}
	} else if err := protocols.Validate(p); err != nil {
		return err
	}
	if p.Kind != protocols.XHTTP && p.Kind != "vless-reality" && p.Kind != protocols.Hysteria2 && (p.Target != "" || p.SNI != "") {
		return protocols.ErrInvalid
	}
	if p.Kind == protocols.Hysteria2 && p.Target != "" {
		return protocols.ErrInvalid
	}
	transport := "tcp"
	if p.Kind != "vless-reality" {
		transport = protocols.Transport(p.Kind)
	}
	if transport == "tcp" && (p.Port == 80 || p.Port == a.ReservedTCPPort) {
		return fmt.Errorf("%w: reserved TCP port", protocols.ErrInvalid)
	}
	if !p.Enabled {
		return nil
	}
	profiles, err := a.Store.ListProtocols(ctx)
	if err != nil {
		return err
	}
	if reality.Address != "" {
		profiles = append(profiles, RealityProfile(reality))
	}
	for _, other := range profiles {
		if !other.Enabled || other.ConnectionID() == p.ConnectionID() {
			continue
		}
		otherTransport := "tcp"
		if other.Kind != "vless-reality" {
			otherTransport = protocols.Transport(other.Kind)
		}
		if (transport == otherTransport && p.Port == other.Port) || (transport == "tcp" && p.Port == engines.HysteriaStatsPort && other.Kind == protocols.Hysteria2) || (p.Kind == protocols.Hysteria2 && otherTransport == "tcp" && other.Port == engines.HysteriaStatsPort) {
			return fmt.Errorf("%w: listener port conflict", protocols.ErrInvalid)
		}
	}
	return nil
}

func (a *App) DeleteReality(ctx context.Context, revision string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	r, err := a.Store.ReadReality(ctx)
	if err != nil {
		return err
	}
	if revision == "" || revision != ConnectionRevision(RealityProfile(r)) {
		return ErrConnectionChanged
	}
	if err = a.Store.DeleteReality(ctx); err != nil {
		return err
	}
	a.Store.Event(ctx, "connection.deleted", "vless-reality")
	return nil
}
