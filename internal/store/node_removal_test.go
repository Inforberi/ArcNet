package store

import (
	"context"
	"errors"
	"testing"
)

func TestRemovalJournalBindsOriginalInstalledNode(t *testing.T) {
	ctx := context.Background()
	s, done := createControlTestStore(t)
	defer done()
	nodeID, _ := createControlTestNode(t, s)
	if _, err := s.InstalledBootstrapForNode(ctx, nodeID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("manual node got SSH uninstall permission: %v", err)
	}
	original, err := s.CreateNodeBootstrap(ctx, "NL-1", "9.9.9.9", 22, 443)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AttachNodeBootstrap(ctx, original.ID, nodeID); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateNodeBootstrap(ctx, original.ID, "finished", "installed", ""); err != nil {
		t.Fatal(err)
	}
	saved, err := s.InstalledBootstrapForNode(ctx, nodeID)
	if err != nil || saved.ID != original.ID || saved.OperationType != "install" {
		t.Fatalf("lost immutable install marker: %+v %v", saved, err)
	}
	removal, err := s.CreateNodeRemoval(ctx, nodeID, "purge")
	if err != nil || removal.OperationType != "remove" || removal.RemovalMode != "purge" {
		t.Fatalf("cannot queue removal: %+v %v", removal, err)
	}
	if _, err = s.CreateNodeRemoval(ctx, nodeID, "purge"); !errors.Is(err, ErrConflict) {
		t.Fatalf("parallel removal allowed: %v", err)
	}
	if err = s.UpdateNodeBootstrap(ctx, removal.ID, "remove", "running", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateNodeBootstrap(ctx, removal.ID, "finished", "removed", ""); err != nil {
		t.Fatal(err)
	}
	after, err := s.NodeBootstrap(ctx, removal.ID)
	if err != nil || after.Status != "removed" || after.RemovalMode != "purge" {
		t.Fatalf("lost removal journal: %+v %v", after, err)
	}
	if err = s.RemoveNode(ctx, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.InstalledBootstrapForNode(ctx, nodeID); err != nil { // Journal survives identity removal for incident forensics.
		t.Fatalf("removal unexpectedly erased historical install record: %v", err)
	}
}
