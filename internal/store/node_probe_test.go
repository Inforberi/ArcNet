package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/auth"
	"github.com/arcvpn/arcvpn/internal/db"
)

func testAppliedProbeNode(t *testing.T, s *Store) (string, string, NodeOperation) {
	t.Helper()
	ctx := context.Background()
	id, secret := createControlTestNode(t, s)
	op, err := s.QueueNodeDeployment(ctx, id, []byte(`{"inbounds":[],"outbounds":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PendingNodeCommand(ctx, id, secret); err != nil {
		t.Fatal(err)
	}
	if err = s.AcknowledgeNodeOperation(ctx, id, secret, op.ID, op.Revision, op.Digest, "applied"); err != nil {
		t.Fatal(err)
	}
	return id, secret, op
}

func TestProbeGrantSingleUseBoundToAppliedRevisionAndNoAutoPublish(t *testing.T) {
	s, cleanup := createControlTestStore(t)
	defer cleanup()
	ctx := context.Background()
	id, _, op := testAppliedProbeNode(t, s)
	grant, err := s.CreateProbeGrant(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	report := ProbeReport{NodeID: id, Revision: op.Revision, Digest: op.Digest, Result: "pass", DurationMS: 123}
	if _, err = s.CompleteProbe(ctx, "not-a-grant", report); !errors.Is(err, ErrProbeGrant) {
		t.Fatalf("untrusted report accepted: %v", err)
	}
	if _, err = s.CompleteProbe(ctx, grant.Token, ProbeReport{NodeID: id, Revision: op.Revision, Digest: strings.Repeat("0", 64), Result: "pass"}); !errors.Is(err, ErrProbeGrant) {
		t.Fatalf("mismatched digest: %v", err)
	}
	state, err := s.CompleteProbe(ctx, grant.Token, report)
	if err != nil || state.Probe != "pass" || state.ProbeRevision != op.Revision || state.Published || state.ProbeCheckedAt == 0 {
		t.Fatalf("probe state: %+v err=%v", state, err)
	}
	if _, err = s.CompleteProbe(ctx, grant.Token, report); !errors.Is(err, ErrProbeGrant) {
		t.Fatalf("replayed token: %v", err)
	}
	if _, err = s.QueueNodeDeployment(ctx, id, []byte(`{"inbounds":[{"tag":"new"}]}`)); err != nil {
		t.Fatal(err)
	}
	state, err = s.NodeControl(ctx, id)
	if err != nil || state.Probe != "unknown" || state.ProbeRevision != 0 || state.Published {
		t.Fatalf("previous pass survived deploy: %+v %v", state, err)
	}
	if _, err = s.CreateProbeGrant(ctx, id); !errors.Is(err, ErrConflict) {
		t.Fatalf("issued grant during deploy: %v", err)
	}
}

func TestProbeGrantRejectsStaleDrainedExpiredAndConcurrentReplay(t *testing.T) {
	s, cleanup := createControlTestStore(t)
	defer cleanup()
	ctx := context.Background()
	id, _, op := testAppliedProbeNode(t, s)
	grant, err := s.CreateProbeGrant(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	report := ProbeReport{NodeID: id, Revision: op.Revision, Digest: op.Digest, Result: "fail", DurationMS: 300}
	if _, err = s.SetNodeDraining(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CompleteProbe(ctx, grant.Token, report); !errors.Is(err, ErrProbeGrant) {
		t.Fatalf("drained accepted: %v", err)
	}
	if _, err = s.CreateProbeGrant(ctx, id); !errors.Is(err, ErrConflict) {
		t.Fatalf("issued drained grant: %v", err)
	}
	if _, err = s.SetNodeDraining(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	newGrant, err := s.CreateProbeGrant(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CompleteProbe(ctx, grant.Token, report); !errors.Is(err, ErrProbeGrant) {
		t.Fatalf("old grant still valid: %v", err)
	}
	if err = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("UPDATE node_probe_grants SET expires=? WHERE hash=?", "1", hashForProbe(newGrant.Token))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CompleteProbe(ctx, newGrant.Token, report); !errors.Is(err, ErrProbeGrant) {
		t.Fatalf("expired accepted: %v", err)
	}
	concurrencyGrant, err := s.CreateProbeGrant(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 16)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.CompleteProbe(ctx, concurrencyGrant.Token, report); results <- e }()
	}
	wg.Wait()
	close(results)
	count := 0
	for e := range results {
		if e == nil {
			count++
		} else if !errors.Is(e, ErrProbeGrant) {
			t.Errorf("unexpected concurrent error: %v", e)
		}
	}
	if count != 1 {
		t.Fatalf("expected one accepted report, got %d", count)
	}
	state, err := s.NodeControl(ctx, id)
	if err != nil || state.Probe != "fail" || state.Published {
		t.Fatalf("failed report promoted endpoint: %+v %v", state, err)
	}
	if err = s.DB.WithTx(ctx, func(tx *db.Tx) error {
		return tx.Exec("UPDATE node_control SET probe_checked_at=? WHERE node_id=?", "1", id)
	}); err != nil {
		t.Fatal(err)
	}
	stale, err := s.NodeControl(ctx, id)
	if err != nil || stale.Probe != "stale" {
		t.Fatalf("old report not stale: %+v %v", stale, err)
	}
}

func hashForProbe(token string) string { return auth.HashToken(token) }

func TestMigrate062ProbeColumnsPreservesNodeRevisions(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	// 0.6.2 layout has draining but no probe_revision/checked_at.
	old := strings.Replace(nodeControlSchema, " probe_checked_at INTEGER NOT NULL DEFAULT 0, probe_revision INTEGER NOT NULL DEFAULT 0,\n", "", 1)
	if err = d.WithTx(ctx, func(tx *db.Tx) error { return tx.Exec(old) }); err != nil {
		t.Fatal(err)
	}
	s := &Store{DB: d}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	id, _, op := testAppliedProbeNode(t, s)
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := s.NodeControl(ctx, id)
	if err != nil || state.Desired != op.Revision || state.Applied != op.Revision || state.Probe != "unknown" {
		t.Fatalf("migration broke revisions: %+v %v", state, err)
	}
	if grant, err := s.CreateProbeGrant(ctx, id); err != nil || grant.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("migrated grant failed: %+v %v", grant, err)
	}
}
