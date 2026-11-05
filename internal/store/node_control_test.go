package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/db"
)

func createControlTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	d, e := db.Open(filepath.Join(t.TempDir(), "controller.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	s := &Store{DB: d}
	if e = s.Migrate(context.Background()); e != nil {
		t.Fatal(e)
	}
	return s, func() { _ = d.Close() }
}
func createControlTestNode(t *testing.T, s *Store) (string, string) {
	t.Helper()
	ctx := context.Background()
	token, e := s.CreateEnrollmentForNode(ctx, "Frankfurt-1", "9.9.9.9", 443)
	if e != nil {
		t.Fatal(e)
	}
	node, secret, e := s.RegisterNode(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	if e := s.Heartbeat(ctx, secret, NodeHeartbeat{Version: "0.6.0-stage4", ProtocolVersion: 1, XrayState: "unknown"}); e != nil {
		t.Fatal(e)
	}
	ctrl, e := s.NodeControl(ctx, node.ID)
	if e != nil {
		t.Fatal(e)
	}
	if ctrl.Address != "9.9.9.9" || ctrl.Port != 443 {
		t.Fatalf("lost enrollment metadata: %+v", ctrl)
	}
	return node.ID, secret
}
func TestNodeDeploymentIsEncryptedOrderedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	s, closeDB := createControlTestStore(t)
	defer closeDB()
	id, secret := createControlTestNode(t, s)
	payload := []byte(`{"password":"not-safe-to-persist-plaintext","inbounds":[]}`)
	op, e := s.QueueNodeDeployment(ctx, id, payload)
	if e != nil {
		t.Fatal(e)
	}
	if op.Revision != 1 || op.State != "queued" {
		t.Fatalf("unexpected operation: %+v", op)
	}
	e = s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, e := tx.Query("SELECT desired_ciphertext FROM node_control WHERE node_id=?", id)
		if e != nil {
			return e
		}
		if len(rows) != 1 || strings.Contains(rows[0]["desired_ciphertext"], "not-safe-to-persist-plaintext") {
			t.Fatal("plaintext VPN secret persisted")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.QueueNodeDeployment(ctx, id, payload); !errors.Is(e, ErrNodeBusy) {
		t.Fatalf("concurrent deployment accepted: %v", e)
	}
	if _, e = s.PendingNodeCommand(ctx, id, "invalid"); !errors.Is(e, ErrNodeUnauthorized) {
		t.Fatalf("unauthenticated poll: %v", e)
	}
	command, e := s.PendingNodeCommand(ctx, id, secret)
	if e != nil || command == nil {
		t.Fatalf("pending command: %+v %v", command, e)
	}
	if string(command.Config) != string(payload) || command.OperationID != op.ID {
		t.Fatal("poll payload differs from sealed config")
	}
	if e = s.AcknowledgeNodeOperation(ctx, id, secret, op.ID, 1, strings.Repeat("0", 64), "applied"); !errors.Is(e, ErrStaleNodeOperation) {
		t.Fatal("stale hash accepted")
	}
	if e = s.AcknowledgeNodeOperation(ctx, id, secret, op.ID, op.Revision, op.Digest, "applied"); e != nil {
		t.Fatal(e)
	}
	if e = s.AcknowledgeNodeOperation(ctx, id, secret, op.ID, op.Revision, op.Digest, "applied"); e != nil {
		t.Fatalf("idempotent ACK rejected: %v", e)
	}
	if e = s.AcknowledgeNodeOperation(ctx, id, secret, op.ID, op.Revision, op.Digest, "failed"); !errors.Is(e, ErrStaleNodeOperation) {
		t.Fatal("conflicting ACK accepted")
	}
	control, e := s.NodeControl(ctx, id)
	if e != nil {
		t.Fatal(e)
	}
	if control.Applied != 1 || control.LastGood != 1 || control.RevisionStatus != "applied" || control.Published {
		t.Fatalf("incorrect post-ACK state %+v", control)
	}
	another, e := s.QueueNodeDeployment(ctx, id, payload)
	if e != nil || another.Revision != 2 {
		t.Fatalf("revision increment: %+v %v", another, e)
	}
	if e = s.AcknowledgeNodeOperation(ctx, id, secret, op.ID, 1, op.Digest, "applied"); !errors.Is(e, ErrStaleNodeOperation) {
		t.Fatal("old ACK promoted revision")
	}
	if e = s.CancelNodeDeployment(ctx, id); e != nil {
		t.Fatal(e)
	}
	if command, e = s.PendingNodeCommand(ctx, id, secret); e != nil || command != nil {
		t.Fatalf("cancelled command delivered: %+v %v", command, e)
	}
}
func TestNodeFailureRetryAndRecovery(t *testing.T) {
	ctx := context.Background()
	s, cleanup := createControlTestStore(t)
	defer cleanup()
	id, secret := createControlTestNode(t, s)
	op, e := s.QueueNodeDeployment(ctx, id, []byte(`{"config":1}`))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.PendingNodeCommand(ctx, id, secret); e != nil {
		t.Fatal(e)
	}
	if e = s.AcknowledgeNodeOperation(ctx, id, secret, op.ID, op.Revision, op.Digest, "rolled_back"); e != nil {
		t.Fatal(e)
	}
	status, e := s.NodeControl(ctx, id)
	if e != nil || status.DeploymentStatus != "failed" || status.Applied != 0 || status.Probe != "unknown" {
		t.Fatalf("failed operation marked healthy: %+v %v", status, e)
	}
	retried, e := s.RetryNodeDeployment(ctx, id)
	if e != nil || retried.Revision != 1 || retried.ID != op.ID {
		t.Fatalf("retry: %+v %v", retried, e)
	}
	if _, e = s.PendingNodeCommand(ctx, id, secret); e != nil {
		t.Fatal(e)
	}
	if e = s.AcknowledgeNodeOperation(ctx, id, secret, retried.ID, retried.Revision, retried.Digest, "applied"); e != nil {
		t.Fatal(e)
	}
}
func TestNodeCredentialRotationAndSelectiveRevoke(t *testing.T) {
	ctx := context.Background()
	s, cleanup := createControlTestStore(t)
	defer cleanup()
	id, secret := createControlTestNode(t, s)
	other, otherSecret := createControlTestNode(t, s)
	next, e := s.RotateNodeCredential(ctx, id, secret)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RotateNodeCredential(ctx, id, secret); !errors.Is(e, ErrConflict) {
		t.Fatalf("overlapping rotation accepted: %v", e)
	}
	if e = s.ConfirmNodeCredential(ctx, id, secret); !errors.Is(e, ErrNodeUnauthorized) {
		t.Fatal("old secret confirmed new credential")
	}
	if e = s.ConfirmNodeCredential(ctx, id, next); e != nil {
		t.Fatal(e)
	}
	if _, e = s.PendingNodeCommand(ctx, id, secret); !errors.Is(e, ErrNodeUnauthorized) {
		t.Fatalf("old token still active: %v", e)
	}
	if _, e = s.PendingNodeCommand(ctx, id, next); e != nil {
		t.Fatalf("new token unauthorized: %v", e)
	}
	if e = s.RemoveNode(ctx, id); e != nil {
		t.Fatal(e)
	}
	if _, e = s.PendingNodeCommand(ctx, other, otherSecret); e != nil {
		t.Fatalf("revoking one node broke another: %v", e)
	}
}
func TestMissingNodeEncryptionKeyFailsClosed(t *testing.T) {
	ctx := context.Background()
	s, cleanup := createControlTestStore(t)
	defer cleanup()
	id, _ := createControlTestNode(t, s)
	if _, e := s.QueueNodeDeployment(ctx, id, []byte(`{"secret":"v1"}`)); e != nil {
		t.Fatal(e)
	}
	key := filepath.Join(filepath.Dir(s.DB.Path()), "node-control.key")
	if e := os.Remove(key); e != nil {
		t.Fatal(e)
	}
	if e := s.Migrate(ctx); e == nil {
		t.Fatal("new encryption key generated over unreadable ciphertext")
	}
}
func TestConcurrentNodeDeployOnlyOneQueued(t *testing.T) {
	ctx := context.Background()
	s, cleanup := createControlTestStore(t)
	defer cleanup()
	id, _ := createControlTestNode(t, s)
	var mu sync.Mutex
	successes := 0
	var wg sync.WaitGroup
	for n := 0; n < 12; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.QueueNodeDeployment(ctx, id, []byte(`{"x":1}`))
			if e == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			} else if !errors.Is(e, ErrNodeBusy) {
				t.Errorf("unexpected concurrent result: %v", e)
			}
		}()
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("expected exactly one active deployment, got %d", successes)
	}
}
func TestBootstrapRestartIsNotSuccess(t *testing.T) {
	ctx := context.Background()
	s, cleanup := createControlTestStore(t)
	defer cleanup()
	op, e := s.CreateNodeBootstrap(ctx, "NL-1", "9.9.9.9", 22, 443)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.MarkInterruptedBootstraps(ctx); e != nil {
		t.Fatal(e)
	}
	found, e := s.NodeBootstrap(ctx, op.ID)
	if e != nil || found.Status != "failed" || found.Step != "queued" {
		t.Fatalf("boot state: %+v %v", found, e)
	}
}

