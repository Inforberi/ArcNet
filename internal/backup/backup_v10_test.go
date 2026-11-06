package backup

import (
	"context"
	"testing"

	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestV10IndependentProtocolBackupAndRemap(t *testing.T) {
	s := sample()
	p, err := protocols.New(context.Background(), protocols.Hysteria2, s.VPN.Address, 0, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	u := s.Users[0]
	c, err := protocols.NewCredential(p.Kind, store.User{ID: u.ID, UUID: u.UUID})
	if err != nil {
		t.Fatal(err)
	}
	s.VPN = nil
	s = s.WithProtocols([]store.Protocol{p}, []store.ProtocolCredential{c})
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	remapped, err := s.RemapAddress("198.51.100.25")
	if err != nil || remapped.VPN != nil || remapped.Protocols[0].Address != "198.51.100.25" || remapped.Protocols[0].SNI != "198.51.100.25" || remapped.Credentials[0].Secret != c.Secret || s.Protocols[0].Address != p.Address {
		t.Fatal("independent remap", err)
	}
	for version := 1; version <= 9; version++ {
		legacy := s
		legacy.Version = version
		if legacy.Validate() == nil {
			t.Fatal("independent capability mislabeled", version)
		}
	}
	p2, err := protocols.New(context.Background(), protocols.WireGuard, "198.51.100.26", 0, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := protocols.NewCredential(p2.Kind, store.User{ID: u.ID, UUID: u.UUID})
	if err != nil {
		t.Fatal(err)
	}
	mixed := s.WithProtocols([]store.Protocol{p, p2}, []store.ProtocolCredential{c, c2})
	if mixed.Validate() == nil {
		t.Fatal("conflicting host identities accepted")
	}
	orphan := s
	orphan.Protocols = nil
	if orphan.Validate() == nil {
		t.Fatal("users/credentials without local host accepted")
	}
}
