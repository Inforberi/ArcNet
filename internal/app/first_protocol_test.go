package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/backup"
	"github.com/arcvpn/arcvpn/internal/db"
	"github.com/arcvpn/arcvpn/internal/engines"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/xray"
)

func firstProtocolApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	st := &store.Store{DB: database}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "xray")
	if err := os.WriteFile(binary, []byte(`#!/bin/sh
if [ "$1" = x25519 ]; then
echo 'PrivateKey: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'
echo 'Password: BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB'
exit 0
fi
if [ "$1" = run ] && [ ! -f "$(dirname "$0")/fail" ]; then exit 0; fi
exit 1
`), 0700); err != nil {
		t.Fatal(err)
	}
	return &App{Store: st, Core: xray.Controller{Binary: binary, Path: filepath.Join(dir, "xray.json"), SkipRestart: true}, Engines: &engines.Manager{Dir: filepath.Join(dir, "engines"), SkipRestart: true}}
}

func TestAnyLocalProtocolFirstWithoutReality(t *testing.T) {
	for _, kind := range []string{protocols.XHTTP, protocols.Hysteria2, protocols.WireGuard} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			a := firstProtocolApp(t)
			sni, target := "", ""
			if kind == protocols.XHTTP {
				sni, target = "www.example.com", "www.example.com"
			}
			if kind != protocols.XHTTP {
				// Auxiliary-first creation must work even with no Xray executable.
				if err := os.Remove(a.Core.Binary); err != nil {
					t.Fatal(err)
				}
			}
			p, err := a.AddProtocol(ctx, kind, "198.51.100.25", 0, sni, target)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := a.Store.ReadReality(ctx); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("hidden Reality config", err)
			}
			if kind != protocols.XHTTP {
				if _, err := os.Stat(a.Core.Path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("unnecessary Xray config", err)
				}
			} else {
				var config struct {
					Inbounds []struct {
						Tag string `json:"tag"`
					} `json:"inbounds"`
				}
				blob, err := os.ReadFile(a.Core.Path)
				if err != nil || json.Unmarshal(blob, &config) != nil || len(config.Inbounds) != 1 || config.Inbounds[0].Tag != "vless-xhttp-reality" {
					t.Fatal("unexpected TCP listener", err)
				}
			}
			u, err := a.AddUser(ctx, "First user")
			if err != nil {
				t.Fatal(err)
			}
			links, err := a.SubscriptionLinks(ctx, u)
			if err != nil || (kind == protocols.WireGuard && len(links) != 0) || (kind != protocols.WireGuard && len(links) != 1) {
				t.Fatal("subscription", links, err)
			}
			connections, err := a.Connections(ctx, u.ID)
			if err != nil || len(connections) != 1 || connections[0].Kind != kind {
				t.Fatal("connections", err)
			}
			state, err := a.ExportState(ctx)
			if err != nil || state.VPN != nil || state.Validate() != nil {
				t.Fatal("invalid independent backup", err)
			}
			blob, err := backup.Encrypt(state, "first-protocol-test-passphrase")
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := backup.Decrypt(blob, "first-protocol-test-passphrase")
			if err != nil {
				t.Fatal(err)
			}
			restored := firstProtocolApp(t)
			if err := restored.RestoreState(ctx, decoded); err != nil {
				t.Fatal(err)
			}
			after, err := restored.ExportState(ctx)
			beforeJSON, _ := json.Marshal(state.Credentials)
			afterJSON, _ := json.Marshal(after.Credentials)
			if err != nil || after.VPN != nil || after.Users[0].Token != u.Subscription || !bytes.Equal(beforeJSON, afterJSON) || after.Protocols[0].PrivateKey != p.PrivateKey {
				t.Fatal("restore changed identity/secrets", err)
			}
			if err := a.ChangeUserDevices(ctx, u.ID, 0, "Phone", "", nil); err != nil {
				t.Fatal(err)
			}
			devices, err := a.Store.UserDevices(ctx, u.ID)
			if err != nil || len(devices.Devices) != 1 {
				t.Fatal(err)
			}
			d := devices.Devices[0]
			deviceLinks, err := a.DeviceLinks(ctx, u.ID, d.ID)
			if err != nil || (kind != protocols.WireGuard && len(deviceLinks) != 1) {
				t.Fatal("device links", err)
			}
			if kind == protocols.WireGuard {
				conf, err := a.DeviceWireGuardConfig(ctx, u.ID, d.ID)
				if err != nil || !strings.Contains(conf, "PrivateKey = "+d.WGPrivate) {
					t.Fatal("device config", err)
				}
			}
			if err := a.ChangeUserDevices(ctx, u.ID, devices.Version, "", d.ID, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := a.DeviceLinks(ctx, u.ID, d.ID); err == nil {
				t.Fatal("revoked device exported")
			}
			if err := a.ToggleUser(ctx, u.ID, false); err != nil {
				t.Fatal(err)
			}
			if err := a.ToggleUser(ctx, u.ID, true); err != nil {
				t.Fatal(err)
			}
			if err := a.SetProtocolEnabled(ctx, kind, false); err != nil {
				t.Fatal(err)
			}
			if err := a.DeleteProtocol(ctx, kind); !errors.Is(err, store.ErrLocalServerInUse) {
				t.Fatal("orphaned user identity", err)
			}
			if err := a.DeleteUser(ctx, u.ID); err != nil {
				t.Fatal(err)
			}
			if err := a.SetProtocolEnabled(ctx, kind, false); err != nil {
				t.Fatal(err)
			}
			if err := a.DeleteProtocol(ctx, kind); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFirstProtocolAddressAndPortRemainStableWhenAddingReality(t *testing.T) {
	ctx := context.Background()
	a := firstProtocolApp(t)
	p, err := a.AddProtocol(ctx, protocols.XHTTP, "198.51.100.25", 443, "www.example.com", "www.example.com")
	if err != nil {
		t.Fatal(err)
	}
	u, err := a.AddUser(ctx, "User")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Setup(ctx, "198.51.100.26", "www.example.com", 8443); err == nil {
		t.Fatal("address drift accepted")
	}
	if err := a.Setup(ctx, p.Address, "www.example.com", p.Port); err == nil {
		t.Fatal("port collision accepted")
	}
	if err := a.Setup(ctx, p.Address, "www.example.com", 8443); err != nil {
		t.Fatal(err)
	}
	after, _ := a.Store.GetProtocol(ctx, protocols.XHTTP)
	user, _ := a.Store.ByID(ctx, u.ID)
	if after.PrivateKey != p.PrivateKey || user.UUID != u.UUID || user.Subscription != u.Subscription {
		t.Fatal("adding TCP rotated existing credentials")
	}
}

func TestIndependentRestoreFailureRecoversAuxiliaryOnlyHost(t *testing.T) {
	ctx := context.Background()
	a := firstProtocolApp(t)
	if _, err := a.AddProtocol(ctx, protocols.Hysteria2, "198.51.100.25", 0, "", ""); err != nil {
		t.Fatal(err)
	}
	u, err := a.AddUser(ctx, "User")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := a.ExportState(ctx)
	other := firstProtocolApp(t)
	if err := other.Setup(ctx, "198.51.100.25", "www.example.com", 443); err != nil {
		t.Fatal(err)
	}
	candidate, _ := other.ExportState(ctx)
	if err := os.WriteFile(filepath.Join(filepath.Dir(a.Core.Path), "fail"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.RestoreState(ctx, candidate); err == nil {
		t.Fatal("failed Xray validation accepted")
	}
	after, err := a.ExportState(ctx)
	if err != nil || after.VPN != nil || len(after.Users) != 1 || after.Users[0].Token != u.Subscription || after.Protocols[0].PrivateKey != before.Protocols[0].PrivateKey || after.Credentials[0].Secret != before.Credentials[0].Secret {
		t.Fatal("failed restore lost prior state", err)
	}
}