// A missed ACK must not require a new operation ID: the remote receipt
// recognizes the existing ID and can return success on the next delivery.
func TestNodeRetryPreservesOperationIdentityAfterLostACK(t *testing.T) {
	ctx := context.Background()
	s, cleanup := createControlTestStore(t)
	defer cleanup()
	id, secret := createControlTestNode(t, s)
	op, err := s.QueueNodeDeployment(ctx, id, []byte(`{"v":1}`))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxNodeDeliveryAttempts; i++ {
		cmd, err := s.PendingNodeCommand(ctx, id, secret)
		if err != nil || cmd == nil || cmd.OperationID != op.ID {
			t.Fatalf("delivery %d: %#v %v", i, cmd, err)
		}
	}
	cmd, err := s.PendingNodeCommand(ctx, id, secret)
	if err != nil || cmd != nil {
		t.Fatalf("retry budget: %#v %v", cmd, err)
	}
	state, err := s.NodeControl(ctx, id)
	if err != nil || state.DeploymentStatus != "failed" {
		t.Fatalf("expected fail closed: %#v %v", state, err)
	}
	retry, err := s.RetryNodeDeployment(ctx, id)
	if err != nil || retry.ID != op.ID || retry.Revision != op.Revision {
		t.Fatalf("new ID would break helper idempotence: %#v %v", retry, err)
	}
	cmd, err = s.PendingNodeCommand(ctx, id, secret)
	if err != nil || cmd == nil || cmd.OperationID != op.ID {
		t.Fatalf("retry delivery: %#v %v", cmd, err)
	}
	if err := s.AcknowledgeNodeOperation(ctx, id, secret, op.ID, op.Revision, op.Digest, "applied"); err != nil {
		t.Fatal(err)
	}
}

