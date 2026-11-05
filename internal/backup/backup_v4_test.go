package backup

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/arcvpn/arcvpn/internal/routes"
	"github.com/arcvpn/arcvpn/internal/store"
)

func TestV4EncryptedRoutingRoundTripAndLegacyVersions(t *testing.T) {
	nodeID := "01234567-89ab-4cde-8f01-23456789abcd"
	p := routes.DefaultPolicy()
	p.ID = "route-a"
	p.Name = "Cascade"
	p.Mode = "cascade"
	p.PrimaryExit = nodeID
	p.Version = 7
	p.UpdatedAt = 1700000000
	snapshot := sample().WithManagement([]store.NodeIdentity{{ID: nodeID, Name: "Exit", TokenHash: strings.Repeat("a", 64), CreatedAt: 1700000000}}, 100).WithRoutes([]routes.Policy{p})
	blob, err := Encrypt(snapshot, "backup-password-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Decrypt(blob, "backup-password-long-enough")
	if err != nil || restored.Version != 4 || !reflect.DeepEqual(restored.Routes, snapshot.Routes) {
		t.Fatalf("routes lost: %+v %v", restored.Routes, err)
	}
	for _, version := range []int{1, 2, 3} {
		legacy := sample()
		legacy.Version = version
		blob, err = Encrypt(legacy, "backup-password-long-enough")
		if err != nil {
			t.Fatal(err)
		}
		if got, err := Decrypt(blob, "backup-password-long-enough"); err != nil || got.Version != version {
			t.Fatalf("legacy v%d: %v", version, err)
		}
	}
	for _, kind := range []string{"orphan", "cycle", "duplicate", "revision", "overflow", "legacy"} {
		t.Run(kind, func(t *testing.T) {
			bad := snapshot.WithRoutes(snapshot.Routes)
			switch kind {
			case "orphan":
				bad.Nodes = nil
			case "cycle":
				q := p
				q.ID = "reverse"
				q.EntryNode = nodeID
				q.PrimaryExit = routes.LocalNode
				bad.Routes = append(bad.Routes, q)
			case "duplicate":
				bad.Routes = append(bad.Routes, p)
			case "revision":
				bad.Routes[0].Version = 0
			case "overflow":
				bad.Routes[0].Version = math.MaxInt64
			case "legacy":
				bad.Version = 3
			}
			if err := bad.Validate(); err == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}
