package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/arcvpn/arcvpn/internal/db"
)

func TestEnrollmentHeartbeatAndRevocation(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := &Store{DB: d}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEnrollment(ctx, ""); err == nil {
		t.Fatal("empty name accepted")
	}
	token, err := s.CreateEnrollment(ctx, "Netherlands-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RegisterNode(ctx, "wrong"); !errors.Is(err, ErrEnrollmentExpired) {
		t.Fatalf("unexpected result: %v", err)
	}
	valid, err := s.EnrollmentValid(ctx, token)
	if err != nil || !valid {
		t.Fatalf("valid enrollment unavailable: %v", err)
	}
	node, secret, err := s.RegisterNode(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	valid, err = s.EnrollmentValid(ctx, token)
	if err != nil || valid {
		t.Fatalf("consumed grant still authorized for binary download: %v", err)
	}
	if _, _, err := s.RegisterNode(ctx, token); !errors.Is(err, ErrEnrollmentExpired) {
		t.Fatal("replayed grant accepted")
	}
	if err := s.Heartbeat(ctx, "bad", NodeHeartbeat{XrayState: "active"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unauthorized agent: %v", err)
	}
	if err := s.Heartbeat(ctx, secret, NodeHeartbeat{XrayState: "invalid"}); err == nil {
		t.Fatal("malformed heartbeat accepted")
	}
	if err := s.Heartbeat(ctx, secret, NodeHeartbeat{Version: "0.3", XrayState: "active", MemoryTotal: 1024, MemoryAvailable: 512, Load1: 0.3, UptimeSeconds: 99}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Netherlands-1" || !got[0].Online || got[0].MemoryFree != 512 || got[0].XrayState != "active" {
		t.Fatalf("unexpected nodes: %+v", got)
	}
	if err := s.RemoveNode(ctx, node.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Heartbeat(ctx, secret, NodeHeartbeat{XrayState: "active"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked agent can connect: %v", err)
	}
	got, err = s.ListNodes(ctx)
	if err != nil || len(got) != 0 {
		t.Fatalf("node not removed: %v %+v", err, got)
	}
}
func TestRecoveryRevokesEverySession(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := &Store{DB: d}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetAdminPassword(ctx, "newhash"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("recovery recreated missing account: %v", err)
	}
	if err := s.CreateAdmin(ctx, "admin", "oldhash"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSession(ctx, "sess", "csrf", 9999999999); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetAdminPassword(ctx, "newhash"); err != nil {
		t.Fatal(err)
	}
	hash, err := s.AdminHash(ctx, "admin")
	if err != nil || hash != "newhash" {
		t.Fatalf("wrong hash %q %v", hash, err)
	}
	_, valid, err := s.Session(ctx, "sess")
	if err != nil || valid {
		t.Fatal("reset did not revoke session")
	}
}