// A compromised directory entry must not redirect the controller to an
// attacker-controlled encryption key, even if that file is mode 0600.
func TestNodeEncryptionKeyRejectsSymlinksAndNonRegularFiles(t *testing.T) {
	s, cleanup := createControlTestStore(t)
	defer cleanup()
	path := filepath.Join(filepath.Dir(s.DB.Path()), "node-control.key")
	original, err := os.ReadFile(path)
	if err != nil || len(original) != 32 {
		t.Fatalf("invalid initial key: %v", err)
	}
	backup := path + ".saved"
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Remove(path)
		_ = os.Rename(backup, path)
	}()
	if err := os.Symlink(backup, path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.nodeControlKey(); err == nil {
		t.Fatal("encryption key symlink was followed")
	}
	if err := s.initializeNodeKey(context.Background()); err == nil {
		t.Fatal("controller initialization accepted a key symlink")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.nodeControlKey(); err == nil {
		t.Fatal("world-readable encryption key accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.nodeControlKey(); err == nil {
		t.Fatal("FIFO accepted as encryption key")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, path); err != nil {
		t.Fatal(err)
	}
	key, err := s.nodeControlKey()
	if err != nil || !bytes.Equal(key, original) {
		t.Fatalf("original key not recovered: %v", err)
	}
}

func TestNodeOperationJournalPrunesOldTerminalRowsButRetainsCurrent(t *testing.T) {
	ctx := context.Background()
	s, cleanup := createControlTestStore(t)
	defer cleanup()
	id, _ := createControlTestNode(t, s)
	secondID, _ := createControlTestNode(t, s)
	now := time.Now().Unix()
	err := s.DB.WithTx(ctx, func(tx *db.Tx) error {
		for i := 0; i < 320; i++ {
			rowID := fmt.Sprintf("historical-%04d", i)
			// Historical ops are terminal, and old enough to expire, but the
			// latest 60 remain available for the administrative journal.
			if err := tx.Exec(`INSERT INTO node_operations(id,node_id,kind,revision,digest,state,attempts,created_at,updated_at)
                VALUES(?,?,'apply_xray_v1',1,'','applied',1,?,?)`, rowID, id,
				strconv.FormatInt(now-int64(400-i), 10), strconv.FormatInt(now-int64((400-i)*24*3600), 10)); err != nil {
				return err
			}
		}
		return tx.Exec(`INSERT INTO node_operations(id,node_id,kind,revision,digest,state,attempts,created_at,updated_at)
            VALUES('unrelated',?,'apply_xray_v1',1,'','failed',1,?,?)`, secondID,
			strconv.FormatInt(now-400, 10), strconv.FormatInt(now-400*24*3600, 10))
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := s.QueueNodeDeployment(ctx, id, []byte(`{"inbounds":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	err = s.DB.Read(ctx, func(tx *db.Tx) error {
		rows, err := tx.Query("SELECT id FROM node_operations WHERE node_id=?", id)
		if err != nil {
			return err
		}
		if len(rows) < nodeOperationRecentLimit || len(rows) > nodeOperationRecentLimit+1 {
			t.Fatalf("journal retained %d entries; expected the latest 60 plus current", len(rows))
		}
		found := false
		for _, row := range rows {
			if row["id"] == operation.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("current in-flight operation was pruned")
		}
		others, err := tx.Query("SELECT id FROM node_operations WHERE node_id=?", secondID)
		if err != nil {
			return err
		}
		if len(others) != 1 || others[0]["id"] != "unrelated" {
			t.Fatal("pruning one node removed another node's history")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
