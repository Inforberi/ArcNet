package store

import (
	"context"
	"github.com/arcvpn/arcvpn/internal/db"
	"path/filepath"
	"strings"
	"testing"
)

func TestAtomicPortableManagement(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	st := &Store{DB: database}
	ctx := context.Background()
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	enrolled, err := st.CreateEnrollment(ctx, "Test Node")
	if err != nil {
		t.Fatal(err)
	}
	oldNode, _, err := st.RegisterNode(ctx, enrolled)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := st.ListNodeIdentities(ctx)
	if err != nil || len(nodes) != 1 || nodes[0].ID != oldNode.ID {
		t.Fatal(err, nodes)
	}
	bad := []NodeIdentity{{ID: "00000000-0000-4000-8000-000000000001", Name: "Node A", TokenHash: strings.Repeat("a", 64), CreatedAt: 1}, {ID: "00000000-0000-4000-8000-000000000002", Name: "Node B", TokenHash: strings.Repeat("a", 64), CreatedAt: 1}}
	budget := 100
	if err := st.ReplaceFullStateWithManagement(ctx, nil, nil, nil, nil, &bad, &budget); err == nil {
		t.Fatal("duplicate hash was accepted")
	}
	after, err := st.ListNodeIdentities(ctx)
	if err != nil || len(after) != 1 || after[0].ID != oldNode.ID {
		t.Fatal("transaction lost old node", err, after)
	}
	good := bad[:1]
	if err := st.ReplaceFullStateWithManagement(ctx, nil, nil, nil, nil, &good, &budget); err != nil {
		t.Fatal(err)
	}
	after, err = st.ListNodeIdentities(ctx)
	if err != nil || len(after) != 1 || after[0].ID != bad[0].ID {
		t.Fatal("node restore failed", err, after)
	}
	got, err := st.LogBudgetMB(ctx)
	if err != nil || got != 100 {
		t.Fatal("budget not restored", err, got)
	}
}
