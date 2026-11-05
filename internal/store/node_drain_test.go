package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/db"
)

func TestNodeDrainDoesNotStopVPNOrClaimProbe(t *testing.T) {
	ctx := context.Background()
	s, cleanup := createControlTestStore(t)
	defer cleanup()
	id, secret := createControlTestNode(t, s)

	// Draining is always possible, even when the agent is offline or never
	// accepted a deployment. It must not mutate the running VPN revision.
	state, err := s.SetNodeDraining(ctx, id, true)
	if err != nil || !state.Draining || state.Published || state.Probe != "unknown" {
		t.Fatalf("failed to drain node: %+v, %v", state, err)
	}
	if _, err = s.SetNodeDraining(ctx, id, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("resumed unconfigured node: %v", err)
	}
	operation, err := s.QueueNodeDeployment(ctx, id, []byte(`{"inbounds":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PendingNodeCommand(ctx, id, secret); err != nil {
		t.Fatal(err)
	}
	if err = s.AcknowledgeNodeOperation(ctx, id, secret, operation.ID, operation.Revision, operation.Digest, "applied"); err != nil {
		t.Fatal(err)
	}
	state, err = s.NodeControl(ctx, id)
	if err != nil || !state.Draining || state.Applied != 1 || state.Published || state.Probe != "unknown" {
		t.Fatalf("deployment lifted drain or faked a probe: %+v, %v", state, err)
	}
	state, err = s.SetNodeDraining(ctx, id, false)
	if err != nil || state.Draining || state.Applied != 1 || state.Published || state.Probe != "unknown" {
		t.Fatalf("resumed incorrectly: %+v, %v", state, err)
	}
	// A duplicate resume is idempotent and cannot clear a separate
	// publication decision or bump its timestamp.
	if err = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("UPDATE node_control SET published=1 WHERE node_id=?", id)
	}); err != nil {
		t.Fatal(err)
	}
	again, err := s.SetNodeDraining(ctx, id, false)
	if err != nil || !again.Published || again.UpdatedAt != state.UpdatedAt {
		t.Fatalf("repeated resume unexpectedly unpublished node: %+v %v", again, err)
	}
	if _, err = s.SetNodeDraining(ctx, "missing", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing node was accepted: %v", err)
	}
}

func TestMigrates061NodeControlWithoutResettingRevisions(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	legacy := `CREATE TABLE IF NOT EXISTS node_control (
        node_id TEXT PRIMARY KEY REFERENCES managed_nodes(id) ON DELETE CASCADE,
        address TEXT NOT NULL DEFAULT '', port INTEGER NOT NULL DEFAULT 443,
        desired_revision INTEGER NOT NULL DEFAULT 0, applied_revision INTEGER NOT NULL DEFAULT 0,
        last_good_revision INTEGER NOT NULL DEFAULT 0,
        desired_digest TEXT NOT NULL DEFAULT '', desired_ciphertext TEXT NOT NULL DEFAULT '',
        deployment_id TEXT NOT NULL DEFAULT '', deployment_status TEXT NOT NULL DEFAULT 'idle',
        last_error TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL DEFAULT 0,
        probe TEXT NOT NULL DEFAULT 'unknown', published INTEGER NOT NULL DEFAULT 0,
        next_secret_hash TEXT NOT NULL DEFAULT '', next_secret_expires INTEGER NOT NULL DEFAULT 0
    )`
	if err = database.WithTx(ctx, func(tx *db.Tx) error { return tx.Exec(legacy) }); err != nil {
		t.Fatal(err)
	}
	s := &Store{DB: database}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	id, _ := createControlTestNode(t, s)
	if err = database.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("UPDATE node_control SET desired_revision=4,applied_revision=3,last_good_revision=3 WHERE node_id=?", id)
	}); err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	ctrl, err := s.NodeControl(ctx, id)
	if err != nil || ctrl.Desired != 4 || ctrl.Applied != 3 || ctrl.Draining {
		t.Fatalf("migration damaged state %+v %v", ctrl, err)
	}
}
