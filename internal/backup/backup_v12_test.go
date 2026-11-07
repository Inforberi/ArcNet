package backup

import (
	"strings"
	"testing"
	"time"

	"github.com/arcvpn/arcvpn/internal/store"
	"github.com/arcvpn/arcvpn/internal/usage"
)

func TestBackupV12PreservesRemoteSourceAndRejectsLegacyShape(t *testing.T) {
	now := time.Now().Unix()
	u := store.User{ID: 1, Name: "Phone", UUID: "11111111-1111-4111-8111-111111111111", Subscription: strings.Repeat("t", 43), Enabled: true, CreatedAt: now}
	node := "22222222-2222-4222-8222-222222222222"
	state := usage.State{Checkpoints: []usage.Counter{{Source: node, UserID: u.ID, Identity: u.UUID, Stream: "vless-reality", Epoch: "process", StartedAt: 1, ObservedAt: now, Upload: 100}}}
	s := FromStore(nil, []store.User{u}).WithManagement([]store.NodeIdentity{{ID: node, Name: "Node", TokenHash: strings.Repeat("a", 64), CreatedAt: now}}, 100).WithUsage(state)
	if s.Version != Version {
		t.Fatal("format")
	}
	// Keep exercising the historical v12 reader after the current format advances.
	s.Version = 12
	if e := s.Validate(); e != nil {
		t.Fatal(e)
	}
	blob, e := Encrypt(s, "private-v12-backup-fixture")
	if e != nil {
		t.Fatal(e)
	}
	r, e := Decrypt(blob, "private-v12-backup-fixture")
	if e != nil || r.Usage.Checkpoints[0].Source != node {
		t.Fatal("lost remote checkpoint", e)
	}
	s.Version = 11
	if s.Validate() == nil {
		t.Fatal("remote fields accepted in legacy format")
	}
}
