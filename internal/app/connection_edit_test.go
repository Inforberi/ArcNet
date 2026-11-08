package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/arcvpn/arcvpn/internal/lifecycle"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

func settingsFor(p store.Protocol) ConnectionSettings {
	return ConnectionSettings{Port: p.Port, Target: p.Target, SNI: p.SNI, Enabled: p.Enabled}
}

func TestClientOptionsRollbackWithServerEdit(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	p, err := a.AddProtocol(ctx, protocols.XHTTP, "198.51.100.19", 2087, "www.example.com", "www.example.com")
	if err != nil {
		t.Fatal(err)
	}
	on, off := true, false
	settings := settingsFor(p)
	settings.ServerOptions = &store.ServerOptions{Sniffing: &on}
	settings.ClientOptions = &store.ClientOptions{XHTTPMode: "packet-up"}
	if err = a.EditConnection(ctx, p.Kind, ConnectionRevision(p), settings); err != nil {
		t.Fatal(err)
	}
	old, err := a.Store.GetProtocol(ctx, p.Kind)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(a.Core.Path)
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nfor arg; do path=\"$arg\"; done\nif grep -q '\"port\": 9443' \"$path\"; then exit 1; fi\nexit 0\n"
	if err = os.WriteFile(a.Core.Binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	settings.Port = 9443
	settings.ServerOptions = &store.ServerOptions{Sniffing: &off}
	settings.ClientOptions = &store.ClientOptions{XHTTPMode: "stream-one"}
	if err = a.EditConnection(ctx, p.Kind, ConnectionRevision(old), settings); err == nil {
		t.Fatal("failed runtime accepted")
	}
	got, err := a.Store.GetProtocol(ctx, p.Kind)
	if err != nil || !got.ServerOptions.SniffingEnabled(p.Kind) || got.Port != old.Port || got.ClientOptions.Mode() != "packet-up" || got.PrivateKey != old.PrivateKey {
		t.Fatal("rollback lost options or identity", err)
	}
	after, err := os.ReadFile(a.Core.Path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("runtime not restored", err)
	}
}

func TestConnectionEditPreservesIdentityAndRejectsStaleForm(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	if err := a.Setup(ctx, "198.51.100.19", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	user, err := a.AddUser(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	hy, err := a.AddProtocol(ctx, protocols.Hysteria2, "198.51.100.19", 8444, "", "")
	if err != nil {
		t.Fatal(err)
	}
	old, _ := a.Store.ReadReality(ctx)
	oldProfile := RealityProfile(old)
	beforeCreds, _ := a.Store.CredentialsForUser(ctx, user.ID)
	next := settingsFor(oldProfile)
	next.Port = 2083
	next.SNI = "www.apple.com"
	next.Target = next.SNI
	if err = a.EditConnection(ctx, "vless-reality", ConnectionRevision(oldProfile), next); err != nil {
		t.Fatal(err)
	}
	current, _ := a.Store.ReadReality(ctx)
	if current.Private != old.Private || current.Public != old.Public || current.ShortID != old.ShortID || current.Port != 2083 || current.SNI != next.SNI {
		t.Fatal("identity rotated or edit not persisted")
	}
	unchanged, _ := a.Store.GetProtocol(ctx, protocols.Hysteria2)
	if unchanged != hy {
		t.Fatal("unrelated Hysteria profile changed")
	}
	afterCreds, _ := a.Store.CredentialsForUser(ctx, user.ID)
	if len(beforeCreds) != len(afterCreds) || beforeCreds[0] != afterCreds[0] {
		t.Fatal("user credentials changed")
	}
	userAfter, _ := a.Store.ByID(ctx, user.ID)
	if userAfter.UUID != user.UUID || userAfter.Subscription != user.Subscription {
		t.Fatal("user identity changed")
	}
	if err = a.EditConnection(ctx, "vless-reality", ConnectionRevision(oldProfile), next); !errors.Is(err, ErrConnectionChanged) {
		t.Fatal("stale form accepted", err)
	}
	if _, err = a.AddProtocol(ctx, protocols.XHTTP, "198.51.100.19", 2087, "www.example.com", "www.example.com"); err != nil {
		t.Fatal(err)
	}
	next.Port = 2087
	if err = a.EditConnection(ctx, "vless-reality", ConnectionRevision(RealityProfile(current)), next); !errors.Is(err, protocols.ErrInvalid) {
		t.Fatal("TCP collision allowed", err)
	}
	udp := settingsFor(hy)
	udp.SNI = "other.example.com"
	if err = a.EditConnection(ctx, hy.Kind, ConnectionRevision(hy), udp); !errors.Is(err, protocols.ErrInvalid) {
		t.Fatal("certificate mismatch accepted", err)
	}
}

func TestConnectionEditFailureRestoresDatabaseAndRuntime(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	if err := a.Setup(ctx, "198.51.100.19", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	old, _ := a.Store.ReadReality(ctx)
	before, err := os.ReadFile(a.Core.Path)
	if err != nil {
		t.Fatal(err)
	}
	// Reject only the candidate, allowing the real compensation path to validate the previous state.
	script := "#!/bin/sh\nfor arg; do path=\"$arg\"; done\nif grep -q '\"port\": 9443' \"$path\"; then exit 1; fi\nexit 0\n"
	if err = os.WriteFile(a.Core.Binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	next := settingsFor(RealityProfile(old))
	next.Port = 9443
	if err = a.EditConnection(ctx, "vless-reality", ConnectionRevision(RealityProfile(old)), next); err == nil {
		t.Fatal("invalid runtime accepted")
	}
	after, _ := a.Store.ReadReality(ctx)
	runtime, _ := os.ReadFile(a.Core.Path)
	if after != old || !bytes.Equal(before, runtime) {
		t.Fatal("failed edit left partial state")
	}
}

func TestConnectionDisableDeleteAndLocalIdentityGate(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	if err := a.Setup(ctx, "198.51.100.19", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddUser(ctx, "Alice"); err != nil {
		t.Fatal(err)
	}
	r, _ := a.Store.ReadReality(ctx)
	if err := a.DeleteReality(ctx, ConnectionRevision(RealityProfile(r))); !errors.Is(err, store.ErrConflict) {
		t.Fatal("enabled Reality deleted", err)
	}
	off := settingsFor(RealityProfile(r))
	off.Enabled = false
	if err := a.EditConnection(ctx, "vless-reality", ConnectionRevision(RealityProfile(r)), off); err != nil {
		t.Fatal(err)
	}
	r, _ = a.Store.ReadReality(ctx)
	if err := a.DeleteReality(ctx, ConnectionRevision(RealityProfile(r))); !errors.Is(err, store.ErrLocalServerInUse) {
		t.Fatal("last local identity deleted", err)
	}
	hy, err := a.AddProtocol(ctx, protocols.Hysteria2, "198.51.100.19", 8444, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.DeleteReality(ctx, ConnectionRevision(RealityProfile(r))); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Store.ReadReality(ctx); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("Reality not deleted")
	}
	if _, err = a.Store.GetProtocol(ctx, hy.Kind); err != nil {
		t.Fatal("Hysteria removed", err)
	}
	offHy := settingsFor(hy)
	offHy.Enabled = false
	if err = a.EditConnection(ctx, hy.Kind, ConnectionRevision(hy), offHy); err != nil {
		t.Fatal(err)
	}
	if err = a.DeleteProtocolRevision(ctx, hy.Kind, ConnectionRevision(hy)); !errors.Is(err, ErrConnectionChanged) {
		t.Fatal("stale deletion accepted", err)
	}
}

func TestAuxiliaryConnectionEditPreservesKeysAndCredentials(t *testing.T) {
	for _, kind := range []string{protocols.XHTTP, protocols.Hysteria2, protocols.WireGuard} {
		t.Run(kind, func(t *testing.T) {
			a := firstProtocolApp(t)
			ctx := context.Background()
			target := ""
			if kind == protocols.XHTTP {
				target = "www.example.com"
			}
			p, err := a.AddProtocol(ctx, kind, "198.51.100.19", 0, target, target)
			if err != nil {
				t.Fatal(err)
			}
			u, err := a.AddUser(ctx, "Alice")
			if err != nil {
				t.Fatal(err)
			}
			creds, _ := a.Store.CredentialsForUser(ctx, u.ID)
			next := settingsFor(p)
			next.Port++
			if err = a.EditConnection(ctx, kind, ConnectionRevision(p), next); err != nil {
				t.Fatal(err)
			}
			got, _ := a.Store.GetProtocol(ctx, kind)
			p.Port = next.Port
			if got != p {
				t.Fatal("edit changed fields outside the requested port")
			}
			after, _ := a.Store.CredentialsForUser(ctx, u.ID)
			if len(creds) != 1 || len(after) != 1 || creds[0] != after[0] {
				t.Fatal("credentials rotated")
			}
		})
	}
}

func TestConnectionEditWhilePausedDoesNotMutate(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	if err := a.Setup(ctx, "198.51.100.19", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	r, _ := a.Store.ReadReality(ctx)
	next := settingsFor(RealityProfile(r))
	next.Port = 2083
	a.RuntimeCheck = func() error { return lifecycle.ErrPaused }
	if err := a.EditConnection(ctx, "vless-reality", ConnectionRevision(RealityProfile(r)), next); !errors.Is(err, lifecycle.ErrPaused) {
		t.Fatal(err)
	}
	after, _ := a.Store.ReadReality(ctx)
	if after != r {
		t.Fatal("paused edit changed persisted state")
	}
}
