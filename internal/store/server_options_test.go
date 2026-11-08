package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/db"
)

func TestServerOptionsValidation(t *testing.T) {
	on, off := true, false
	for _, tc := range []struct {
		kind  string
		o     ServerOptions
		valid bool
	}{
		{"vless-reality", ServerOptions{Sniffing: &off}, true}, {"vless-xhttp", ServerOptions{Sniffing: &on}, true},
		{"vless-reality", ServerOptions{UDPIdleSeconds: 60}, false}, {"hysteria2", ServerOptions{Sniffing: &off}, false},
		{"hysteria2", ServerOptions{UDPIdleSeconds: 2, QUICIdleSeconds: 120}, true}, {"hysteria2", ServerOptions{UDPIdleSeconds: 601}, false},
		{"hysteria2", ServerOptions{QUICIdleSeconds: 3}, false}, {"wireguard", ServerOptions{QUICIdleSeconds: 30}, false},
		{"vless-reality", ServerOptions{Flow: "none"}, true}, {"vless-reality", ServerOptions{Flow: "unsupported"}, false}, {"vless-xhttp", ServerOptions{Flow: "none"}, false},
		{"hysteria2", ServerOptions{Salamander: true, UpMbps: 100, DownMbps: 10}, true}, {"hysteria2", ServerOptions{UpMbps: -1}, false}, {"hysteria2", ServerOptions{DownMbps: 100001}, false}, {"wireguard", ServerOptions{Salamander: true}, false},
	} {
		if got := ValidateServerOptions(tc.kind, tc.o) == nil; got != tc.valid {
			t.Fatalf("invalid %s validation=%v", tc.kind, got)
		}
	}
	if !(ServerOptions{}).SniffingEnabled("vless-reality") || (ServerOptions{}).SniffingEnabled("vless-xhttp") {
		t.Fatal("legacy defaults changed")
	}
	other := false
	if !(ServerOptions{Sniffing: &off}).Equal(ServerOptions{Sniffing: &other}) {
		t.Fatal("pointer identity compared instead of boolean")
	}
}
func TestServerOptionsStoreRestore(t *testing.T) {
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
	off := false
	r := Reality{Address: "198.51.100.1", Port: 443, ServerOptions: ServerOptions{Sniffing: &off}}
	p := Protocol{Kind: "hysteria2", Address: r.Address, Port: 8444, ServerOptions: ServerOptions{UDPIdleSeconds: 90, QUICIdleSeconds: 45}}
	if err = s.SaveReality(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err = s.UpsertProtocol(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadReality(ctx)
	if err != nil || got.ServerOptions.SniffingEnabled("vless-reality") {
		t.Fatal("TCP setting lost", err)
	}
	all, err := s.ListProtocols(ctx)
	if err != nil || len(all) != 1 || all[0].ServerOptions.UDPIdleSeconds != 90 {
		t.Fatal("profile setting lost", err)
	}
	if err = s.ReplaceProtocols(ctx, nil, nil); err != nil {
		t.Fatal(err)
	}
	p.ServerOptions = ServerOptions{}
	if err = s.UpsertProtocol(ctx, p); err != nil {
		t.Fatal(err)
	}
	profile, err := s.GetProtocol(ctx, p.Kind)
	if err != nil || profile.ServerOptions != (ServerOptions{}) {
		t.Fatal("restore retained stale setting", err)
	}
	r.ServerOptions = ServerOptions{}
	if err = s.ReplaceFullState(ctx, &r, nil, []Protocol{p}, nil); err != nil {
		t.Fatal(err)
	}
	got, err = s.ReadReality(ctx)
	if err != nil || !got.ServerOptions.SniffingEnabled("vless-reality") {
		t.Fatal("legacy restore lost default", err)
	}
}
