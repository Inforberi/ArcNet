package protocols

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/store"
	"strings"
	"testing"
)

func TestClientOptionsWireGuardExport(t *testing.T) {
	p, err := New(context.Background(), WireGuard, "198.51.100.1", 0, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewCredential(WireGuard, store.User{ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	before, err := Render(p, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	zero := 0
	p.ClientOptions = store.ClientOptions{MTU: 1420, DNS: "9.9.9.9", Keepalive: &zero}
	conf, err := ClientTunnel(p, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"MTU = 1420", "DNS = 9.9.9.9", "PersistentKeepalive = 0"} {
		if !strings.Contains(conf, want) {
			t.Fatal("export missing setting", want)
		}
	}
	after, err := Render(p, nil, nil)
	if err != nil || string(before) != string(after) {
		t.Fatal("client options changed server config", err)
	}
	p.ClientOptions.DNS = "1.1.1.1\nPostUp = bad"
	if _, err = ClientTunnel(p, c); err == nil {
		t.Fatal("config injection accepted")
	}
}
