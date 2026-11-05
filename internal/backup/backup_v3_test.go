package backup

import (
	"github.com/arcvpn/arcvpn/internal/store"
	"strings"
	"testing"
)

func TestV3PortableManagementAndIPRemap(t *testing.T) {
	base := sample()
	base.Version = 3
	nodes := []store.NodeIdentity{{ID: "01234567-89ab-4cde-8f01-23456789abcd", Name: "Remote VPS", TokenHash: strings.Repeat("a", 64), CreatedAt: 1700000000}}
	base = base.WithManagement(nodes, 500)
	blob, err := Encrypt(base, "an-adequately-long-passphrase-123")
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := Decrypt(blob, "an-adequately-long-passphrase-123")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Version != 3 || len(recovered.Nodes) != 1 || recovered.Nodes[0].TokenHash != nodes[0].TokenHash || recovered.LogBudgetMB == nil || *recovered.LogBudgetMB != 500 {
		t.Fatal("incomplete management recovery")
	}
	replaced, err := recovered.RemapAddress("198.51.100.42")
	if err != nil || replaced.VPN.Address != "198.51.100.42" || recovered.VPN.Address == replaced.VPN.Address {
		t.Fatal("IP remap invalid or mutates original", err)
	}
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "example.com", "::1"} {
		if _, e := recovered.RemapAddress(ip); e == nil {
			t.Fatalf("accepted private or non-IP address: %s", ip)
		}
	}
	bad := recovered
	bad.Nodes = append([]store.NodeIdentity(nil), recovered.Nodes...)
	bad.Nodes[0].TokenHash = "bad"
	if e := bad.Validate(); e == nil {
		t.Fatal("accepted invalid agent identity")
	}
}
func TestLegacyVersion2StillReadable(t *testing.T) {
	snapshot := sample()
	snapshot.Version = 2
	blob, err := Encrypt(snapshot, "an-adequately-long-passphrase-123")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(blob, "an-adequately-long-passphrase-123")
	if err != nil || got.Version != 2 {
		t.Fatal(err)
	}
}
