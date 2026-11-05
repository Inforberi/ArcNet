package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestBackupV2RoundtripAllProfiles(t *testing.T) {
	original := sample()
	user := store.User{ID: original.Users[0].ID, UUID: original.Users[0].UUID, Enabled: true}
	profiles := make([]store.Protocol, 0, 3)
	credentials := make([]store.ProtocolCredential, 0, 3)
	for _, kind := range []string{protocols.Hysteria2, protocols.WireGuard, protocols.AmneziaWG} {
		p, e := protocols.New(context.Background(), kind, original.VPN.Address, 0, "", "", "")
		if e != nil {
			t.Fatal(e)
		}
		c, e := protocols.NewCredential(kind, user)
		if e != nil {
			t.Fatal(e)
		}
		profiles = append(profiles, p)
		credentials = append(credentials, c)
	}
	original = original.WithProtocols(profiles, credentials)
	blob, e := Encrypt(original, "an-adequately-long-passphrase-123")
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(blob, []byte(profiles[2].PrivateKey)) || bytes.Contains(blob, []byte(credentials[0].Secret)) {
		t.Fatal("encrypted envelope leaks secrets")
	}
	restored, e := Decrypt(blob, "an-adequately-long-passphrase-123")
	if e != nil {
		t.Fatal(e)
	}
	savedP, savedC := restored.ToProtocols()
	if restored.Version != Version || len(savedP) != 3 || len(savedC) != 3 || savedP[2].PrivateKey != profiles[2].PrivateKey || savedC[0].Secret != credentials[0].Secret {
		t.Fatal("protocol credentials corrupted during backup")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{"orphan", func(s *Snapshot) { s.Credentials[0].UserID = 999 }},
		{"duplicate", func(s *Snapshot) { s.Protocols = append(s.Protocols, s.Protocols[0]) }},
		{"credential missing", func(s *Snapshot) { s.Credentials = s.Credentials[:1] }},
		{"untrusted cert path", func(s *Snapshot) { s.Protocols[0].TLSCert = "/tmp/unsafe" }},
		{"wrong protocol server IP", func(s *Snapshot) { s.Protocols[0].Address = "198.51.100.99" }},
		{"no VPN with protocols", func(s *Snapshot) { s.VPN = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copyState := original.WithProtocols(profiles, credentials)
			tc.mutate(&copyState)
			if !errors.Is(copyState.Validate(), ErrInvalid) {
				t.Fatal("unsafe backup accepted")
			}
		})
	}
}

func TestLegacyBackupCompatibility(t *testing.T) {
	old := sample()
	old.Version = 1
	blob, e := Encrypt(old, "an-adequately-long-passphrase-123")
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := Decrypt(blob, "an-adequately-long-passphrase-123")
	if e != nil {
		t.Fatal(e)
	}
	if recovered.Version != 1 || len(recovered.Protocols) != 0 || recovered.Users[0].UUID != old.Users[0].UUID {
		t.Fatal("version 1 backup no longer supported")
	}
}

// A portable XHTTP URI must use exactly the UUID accepted by the Xray inbound.
// A syntactically valid but unrelated credential must not survive import.
func TestBackupRejectsXHTTPCredentialDifferentFromVPNUser(t *testing.T) {
	state := sample()
	user := store.User{ID: state.Users[0].ID, UUID: state.Users[0].UUID, Enabled: true}
	binary := filepath.Join(t.TempDir(), "xray")
	stub := "#!/bin/sh\nif [ \"$1\" = x25519 ]; then echo 'PrivateKey: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'; echo 'Password: BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB'; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(binary, []byte(stub), 0700); err != nil {
		t.Fatal(err)
	}
	profile, err := protocols.New(context.Background(), protocols.XHTTP, state.VPN.Address, 2087, "www.example.com", "www.example.com", binary)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := protocols.NewCredential(protocols.XHTTP, user)
	if err != nil {
		t.Fatal(err)
	}
	valid := state.WithProtocols([]store.Protocol{profile}, []store.ProtocolCredential{credential})
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}
	invalid := state.WithProtocols([]store.Protocol{profile}, []store.ProtocolCredential{credential})
	invalid.Credentials[0].Secret = "00000000-0000-4000-8000-000000000001"
	if !errors.Is(invalid.Validate(), ErrInvalid) {
		t.Fatal("mismatched XHTTP authentication credential accepted")
	}
}
