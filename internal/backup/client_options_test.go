package backup

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"testing"
)

func TestClientOptionsBackupRoundTripAndLegacy(t *testing.T) {
	s := sample()
	p := store.Protocol{Kind: "vless-xhttp", Address: s.VPN.Address, Port: 2087, SNI: s.VPN.SNI, Target: s.VPN.Target, PrivateKey: s.VPN.PrivateKey, PublicKey: s.VPN.PublicKey, ShortID: s.VPN.ShortID, Path: "/fixture", Enabled: true, ClientOptions: store.ClientOptions{XHTTPMode: "packet-up"}}
	wg, err := protocols.New(context.Background(), protocols.WireGuard, p.Address, 0, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := protocols.NewCredential(protocols.WireGuard, store.User{ID: s.Users[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	zero := 0
	wg.ClientOptions = store.ClientOptions{MTU: 1420, DNS: "9.9.9.9", Keepalive: &zero}
	s = s.WithProtocols([]store.Protocol{p, wg}, []store.ProtocolCredential{{UserID: s.Users[0].ID, Kind: p.Kind, Secret: s.Users[0].UUID}, credential})
	blob, err := Encrypt(s, "client-options-backup-password")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(blob, "client-options-backup-password")
	if err != nil {
		t.Fatal(err)
	}
	profiles, _ := got.ToProtocols()
	if len(profiles) != 2 || profiles[0].ClientOptions.Mode() != "packet-up" || profiles[1].ClientOptions.Keepalive == nil || *profiles[1].ClientOptions.Keepalive != 0 || profiles[1].ClientOptions.MTU != 1420 || profiles[1].ClientOptions.DNS != "9.9.9.9" {
		t.Fatal("options lost in backup")
	}
	s.Version = 15
	if s.Validate() == nil {
		t.Fatal("new options accepted in legacy version")
	}
	s.Protocols[0].ClientOptions = store.ClientOptions{}
	s.Protocols[1].ClientOptions = store.ClientOptions{}
	if err = s.Validate(); err != nil {
		t.Fatal("legacy rejected", err)
	}
}

func TestFlowAndSalamanderBackup(t *testing.T) {
	s := sample()
	s.VPN.ServerOptions.Flow = "none"
	p, err := protocols.New(context.Background(), protocols.Hysteria2, s.VPN.Address, 8444, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	p.ServerOptions.Salamander = true
	p.ServerOptions.UpMbps = 10
	p.ObfsKey, err = auth.Secret(32)
	if err != nil {
		t.Fatal(err)
	}
	c, err := protocols.NewCredential(p.Kind, store.User{ID: s.Users[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	s = s.WithProtocols([]store.Protocol{p}, []store.ProtocolCredential{c})
	blob, err := Encrypt(s, "flow-obfuscation-backup-password")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(blob, "flow-obfuscation-backup-password")
	if err != nil {
		t.Fatal(err)
	}
	r, _ := got.ToStore()
	profiles, _ := got.ToProtocols()
	if r.ServerOptions.VLESSFlow() != "" || len(profiles) != 1 || profiles[0].ObfsKey != p.ObfsKey || !profiles[0].ServerOptions.Salamander || profiles[0].ServerOptions.UpMbps != 10 {
		t.Fatal("new settings lost")
	}
	s.Version = 17
	if s.Validate() == nil {
		t.Fatal("extended fields accepted under old version")
	}
	s.VPN.ServerOptions.Flow = ""
	s.Protocols[0].ServerOptions = store.ServerOptions{}
	s.Protocols[0].ObfsKey = ""
	if err = s.Validate(); err != nil {
		t.Fatal("legacy rejected", err)
	}
}

func TestServerOptionsBackupRoundTripAndLegacy(t *testing.T) {
	s := sample()
	off := false
	s.VPN.ServerOptions = store.ServerOptions{Sniffing: &off}
	p, err := protocols.New(context.Background(), protocols.Hysteria2, s.VPN.Address, 8444, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	p.ServerOptions = store.ServerOptions{UDPIdleSeconds: 90, QUICIdleSeconds: 45}
	c, err := protocols.NewCredential(p.Kind, store.User{ID: s.Users[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	s = s.WithProtocols([]store.Protocol{p}, []store.ProtocolCredential{c})
	blob, err := Encrypt(s, "server-options-backup-password")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(blob, "server-options-backup-password")
	if err != nil {
		t.Fatal(err)
	}
	r, _ := got.ToStore()
	profiles, _ := got.ToProtocols()
	if r.ServerOptions.SniffingEnabled("vless-reality") || len(profiles) != 1 || profiles[0].ServerOptions.UDPIdleSeconds != 90 {
		t.Fatal("server options lost")
	}
	remapped, err := got.RemapAddress("1.1.1.1")
	if err != nil || remapped.VPN.ServerOptions.SniffingEnabled("vless-reality") {
		t.Fatal("rebind lost settings", err)
	}
	s.Version = 16
	if s.Validate() == nil {
		t.Fatal("new field accepted under old version")
	}
	s.VPN.ServerOptions = store.ServerOptions{}
	s.Protocols[0].ServerOptions = store.ServerOptions{}
	if err = s.Validate(); err != nil {
		t.Fatal(err)
	}
}
