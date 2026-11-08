package app

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/protocols"
	"github.com/arcvpn/arcvpn/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserPreferencesRuntimeAndRollback(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	if e := a.Setup(ctx, "198.51.100.19", "www.example.com", 443); e != nil {
		t.Fatal(e)
	}
	u, e := a.AddUser(ctx, "Alice")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.AddProtocol(ctx, protocols.Hysteria2, "198.51.100.19", 8444, "", ""); e != nil {
		t.Fatal(e)
	}
	p := store.UserPreferences{Title: "Personal", AllowedKinds: []string{"hysteria2"}}
	if e = a.EditUserPreferences(ctx, u.ID, "Renamed", p, 0); e != nil {
		t.Fatal(e)
	}
	config, e := os.ReadFile(a.Core.Path)
	if e != nil || strings.Contains(string(config), u.UUID) {
		t.Fatal("revoked TCP identity remains", e)
	}
	current, _ := a.Store.ByID(ctx, u.ID)
	links, e := a.SubscriptionLinks(ctx, current)
	if e != nil || len(links) != 1 || !strings.HasPrefix(links[0], "hysteria2:") {
		t.Fatal("wrong subscriptions", links, e)
	}
	if e = os.WriteFile(filepath.Join(filepath.Dir(a.Core.Binary), "fail"), []byte("1"), 0600); e != nil {
		t.Fatal(e)
	}
	// Metadata updates require no engine run, even while its validator is broken.
	p.Comment = "metadata"
	if e = a.EditUserPreferences(ctx, u.ID, "Renamed twice", p, 1); e != nil {
		t.Fatal(e)
	}
	p.AllowedKinds = []string{"vless-reality"}
	if e = a.EditUserPreferences(ctx, u.ID, "Should rollback", p, 2); e == nil {
		t.Fatal("expected apply failure")
	}
	current, _ = a.Store.ByID(ctx, u.ID)
	if current.Name != "Renamed twice" || !current.Allows("hysteria2") || current.Allows("vless-reality") || current.UUID != u.UUID || current.Subscription != u.Subscription {
		t.Fatal("rollback lost policy or identity")
	}
}
