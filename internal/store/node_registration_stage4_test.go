package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/db"
)

func TestEnrollmentPreallocatedNodeIDAndOneTimeUse(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "enroll.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := &Store{DB: d}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	token, preallocated, err := s.CreateEnrollmentForNodeWithID(ctx, "same display name", "9.9.9.9", 443)
	if err != nil {
		t.Fatal(err)
	}
	node, secret, err := s.RegisterNode(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if node.ID != preallocated || len(secret) < 40 {
		t.Fatal("preallocated identity lost")
	}
	control, err := s.NodeControl(ctx, node.ID)
	if err != nil || control.Address != "9.9.9.9" || control.Port != 443 {
		t.Fatal("registered endpoint incorrect")
	}
	if _, _, err = s.RegisterNode(ctx, token); err != ErrEnrollmentExpired {
		t.Fatalf("one-time enrollment reused: %v", err)
	}
}
