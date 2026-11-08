package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/protocols"
)

func TestHysteriaSalamanderAndBandwidth(t *testing.T) {
	a := firstProtocolApp(t)
	ctx := context.Background()
	p, err := a.AddProtocol(ctx, protocols.Hysteria2, "198.51.100.1", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	u, err := a.AddUser(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	settings := settingsFor(p)
	options := p.ServerOptions
	options.Salamander = true
	options.UpMbps = 10
	options.DownMbps = 20
	settings.ServerOptions = &options
	if err = a.EditConnection(ctx, p.Kind, ConnectionRevision(p), settings); err != nil {
		t.Fatal(err)
	}
	got, err := a.Store.GetProtocol(ctx, p.Kind)
	if err != nil || len(got.ObfsKey) < 32 {
		t.Fatal("secret not generated", err)
	}
	public, _ := json.Marshal(got)
	if strings.Contains(string(public), got.ObfsKey) {
		t.Fatal("secret in public profile")
	}
	links, err := a.SubscriptionLinks(ctx, u)
	if err != nil || len(links) != 1 || !strings.Contains(links[0], "obfs=salamander") || !strings.Contains(links[0], "obfs-password=") {
		t.Fatal("subscription missing obfuscation", err)
	}
	config, err := protocols.Render(got, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	_ = json.Unmarshal(config, &cfg)
	if cfg["obfs"].(map[string]any)["salamander"].(map[string]any)["password"] != got.ObfsKey || cfg["bandwidth"].(map[string]any)["up"] != "10 mbps" || cfg["bandwidth"].(map[string]any)["down"] != "20 mbps" {
		t.Fatal("server/client options differ")
	}
	settings.ServerOptions.UpMbps = 15
	if err = a.EditConnection(ctx, p.Kind, ConnectionRevision(got), settings); err != nil {
		t.Fatal(err)
	}
	after, _ := a.Store.GetProtocol(ctx, p.Kind)
	if after.ObfsKey != got.ObfsKey {
		t.Fatal("ordinary edit regenerated secret")
	}
}
