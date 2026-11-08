package store

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/db"
	"path/filepath"
	"testing"
)

func TestClientOptionsValidation(t *testing.T) {
	zero := 0
	negative, overflow := -1, 65536
	for _, tc := range []struct {
		kind  string
		o     ClientOptions
		valid bool
	}{
		{"wireguard", ClientOptions{MTU: 1420, DNS: "1.1.1.1, 9.9.9.9", Keepalive: &zero}, true},
		{"wireguard", ClientOptions{DNS: "2606:4700:4700::1111"}, false},
		{"wireguard", ClientOptions{DNS: "1.1.1.1\nPostUp = bad"}, false},
		{"wireguard", ClientOptions{MTU: 1279}, false},
		{"wireguard", ClientOptions{Keepalive: &negative}, false},
		{"wireguard", ClientOptions{Keepalive: &overflow}, false},
		{"wireguard", ClientOptions{DNS: "example.com"}, false},
		{"wireguard", ClientOptions{DNS: "::"}, false},
		{"wireguard", ClientOptions{XHTTPMode: "auto"}, false},
		{"hysteria2", ClientOptions{MTU: 1280}, false},
		{"vless-xhttp", ClientOptions{XHTTPMode: "stream-one"}, true},
		{"vless-xhttp", ClientOptions{XHTTPMode: "shell"}, false},
		{"vless-reality", ClientOptions{}, true},
	} {
		if got := ValidateClientOptions(tc.kind, tc.o) == nil; got != tc.valid {
			t.Fatalf("%s: validation=%v", tc.kind, got)
		}
	}
	if (ClientOptions{}).TunnelKeepalive() != 25 || (ClientOptions{Keepalive: &zero}).TunnelKeepalive() != 0 {
		t.Fatal("explicit zero lost")
	}
}
func TestClientOptionsPersistenceRestoreAndRollback(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s := &Store{DB: database}
	ctx := context.Background()
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p := Protocol{Kind: "vless-xhttp", Address: "198.51.100.1", Port: 2087, ClientOptions: ClientOptions{XHTTPMode: "packet-up"}}
	if err = s.UpsertProtocol(ctx, p); err != nil {
		t.Fatal(err)
	}
	check := func(mode string) {
		t.Helper()
		got, e := s.GetProtocol(ctx, p.Kind)
		if e != nil || got.ClientOptions.Mode() != mode {
			t.Fatal("options lost", e)
		}
		list, e := s.ListProtocols(ctx)
		if e != nil || len(list) != 1 || list[0].ClientOptions.Mode() != mode {
			t.Fatal("list lost options", e)
		}
	}
	check("packet-up")
	p.ClientOptions.XHTTPMode = "stream-one"
	if err = s.ReplaceProtocols(ctx, []Protocol{p}, []ProtocolCredential{{UserID: 999, Kind: p.Kind, Secret: "fixture"}}); err == nil {
		t.Fatal("invalid restore accepted")
	}
	check("packet-up")
	if err = s.ReplaceFullState(ctx, nil, nil, []Protocol{p}, nil); err != nil {
		t.Fatal(err)
	}
	check("stream-one")
	p.ClientOptions = ClientOptions{}
	if err = s.ReplaceFullState(ctx, nil, nil, []Protocol{p}, nil); err != nil {
		t.Fatal(err)
	}
	check("auto")
	p.ClientOptions.XHTTPMode = "packet-up"
	if err = s.UpsertProtocol(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteProtocol(ctx, p.Kind); err != nil {
		t.Fatal(err)
	}
	p.ClientOptions = ClientOptions{}
	if err = s.UpsertProtocol(ctx, p); err != nil {
		t.Fatal(err)
	}
	check("auto")
}
