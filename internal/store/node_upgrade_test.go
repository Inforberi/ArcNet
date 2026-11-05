package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/db"
)

func TestNodeUpgradeJournalSerializesRemovalAndPreservesTarget(t *testing.T) {
	ctx := context.Background()
	s, done := createControlTestStore(t)
	defer done()
	nodeID, _ := createControlTestNode(t, s)
	if _, err := s.CreateNodeUpgrade(ctx, nodeID, "0.6.1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("manual node accepted SSH upgrade: %v", err)
	}
	original, err := s.CreateNodeBootstrap(ctx, "NL", "9.9.9.9", 22, 443)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AttachNodeBootstrap(ctx, original.ID, nodeID); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateNodeBootstrap(ctx, original.ID, "finished", "installed", ""); err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateNodeUpgrade(ctx, nodeID, "0.6.1-stage4")
	if err != nil {
		t.Fatal(err)
	}
	if first.OperationType != "upgrade" || first.TargetVersion != "0.6.1-stage4" || first.NodeID != nodeID {
		t.Fatalf("bad upgrade journal %+v", first)
	}
	if _, err = s.CreateNodeUpgrade(ctx, nodeID, "0.6.2-stage4"); !errors.Is(err, ErrConflict) {
		t.Fatalf("parallel upgrade allowed: %v", err)
	}
	if _, err = s.CreateNodeRemoval(ctx, nodeID, "purge"); !errors.Is(err, ErrConflict) {
		t.Fatalf("parallel removal allowed: %v", err)
	}
	if err = s.UpdateNodeBootstrap(ctx, first.ID, "upgrade", "running", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateNodeBootstrap(ctx, first.ID, "finished", "upgraded", ""); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.NodeBootstrap(ctx, first.ID)
	if err != nil || recovered.OperationType != "upgrade" || recovered.TargetVersion != "0.6.1-stage4" || recovered.Status != "upgraded" {
		t.Fatalf("lost completed upgrade %+v %v", recovered, err)
	}
	second, err := s.CreateNodeUpgrade(ctx, nodeID, "0.6.2-stage4")
	if err != nil || second.TargetVersion != "0.6.2-stage4" {
		t.Fatalf("no subsequent upgrade: %+v %v", second, err)
	}
	if err = s.MarkInterruptedBootstraps(ctx); err != nil {
		t.Fatal(err)
	}
	interrupted, err := s.NodeBootstrap(ctx, second.ID)
	if err != nil || interrupted.Status != "failed" {
		t.Fatalf("restart produced false upgrade success %+v %v", interrupted, err)
	}
}

func TestNodeBootstrapMigrationPreservesOriginalInstallMarkers(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "old.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	// Recreate a pre-0.6 journal with no operation_type, removal_mode or
	// target_version fields; 0.6.1 migration must be additive and repeatable.
	err = database.WithTx(ctx, func(tx *db.Tx) error {
		if e := tx.Exec(`CREATE TABLE node_bootstrap_operations (
            id TEXT PRIMARY KEY,name TEXT NOT NULL,address TEXT NOT NULL,ssh_port INTEGER NOT NULL,
            vpn_port INTEGER NOT NULL,status TEXT NOT NULL,step TEXT NOT NULL,created_at INTEGER NOT NULL,
            updated_at INTEGER NOT NULL,node_id TEXT NOT NULL DEFAULT '',error TEXT NOT NULL DEFAULT '')`); e != nil {
			return e
		}
		return tx.Exec(`INSERT INTO node_bootstrap_operations(id,name,address,ssh_port,vpn_port,status,step,created_at,updated_at,node_id)
            VALUES('original-marker','NL','9.9.9.9',22,443,'installed','finished',1,2,'persisted-node')`)
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{DB: database}
	for n := 0; n < 2; n++ {
		if err = s.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := s.NodeBootstrap(ctx, "original-marker")
	if err != nil || saved.OperationType != "install" || saved.Address != "9.9.9.9" || saved.NodeID != "persisted-node" || saved.TargetVersion != "" {
		t.Fatalf("migration lost journal %+v %v", saved, err)
	}
}
